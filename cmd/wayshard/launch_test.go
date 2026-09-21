//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOldInteractiveLoopGone proves the handwritten `wayshard>` prompt and its
// runTUI function cannot silently return to the production CLI.
func TestOldInteractiveLoopGone(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, forbidden := range []string{"wayshard>", "func runTUI", "Interactive TUI talks to"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("main.go still contains retired interactive CLI artifact %q", forbidden)
		}
	}
	if !strings.Contains(text, "launchTUI(") {
		t.Fatal("main.go no longer launches the packaged TUI on no-argument invocation")
	}
}

func buildCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "wayshard")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build wayshard: %v", err)
	}
	return bin
}

// TestNoArgLaunchFailsClearlyWithoutCompanion proves that a missing packaged TUI
// yields a clear error instead of the retired prompt.
func TestNoArgLaunchFailsClearlyWithoutCompanion(t *testing.T) {
	bin := buildCLI(t)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "WAYSHARD_TUI=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit when the TUI companion is missing, got output: %s", out)
	}
	if !strings.Contains(string(out), "Wayshard TUI not found") {
		t.Fatalf("missing clear companion-missing error: %s", out)
	}
	if strings.Contains(string(out), "wayshard>") {
		t.Fatal("retired interactive prompt reappeared")
	}
}

// TestNoArgLaunchExecsAdjacentCompanion proves no-argument invocation hands off
// to the adjacent packaged TUI by explicit path (no PATH search).
func TestNoArgLaunchExecsAdjacentCompanion(t *testing.T) {
	bin := buildCLI(t)
	dir := filepath.Dir(bin)
	marker := filepath.Join(dir, "tui-ran.txt")
	companion := filepath.Join(dir, "wayshard-tui")
	script := "#!/bin/sh\nprintf '%s' \"$WAYSHARD_URL|$WAYSHARD_TOKEN\" > " + marker + "\n"
	if err := os.WriteFile(companion, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--server", "http://example.test:7420", "--token", "tok")
	cmd.Env = append(os.Environ(), "WAYSHARD_URL=", "WAYSHARD_TOKEN=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch companion: %v (%s)", err, out)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("companion did not run: %v", err)
	}
	if string(got) != "http://example.test:7420|tok" {
		t.Fatalf("companion env = %q", string(got))
	}
}
