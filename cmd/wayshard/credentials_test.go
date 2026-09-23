package main

import (
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/zalando/go-keyring"
)

// withKeyring overrides the keyring indirection points for the duration of a
// test so an unavailable or failing OS keyring can be simulated deterministically.
func withKeyring(t *testing.T, set func(service, user, password string) error, get func(service, user string) (string, error)) {
	t.Helper()
	origSet, origGet := keyringSet, keyringGet
	keyringSet, keyringGet = set, get
	t.Cleanup(func() { keyringSet, keyringGet = origSet, origGet })
}

// TestMain installs a deterministic in-memory keyring so package tests do not
// depend on a host Secret Service/keychain. Credential tests override the
// indirection points to simulate an unavailable or failing keyring.
func TestMain(m *testing.M) {
	mem := map[string]string{}
	keyringSet = func(service, user, password string) error {
		mem[service+"\x00"+user] = password
		return nil
	}
	keyringGet = func(service, user string) (string, error) {
		v, ok := mem[service+"\x00"+user]
		if !ok {
			return "", keyring.ErrNotFound
		}
		return v, nil
	}
	os.Exit(m.Run())
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestTokenKeyringFailureFailsClosed proves normal mode never writes a plaintext
// credential file when the OS keyring is unavailable.
func TestTokenKeyringFailureFailsClosed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAYSHARD_HEADLESS", "")
	t.Setenv("WAYSHARD_TOKEN", "")
	withKeyring(t,
		func(_, _, _ string) error { return errors.New("secret service unavailable") },
		func(_, _ string) (string, error) { return "", errors.New("secret service unavailable") },
	)

	if err := saveToken("credential"); err == nil {
		t.Fatal("saveToken must fail closed when the keyring is unavailable")
	}
	if fileExists(tokenPath()) {
		t.Fatalf("a plaintext credential file was written at %s", tokenPath())
	}
	if _, err := loadToken(); err == nil {
		t.Fatal("loadToken must fail closed when the keyring is unavailable")
	}
}

// TestTokenKeyringNoEntryIsEmpty proves a missing keyring entry is not an error.
func TestTokenKeyringNoEntryIsEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAYSHARD_HEADLESS", "")
	t.Setenv("WAYSHARD_TOKEN", "")
	withKeyring(t,
		func(_, _, _ string) error { return nil },
		func(_, _ string) (string, error) { return "", keyring.ErrNotFound },
	)

	got, err := loadToken()
	if err != nil {
		t.Fatalf("missing entry must not be an error: %v", err)
	}
	if got != "" {
		t.Fatalf("loadToken = %q, want empty", got)
	}
}

// TestTokenNormalModeUsesKeyring proves normal mode stores and reads through the
// keyring and writes no file.
func TestTokenNormalModeUsesKeyring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAYSHARD_HEADLESS", "")
	t.Setenv("WAYSHARD_TOKEN", "")
	stored := ""
	withKeyring(t,
		func(_, _, password string) error { stored = password; return nil },
		func(_, _ string) (string, error) {
			if stored == "" {
				return "", keyring.ErrNotFound
			}
			return stored, nil
		},
	)

	if err := saveToken("keyring-credential"); err != nil {
		t.Fatalf("saveToken: %v", err)
	}
	if fileExists(tokenPath()) {
		t.Fatalf("normal mode must not write a file at %s", tokenPath())
	}
	got, err := loadToken()
	if err != nil || got != "keyring-credential" {
		t.Fatalf("loadToken = %q err=%v", got, err)
	}
}

// TestTokenNormalModeIgnoresHeadlessFile proves a leftover headless file is never
// read in normal mode (no silent downgrade).
func TestTokenNormalModeIgnoresHeadlessFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAYSHARD_TOKEN", "")
	t.Setenv("WAYSHARD_HEADLESS", "1")
	if err := saveToken("file-credential"); err != nil {
		t.Fatalf("headless saveToken: %v", err)
	}
	if !fileExists(tokenPath()) {
		t.Fatal("headless mode should have written the protected file")
	}

	t.Setenv("WAYSHARD_HEADLESS", "")
	withKeyring(t,
		func(_, _, _ string) error { return nil },
		func(_, _ string) (string, error) { return "", keyring.ErrNotFound },
	)
	got, err := loadToken()
	if err != nil {
		t.Fatalf("loadToken: %v", err)
	}
	if got != "" {
		t.Fatalf("normal mode must not read the headless file, got %q", got)
	}
}

// TestTokenHeadlessFallbackIsProtected proves the explicit headless fallback
// stores the credential in a user-private file and round-trips it.
func TestTokenHeadlessFallbackIsProtected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAYSHARD_HEADLESS", "1")
	t.Setenv("WAYSHARD_TOKEN", "")

	if err := saveToken("headless-credential"); err != nil {
		t.Fatalf("saveToken: %v", err)
	}
	got, err := loadToken()
	if err != nil {
		t.Fatalf("loadToken: %v", err)
	}
	if got != "headless-credential" {
		t.Fatalf("loadToken = %q", got)
	}
	fi, err := os.Stat(tokenPath())
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	// Unix enforces 0600; Windows relies on the user-profile ACL instead.
	if runtime.GOOS != "windows" {
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Fatalf("token file mode = %o, want 600", perm)
		}
	}
}

// TestTokenEnvTakesPrecedence proves an explicitly supplied credential wins over
// any stored material.
func TestTokenEnvTakesPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAYSHARD_HEADLESS", "1")
	if err := saveToken("stored"); err != nil {
		t.Fatalf("saveToken: %v", err)
	}
	t.Setenv("WAYSHARD_TOKEN", "from-env")
	got, err := loadToken()
	if err != nil {
		t.Fatalf("loadToken: %v", err)
	}
	if got != "from-env" {
		t.Fatalf("loadToken = %q, want from-env", got)
	}
}
