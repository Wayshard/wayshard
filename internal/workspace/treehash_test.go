package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func mustWrite(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func buildBaseline(t *testing.T, root string) {
	t.Helper()
	mustWrite(t, filepath.Join(root, "a.txt"), "one", 0o644)
	mustWrite(t, filepath.Join(root, "bin", "run.sh"), "#!/bin/sh\n", 0o755)
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalTreeHashVectors(t *testing.T) {
	root := t.TempDir()
	buildBaseline(t, root)
	base, err := CanonicalTreeHash(root)
	if err != nil {
		t.Fatal(err)
	}
	if base == "" {
		t.Fatal("empty hash")
	}

	// Same logical tree at a different absolute path hashes identically.
	root2 := t.TempDir()
	buildBaseline(t, root2)
	again, err := CanonicalTreeHash(root2)
	if err != nil {
		t.Fatal(err)
	}
	if again != base {
		t.Fatalf("hash depends on absolute path: %s != %s", again, base)
	}

	check := func(name string, mutate func(root string)) {
		t.Helper()
		r := t.TempDir()
		buildBaseline(t, r)
		mutate(r)
		got, err := CanonicalTreeHash(r)
		if err != nil {
			t.Fatal(err)
		}
		if got == base {
			t.Fatalf("%s did not change tree hash", name)
		}
	}
	check("content change", func(r string) { mustWrite(t, filepath.Join(r, "a.txt"), "two", 0o644) })
	check("file add", func(r string) { mustWrite(t, filepath.Join(r, "new.txt"), "n", 0o644) })
	check("file remove", func(r string) { _ = os.Remove(filepath.Join(r, "a.txt")) })
	check("path change", func(r string) {
		_ = os.Rename(filepath.Join(r, "a.txt"), filepath.Join(r, "renamed.txt"))
	})
	if runtime.GOOS != "windows" {
		check("mode change", func(r string) { _ = os.Chmod(filepath.Join(r, "bin", "run.sh"), 0o644) })
	}
	check("symlink target", func(r string) {
		_ = os.Remove(filepath.Join(r, "link"))
		_ = os.Symlink("bin/run.sh", filepath.Join(r, "link"))
	})
	check("file becomes symlink", func(r string) {
		_ = os.Remove(filepath.Join(r, "a.txt"))
		_ = os.Symlink("bin/run.sh", filepath.Join(r, "a.txt"))
	})
	check("symlink becomes file", func(r string) {
		_ = os.Remove(filepath.Join(r, "link"))
		mustWrite(t, filepath.Join(r, "link"), "now a file", 0o644)
	})
	check("empty dir add", func(r string) { _ = os.MkdirAll(filepath.Join(r, "empty"), 0o755) })
}
