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
	if len(c.PathDirs) == 0 || resolvePath(c.PathDirs[0]) != resolvePath(filepath.Dir(interp)) {
		t.Fatalf("expected interpreter dir on PATH, got %v", c.PathDirs)
	}
	for _, r := range c.Roots {
		if resolvePath(r) == resolvePath(root) {
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
	if len(c.Roots) == 0 {
		t.Fatal("native closure had no roots")
	}
	wantDir := resolvePath(filepath.Dir(exe))
	for _, r := range c.Roots {
		if resolvePath(r) != wantDir {
			t.Fatalf("native closure root %q does not resolve to %q (roots=%v)", r, wantDir, c.Roots)
		}
	}
	if len(c.PathDirs) != 0 {
		t.Fatalf("native closure path dirs = %v, want none", c.PathDirs)
	}
}

// resolvePath resolves symlinks (macOS /var -> /private/var, Windows 8.3 names)
// so path comparisons are portable.
func resolvePath(p string) string {
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		return rp
	}
	return p
}

func assertHasRoot(t *testing.T, roots []string, want string) {
	t.Helper()
	wantResolved := resolvePath(want)
	for _, r := range roots {
		if r == want || resolvePath(r) == wantResolved {
			return
		}
	}
	t.Fatalf("root %q missing from %v", want, roots)
}
