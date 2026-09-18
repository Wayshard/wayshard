package validation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPassiveDiscoveryDoesNotExecute(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// a "test" script that would be dangerous if executed during discovery
	if err := os.WriteFile(filepath.Join(dir, "hack.sh"), []byte("#!/bin/sh\ntouch WAS_EXECUTED\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	checks := Discover(dir)
	if len(checks) == 0 {
		t.Fatal("expected go checks")
	}
	if _, err := os.Stat(filepath.Join(dir, "WAS_EXECUTED")); err == nil {
		t.Fatal("discovery executed project code")
	}
}

func TestPackageJSONScriptsDiscoveredNotRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"test":"touch RAN","lint":"echo hi"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	checks := Discover(dir)
	found := false
	for _, c := range checks {
		if c.Name == "npm-test" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected npm-test")
	}
	if _, err := os.Stat(filepath.Join(dir, "RAN")); err == nil {
		t.Fatal("executed npm test during discovery")
	}
}
