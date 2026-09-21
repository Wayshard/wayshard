//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// B/C. A symlinked executable resolves to its real install directory, and the
// companion beside the real executable is found.
func TestResolveTUISymlinkedExecutableUsesRealInstallDir(t *testing.T) {
	real := t.TempDir()
	linkdir := t.TempDir()
	realExe := filepath.Join(real, "wayshard")
	companion := filepath.Join(real, tuiCompanionName())
	writeExecutable(t, realExe)
	writeExecutable(t, companion)

	link := filepath.Join(linkdir, "wayshard")
	if err := os.Symlink(realExe, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := resolveTUIFor(link)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedPath(t, got) != resolvedPath(t, companion) {
		t.Fatalf("symlinked launcher resolved %q, want %q", got, companion)
	}
}

// E. A companion symlink escaping the install directory remains rejected.
func TestResolveTUIRejectsEscapingCompanionSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	exe := filepath.Join(dir, "wayshard")
	writeExecutable(t, exe)
	target := filepath.Join(outside, "wayward-tui")
	writeExecutable(t, target)

	if err := os.Symlink(target, filepath.Join(dir, tuiCompanionName())); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := resolveTUIFor(exe); err == nil {
		t.Fatal("escaping companion symlink was accepted")
	} else if !strings.Contains(err.Error(), "outside the install directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A companion symlink that stays inside the install directory is accepted.
func TestResolveTUIAcceptsCompanionSymlinkWithinInstallDir(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "wayshard")
	writeExecutable(t, exe)
	realCompanion := filepath.Join(dir, "wayshard-tui-real")
	writeExecutable(t, realCompanion)

	link := filepath.Join(dir, tuiCompanionName())
	if err := os.Symlink(realCompanion, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := resolveTUIFor(exe)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedPath(t, got) != resolvedPath(t, realCompanion) {
		t.Fatalf("resolved %q, want %q", got, realCompanion)
	}
}

// D. A symlinked launcher successfully launches the companion beside the real
// executable.
func TestNoArgLaunchThroughSymlinkedExecutable(t *testing.T) {
	bin := buildCLI(t)
	realdir := filepath.Dir(bin)
	marker := filepath.Join(realdir, "tui-ran.txt")
	companion := filepath.Join(realdir, "wayshard-tui")
	script := "#!/bin/sh\nprintf '%s' \"$WAYSHARD_URL|$WAYSHARD_TOKEN\" > " + marker + "\n"
	if err := os.WriteFile(companion, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	linkdir := t.TempDir()
	link := filepath.Join(linkdir, "wayshard")
	if err := os.Symlink(bin, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	cmd := exec.Command(link, "--server", "http://example.test:7420", "--token", "tok")
	cmd.Env = append(os.Environ(), "WAYSHARD_URL=", "WAYSHARD_TOKEN=", "WAYSHARD_TUI=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch companion through symlinked executable: %v (%s)", err, out)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("companion did not run: %v", err)
	}
	if string(got) != "http://example.test:7420|tok" {
		t.Fatalf("companion env = %q", string(got))
	}
}
