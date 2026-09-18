package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVaultRoundTripAndNoReplace(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	p := FileProvider{Path: filepath.Join(dir, "vault.key")}
	v, err := Open(filepath.Join(dir, "vault"), p)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Put(ctx, "jev.api_key", []byte("secret")); err != nil {
		t.Fatal(err)
	}
	got, err := v.Get(ctx, "jev.api_key")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret" {
		t.Fatalf("got %q", got)
	}
	// reopen with same key
	v2, err := Open(filepath.Join(dir, "vault"), p)
	if err != nil {
		t.Fatal(err)
	}
	got, err = v2.Get(ctx, "jev.api_key")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret" {
		t.Fatal("lost secret")
	}
	// wrong key must not create a replacement vault
	bad := FileProvider{Path: filepath.Join(dir, "other.key")}
	_ = os.WriteFile(bad.Path, []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"), 0o600)
	v3, err := Open(filepath.Join(dir, "vault"), bad)
	if err == nil && !v3.Locked() {
		t.Fatal("expected locked or error with wrong wrapping key")
	}
	if _, err := os.Stat(filepath.Join(dir, "vault", "vault.json")); err != nil {
		t.Fatal("vault envelope disappeared")
	}
}

func TestAPIsDoNotReturnPlaintext(t *testing.T) {
	v, _ := Open(t.TempDir()+"/v", FileProvider{Path: t.TempDir() + "/k"})
	st := v.Status("x")
	if _, ok := st["value"]; ok {
		t.Fatal("status leaked value")
	}
}
