//go:build linux

package validation

import (
	"context"
	"os"
	"testing"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
)

// TestValidationCommandsDoNotLeakSupervisorPipes proves repeated validation
// commands through the production Runner release the supervisor death pipe: open
// FDs stay flat instead of growing ~2 per command (the read end and the write
// end retained in linuxDeathPipes when Attach is skipped).
func TestValidationCommandsDoNotLeakSupervisorPipes(t *testing.T) {
	requirePosixSandbox(t)
	ws := t.TempDir()
	r := &Runner{DataDir: t.TempDir()}

	runOnce := func() {
		t.Helper()
		art := r.Run(context.Background(), ws, []artifacts.ValidationCheck{
			{Name: "true", Command: "true", Required: true, Status: string(domain.CheckNotVerified)},
		}, nil)
		if len(art.Checks) != 1 || art.Checks[0].Status != string(domain.CheckPass) {
			t.Fatalf("validation command did not pass: %+v", art.Checks)
		}
	}
	fdCount := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatalf("read /proc/self/fd: %v", err)
		}
		return len(entries)
	}

	runOnce() // warm up runtime caches before measuring
	before := fdCount()
	for i := 0; i < 40; i++ {
		runOnce()
	}
	after := fdCount()
	// A leak would add ~2 FDs per command (80+ here); allow small runtime jitter.
	if after > before+8 {
		t.Fatalf("open FDs grew from %d to %d across 40 validation commands (supervisor death-pipe leak)", before, after)
	}
}
