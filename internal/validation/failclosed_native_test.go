//go:build darwin || windows

package validation

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/sandbox"
)

// TestNativeValidationFailsClosed proves a required validation command is
// BLOCKED before execution on platforms without a non-removable process-tree
// ownership boundary (macOS, Windows): the marker file is never created.
func TestNativeValidationFailsClosed(t *testing.T) {
	if sandbox.Probe().Available {
		t.Skipf("required isolation is available on %s; the execution path is covered elsewhere", runtime.GOOS)
	}
	ws := t.TempDir()
	marker := filepath.Join(ws, "executed.marker")
	script := "#!/bin/sh\n: > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(ws, "check.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{DataDir: t.TempDir()}
	art := r.Run(context.Background(), ws, []artifacts.ValidationCheck{
		{Name: "blocked", Command: "./check.sh", Required: true, Status: string(domain.CheckNotVerified)},
	}, nil)
	if len(art.Checks) != 1 {
		t.Fatalf("checks: %+v", art.Checks)
	}
	if art.Checks[0].Status != string(domain.CheckBlocked) {
		t.Fatalf("required validation must be BLOCKED, got %q", art.Checks[0].Status)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("validation command executed before containment was established")
	}
}
