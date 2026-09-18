package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/pbkdf2"
)

// KeyProvider wraps/unwraps the vault data key.
type KeyProvider interface {
	Name() string
	Wrap(dataKey []byte) ([]byte, error)
	Unwrap(wrapped []byte) ([]byte, error)
}

// FileProvider stores a wrapping key in an explicitly protected local key file.
type FileProvider struct {
	Path string
}

func (p FileProvider) Name() string { return "file" }

func (p FileProvider) loadOrCreate() ([]byte, error) {
	if b, err := os.ReadFile(p.Path); err == nil {
		if len(b) < 32 {
			return nil, errors.New("vault key file too short")
		}
		k, err := hex.DecodeString(string(trim(b)))
		if err == nil && len(k) == 32 {
			return k, nil
		}
		if len(b) == 32 {
			return b, nil
		}
		return nil, errors.New("invalid vault key file")
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p.Path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (p FileProvider) Wrap(dataKey []byte) ([]byte, error) {
	wk, err := p.loadOrCreate()
	if err != nil {
		return nil, err
	}
	return wrapAES(wk, dataKey)
}

func (p FileProvider) Unwrap(wrapped []byte) ([]byte, error) {
	wk, err := p.loadOrCreate()
	if err != nil {
		return nil, err
	}
	return unwrapAES(wk, wrapped)
}

// EnvProvider uses WAYSHARD_VAULT_KEY hex material (headless/service deployments).
type EnvProvider struct {
	Value string
}

func (p EnvProvider) Name() string { return "env" }

func (p EnvProvider) key() ([]byte, error) {
	if p.Value == "" {
		p.Value = os.Getenv("WAYSHARD_VAULT_KEY")
	}
	if p.Value == "" {
		return nil, errors.New("WAYSHARD_VAULT_KEY is not set")
	}
	b, err := hex.DecodeString(p.Value)
	if err != nil || len(b) != 32 {
		sum := sha256.Sum256([]byte(p.Value))
		return sum[:], nil
	}
	return b, nil
}

func (p EnvProvider) Wrap(dataKey []byte) ([]byte, error) {
	k, err := p.key()
	if err != nil {
		return nil, err
	}
	return wrapAES(k, dataKey)
}

func (p EnvProvider) Unwrap(wrapped []byte) ([]byte, error) {
	k, err := p.key()
	if err != nil {
		return nil, err
	}
	return unwrapAES(k, wrapped)
}

// PassphraseProvider derives a wrapping key with PBKDF2.
type PassphraseProvider struct {
	Passphrase string
	Salt       []byte
}

func (p PassphraseProvider) Name() string { return "passphrase" }

func (p PassphraseProvider) derive() []byte {
	salt := p.Salt
	if len(salt) == 0 {
		salt = []byte("wayshard-vault-v1")
	}
	return pbkdf2.Key([]byte(p.Passphrase), salt, 100000, 32, sha256.New)
}

func (p PassphraseProvider) Wrap(dataKey []byte) ([]byte, error) {
	return wrapAES(p.derive(), dataKey)
}

func (p PassphraseProvider) Unwrap(wrapped []byte) ([]byte, error) {
	return unwrapAES(p.derive(), wrapped)
}

func wrapAES(key, dataKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, dataKey, nil)...), nil
}

func unwrapAES(key, wrapped []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(wrapped) < ns {
		return nil, fmt.Errorf("wrapped key too short")
	}
	return gcm.Open(nil, wrapped[:ns], wrapped[ns:], nil)
}

func trim(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\r' || b[j-1] == '\t') {
		j--
	}
	return b[i:j]
}

func DefaultProvider(dataDir string) KeyProvider {
	if os.Getenv("WAYSHARD_VAULT_KEY") != "" {
		return EnvProvider{}
	}
	if p := os.Getenv("WAYSHARD_VAULT_PASSPHRASE"); p != "" {
		return PassphraseProvider{Passphrase: p}
	}
	return FileProvider{Path: filepath.Join(dataDir, "vault.key")}
}
