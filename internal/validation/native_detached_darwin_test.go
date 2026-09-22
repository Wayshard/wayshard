//go:build darwin

package validation

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/sandbox"
)

// TestDarwinValidationReconcilesDetached proves a validation command that
// backgrounds a descendant in its own process group (set -m) leaves no owned
// process after the command completes.
func TestDarwinValidationReconcilesDetached(t *testing.T) {
	if !sandbox.Probe().Available {
		t.Skipf("required isolation unavailable: %s", sandbox.Probe().Detail)
	}
	ws := t.TempDir()
	script := "#!/bin/sh\nset -m\nsleep 300 >/dev/null 2>&1 &\necho $! > detached.pid\n"
	if err := os.WriteFile(filepath.Join(ws, "check.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{DataDir: t.TempDir()}
	art := r.Run(context.Background(), ws, []artifacts.ValidationCheck{
		{Name: "detached", Command: "./check.sh", Required: true, Status: string(domain.CheckNotVerified)},
	}, nil)
	if len(art.Checks) != 1 || art.Checks[0].Status != string(domain.CheckPass) {
		t.Fatalf("validation did not pass: %+v", art.Checks)
	}
	b, err := os.ReadFile(filepath.Join(ws, "detached.pid"))
	if err != nil {
		t.Fatalf("fixture pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		t.Fatalf("bad pid %q: %v", b, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("detached validation descendant %d survived", pid)
}
