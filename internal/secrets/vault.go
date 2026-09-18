// Package secrets implements the encrypted Wayshard SecretVault.
package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Wayshard/wayshard/internal/crypto"
)

var (
	ErrLocked    = errors.New("secret vault is locked")
	ErrMissing   = errors.New("secret not found")
	ErrExists    = errors.New("vault already exists")
	ErrUnlock    = errors.New("unable to unlock vault")
	ErrNoReplace = errors.New("refusing to replace an existing vault")
)

type envelope struct {
	Version    int    `json:"version"`
	Provider   string `json:"provider"`
	WrappedKey []byte `json:"wrappedKey"`
	Nonce      []byte `json:"nonce"`
}

type record struct {
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

type Vault struct {
	mu       sync.Mutex
	root     string
	provider KeyProvider
	key      []byte
	locked   bool
}

func Open(root string, provider KeyProvider) (*Vault, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	v := &Vault{root: root, provider: provider, locked: true}
	envPath := filepath.Join(root, "vault.json")
	if _, err := os.Stat(envPath); errors.Is(err, os.ErrNotExist) {
		if err := v.create(); err != nil {
			return nil, err
		}
		return v, nil
	}
	if err := v.unlock(); err != nil {
		return v, err // locked vault still returned
	}
	return v, nil
}

func (v *Vault) Locked() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.locked
}

func (v *Vault) create() error {
	key, err := crypto.Random(32)
	if err != nil {
		return err
	}
	wrapped, err := v.provider.Wrap(key)
	if err != nil {
		return err
	}
	env := envelope{Version: 1, Provider: v.provider.Name(), WrappedKey: wrapped}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(v.root, "vault.json"), b, 0o600); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(v.root, "entries"), 0o700); err != nil {
		return err
	}
	v.key = key
	v.locked = false
	return nil
}

func (v *Vault) unlock() error {
	b, err := os.ReadFile(filepath.Join(v.root, "vault.json"))
	if err != nil {
		return err
	}
	var env envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return err
	}
	key, err := v.provider.Unwrap(env.WrappedKey)
	if err != nil {
		v.locked = true
		return fmt.Errorf("%w: %v", ErrUnlock, err)
	}
	v.key = key
	v.locked = false
	return nil
}

func (v *Vault) Put(ctx context.Context, name string, plaintext []byte) error {
	_ = ctx
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.locked {
		return ErrLocked
	}
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, plaintext, []byte(name))
	rec := record{Nonce: nonce, Ciphertext: ct}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(v.root, "entries"), 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(v.root, "entries", safe(name)+".json"), b, 0o600)
}

func (v *Vault) Get(ctx context.Context, name string) ([]byte, error) {
	_ = ctx
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.locked {
		return nil, ErrLocked
	}
	b, err := os.ReadFile(filepath.Join(v.root, "entries", safe(name)+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrMissing
		}
		return nil, err
	}
	var rec record
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, rec.Nonce, rec.Ciphertext, []byte(name))
}

func (v *Vault) Has(name string) bool {
	_, err := os.Stat(filepath.Join(v.root, "entries", safe(name)+".json"))
	return err == nil
}

func (v *Vault) Status(name string) map[string]any {
	return map[string]any{
		"name":   name,
		"set":    v.Has(name),
		"locked": v.Locked(),
	}
}

func safe(name string) string {
	sum := crypto.HashSHA256([]byte(name))
	return sum
}
