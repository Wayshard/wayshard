package main

import (
	"os"
	"testing"
)

// TestTokenHeadlessFallbackIsProtected proves the explicit headless fallback
// stores the credential in a 0600 file and round-trips it.
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
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("token file mode = %o, want 600", perm)
	}
}

// TestTokenEnvTakesPrecedence proves an explicitly supplied credential wins over
// stored material, so automation never depends on local storage.
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
