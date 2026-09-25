package main

import (
	"os"
	"runtime"
	"testing"
)

// TestTokenRoundTrip proves a device credential is stored in the restricted
// config file and read back.
func TestTokenRoundTrip(t *testing.T) {
	t.Setenv("WAYSHARD_CONFIG", t.TempDir())
	t.Setenv("WAYSHARD_TOKEN", "")

	if err := saveToken("config-credential"); err != nil {
		t.Fatalf("saveToken: %v", err)
	}
	got, err := loadToken()
	if err != nil {
		t.Fatalf("loadToken: %v", err)
	}
	if got != "config-credential" {
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
	t.Setenv("WAYSHARD_CONFIG", t.TempDir())
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

// TestTokenMissingFileIsEmpty proves a missing credential file is not an error.
func TestTokenMissingFileIsEmpty(t *testing.T) {
	t.Setenv("WAYSHARD_CONFIG", t.TempDir())
	t.Setenv("WAYSHARD_TOKEN", "")
	got, err := loadToken()
	if err != nil {
		t.Fatalf("missing credential must not be an error: %v", err)
	}
	if got != "" {
		t.Fatalf("loadToken = %q, want empty", got)
	}
}
