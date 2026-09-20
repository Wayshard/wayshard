package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// TestHarnessClosureForScriptHarness proves the launch closure for a Node-style
// script harness includes the package tree and the PATH-resolved interpreter,
// without granting the home directory.
func TestHarnessClosureForScriptHarness(t *testing.T) {
	root := t.TempDir()
	// A version-manager-like layout: <root>/bin/fakeinterp and a package under
	// <root>/lib/node_modules/@scope/pkg.
	interp := filepath.Join(root, "bin", "fakeinterp")
	writeFile(t, interp, "#!/bin/sh\nexit 0\n", 0o755)
	pkg := filepath.Join(root, "lib", "node_modules", "@scope", "pkg")
	writeFile(t, filepath.Join(pkg, "package.json"), `{"name":"pkg"}`, 0o644)
	script := filepath.Join(pkg, "dist", "index.js")
	writeFile(t, script, "#!/usr/bin/env fakeinterp\n", 0o755)

	c := harnessClosureFor(script)
	assertHasRoot(t, c.Roots, pkg)
	assertHasRoot(t, c.Roots, filepath.Dir(script))
	assertHasRoot(t, c.Roots, filepath.Dir(interp))
	if len(c.PathDirs) == 0 || c.PathDirs[0] != filepath.Dir(interp) {
		t.Fatalf("expected interpreter dir on PATH, got %v", c.PathDirs)
	}
	for _, r := range c.Roots {
		if r == root {
			t.Fatalf("closure must not grant the whole temp root: %v", c.Roots)
		}
	}
}

// TestHarnessClosureForNativeBinary proves a native executable gets only its own
// directory.
func TestHarnessClosureForNativeBinary(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "bin", "native")
	writeFile(t, exe, "\x7fELF-not-really", 0o755)
	c := harnessClosureFor(exe)
	if len(c.Roots) != 1 || c.Roots[0] != filepath.Dir(exe) {
		t.Fatalf("native closure roots = %v, want [%s]", c.Roots, filepath.Dir(exe))
	}
	if len(c.PathDirs) != 0 {
		t.Fatalf("native closure path dirs = %v, want none", c.PathDirs)
	}
}

func assertHasRoot(t *testing.T, roots []string, want string) {
	t.Helper()
	for _, r := range roots {
		if r == want {
			return
		}
	}
	t.Fatalf("root %q missing from %v", want, roots)
}
