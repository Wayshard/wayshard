package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeExecutable creates a small executable stand-in at path.
func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// resolvedPath returns the canonical physical path so assertions are stable on
// platforms whose temp directory is itself a symlink (for example macOS /var).
func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	return resolved
}

// A. A directly executed binary resolves the companion beside itself.
func TestResolveTUIDirectExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "wayshard")
	companion := filepath.Join(dir, tuiCompanionName())
	writeExecutable(t, exe)
	writeExecutable(t, companion)

	got, err := resolveTUIFor(exe)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedPath(t, got) != resolvedPath(t, companion) {
		t.Fatalf("resolved %q, want companion %q", got, companion)
	}
}

// A missing companion fails clearly instead of silently searching elsewhere.
func TestResolveTUIMissingCompanion(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "wayshard")
	writeExecutable(t, exe)

	if _, err := resolveTUIFor(exe); err == nil {
		t.Fatal("expected an error when the companion is absent")
	} else if !strings.Contains(err.Error(), "Wayshard TUI not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// F. An ordinary PATH search remains forbidden: a companion that only exists on
// PATH is not found.
func TestResolveTUIDoesNotSearchPATH(t *testing.T) {
	installdir := t.TempDir()
	pathdir := t.TempDir()
	exe := filepath.Join(installdir, "wayshard")
	writeExecutable(t, exe)
	writeExecutable(t, filepath.Join(pathdir, tuiCompanionName()))
	t.Setenv("PATH", pathdir)

	if _, err := resolveTUIFor(exe); err == nil {
		t.Fatal("resolved a companion through PATH; PATH lookup must be forbidden")
	}
}

// The explicit WAYSHARD_TUI override keeps its designed semantics (absolute
// path used directly, relative path resolved beside the real executable).
func TestResolveTUIOverrideSemantics(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	exe := filepath.Join(dir, "wayshard")
	writeExecutable(t, exe)

	absolute := filepath.Join(other, "custom-tui")
	writeExecutable(t, absolute)
	t.Setenv("WAYSHARD_TUI", absolute)
	got, err := resolveTUIFor(exe)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedPath(t, got) != resolvedPath(t, absolute) {
		t.Fatalf("absolute override resolved %q, want %q", got, absolute)
	}

	relative := "relative-tui"
	writeExecutable(t, filepath.Join(dir, relative))
	t.Setenv("WAYSHARD_TUI", relative)
	got, err = resolveTUIFor(exe)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedPath(t, got) != resolvedPath(t, filepath.Join(dir, relative)) {
		t.Fatalf("relative override resolved %q", got)
	}
}

// A non-regular companion is refused.
func TestResolveTUIRejectsNonRegularCompanion(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "wayshard")
	writeExecutable(t, exe)
	if err := os.Mkdir(filepath.Join(dir, tuiCompanionName()), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTUIFor(exe); err == nil {
		t.Fatal("expected a non-regular companion to be refused")
	}
}
