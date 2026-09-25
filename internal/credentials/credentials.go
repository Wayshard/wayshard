// Package credentials stores Wayshard-owned secrets (the server identity
// private key and other control-plane credentials) as restricted user config
// files rather than an encrypted vault or OS keyring.
//
// Wayshard trusts the server's own OS user, so these secrets live in the
// server data directory (created 0700) with each file written 0600. Provider
// credentials owned by a coding harness are never read or duplicated here.
package credentials

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrMissing is returned when a credential has not been set.
var ErrMissing = errors.New("credential not found")

// Store is a filesystem-backed credential store rooted at a private directory.
type Store struct {
	mu   sync.Mutex
	root string
}

// Open opens (creating if needed) a credential directory at root.
func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

// safeName maps a credential name to a filesystem-safe file name.
func safeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func (s *Store) path(name string) string {
	return filepath.Join(s.root, safeName(name))
}

// Put writes a credential value with a private file mode.
func (s *Store) Put(_ context.Context, name string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.WriteFile(s.path(name), value, 0o600)
}

// Get reads a credential value.
func (s *Store) Get(_ context.Context, name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrMissing
		}
		return nil, err
	}
	return b, nil
}

// Has reports whether a credential has been set.
func (s *Store) Has(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := os.Stat(s.path(name))
	return err == nil
}

// Status describes a credential without revealing its value.
func (s *Store) Status(name string) map[string]any {
	return map[string]any{
		"name":    name,
		"set":     s.Has(name),
		"storage": "config_file",
	}
}
