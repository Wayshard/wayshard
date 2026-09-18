package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ObjectStore is a SHA-256 content-addressed blob store.
type ObjectStore struct {
	Root string
}

func NewObjectStore(root string) (*ObjectStore, error) {
	if err := os.MkdirAll(filepath.Join(root, "sha256"), 0o700); err != nil {
		return nil, err
	}
	return &ObjectStore{Root: root}, nil
}

func (o *ObjectStore) pathFor(hash string) string {
	hash = strings.ToLower(hash)
	if len(hash) < 4 {
		return filepath.Join(o.Root, "sha256", hash)
	}
	return filepath.Join(o.Root, "sha256", hash[:2], hash[2:4], hash)
}

func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (o *ObjectStore) Put(b []byte) (string, error) {
	h := HashBytes(b)
	p := o.pathFor(h)
	if _, err := os.Stat(p); err == nil {
		return h, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return h, nil
}

func (o *ObjectStore) Get(hash string) ([]byte, error) {
	b, err := os.ReadFile(o.pathFor(hash))
	if err != nil {
		return nil, err
	}
	if HashBytes(b) != strings.ToLower(hash) {
		return nil, fmt.Errorf("%w: object hash mismatch", ErrCorrupt)
	}
	return b, nil
}

func (o *ObjectStore) Has(hash string) bool {
	_, err := os.Stat(o.pathFor(hash))
	return err == nil
}

func (o *ObjectStore) Open(hash string) (*os.File, error) {
	return os.Open(o.pathFor(hash))
}

func (o *ObjectStore) PutReader(r io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(o.Root, "obj-*.tmp")
	if err != nil {
		return "", 0, err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		return "", 0, err
	}
	hash := hex.EncodeToString(h.Sum(nil))
	p := o.pathFor(hash)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		if _, statErr := os.Stat(p); statErr == nil {
			return hash, n, nil
		}
		return "", 0, err
	}
	return hash, n, nil
}

func (o *ObjectStore) Verify(hash string) error {
	b, err := o.Get(hash)
	if err != nil {
		return err
	}
	_ = b
	return nil
}
