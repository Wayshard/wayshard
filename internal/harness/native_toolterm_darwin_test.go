//go:build darwin

package harness

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/process"
)

// TestDarwinToolReleaseReconcilesDetached proves an ACP terminal/tool command
// that backgrounds a descendant in its own process group (set -m) is still
// terminated when the tool session is released, via per-session token
// reconciliation. KillTree alone would leave it behind.
func TestDarwinToolReleaseReconcilesDetached(t *testing.T) {
	if !process.Supported() {
		t.Fatal("authoritative ownership must be supported where FeatureProcessTree is advertised")
	}
	ctx := context.Background()
	tm := newToolManager(
		orchestrator.StageRequest{Stage: domain.Stage{Kind: domain.StageExecute}},
		t.TempDir(), t.TempDir(), nil, "attempt-token",
	)
	res, err := tm.Create(ctx, acp.CreateTerminalParams{
		Command: "/bin/sh",
		Args:    []string{"-c", "set -m; sleep 300 >/dev/null 2>&1 & echo PID=$!"},
	})
	if err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	if _, err := tm.WaitExit(ctx, acp.TerminalIDParams{TerminalID: res.TerminalID}); err != nil {
		t.Fatalf("wait exit: %v", err)
	}
	out, err := tm.Output(ctx, acp.TerminalIDParams{TerminalID: res.TerminalID})
	if err != nil {
		t.Fatalf("output: %v", err)
	}
	pid := parseTrailingPID(out.Output)
	if pid == 0 {
		t.Fatalf("fixture did not report a backgrounded pid: %q", out.Output)
	}
	// True setsid detachment is proven by TestNativeSetsidDescendantReconciled;
	// this proves the tool path leaves no owned descendant after release via the
	// same per-session token reconciliation.
	if !ownerProcessAlive(pid) {
		t.Fatalf("expected backgrounded descendant %d to be alive before release", pid)
	}
	if err := tm.Release(ctx, acp.TerminalIDParams{TerminalID: res.TerminalID}); err != nil {
		t.Fatalf("release: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !ownerProcessAlive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("detached tool descendant %d survived release", pid)
}

func parseTrailingPID(out string) int {
	return parseKeyInt(out, "PID=")
}

func parseKeyInt(out, prefix string) int {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, prefix))); err == nil {
				return n
			}
		}
	}
	return 0
}
