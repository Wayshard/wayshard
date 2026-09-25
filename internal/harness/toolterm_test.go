//go:build linux

package harness

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
)

func newTestToolManager(t *testing.T, kind domain.StageKind) (*toolManager, string) {
	t.Helper()
	ws := t.TempDir()
	req := orchestrator.StageRequest{Stage: domain.Stage{Kind: kind}}
	return newToolManager(req, ws), ws
}

func runTool(t *testing.T, tm *toolManager, cmd string, args ...string) string {
	t.Helper()
	ctx := context.Background()
	res, err := tm.Create(ctx, acp.CreateTerminalParams{Command: cmd, Args: args})
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
	return out.Output
}

// TestToolRunsInWorkspace proves ACP tool commands execute in the run workspace
// as the server OS user with the server environment.
func TestToolRunsInWorkspace(t *testing.T) {
	t.Setenv("WAYSHARD_TOOL_CANARY", "inherited")
	tm, ws := newTestToolManager(t, domain.StageExecute)

	out := runTool(t, tm, "/bin/sh", "-c", "echo hello; pwd; echo X=$WAYSHARD_TOOL_CANARY")
	if !strings.Contains(out, "hello") {
		t.Fatalf("tool did not run: %q", out)
	}
	if !strings.Contains(out, ws) {
		t.Fatalf("tool cwd not run workspace: %q (want %s)", out, ws)
	}
	if !strings.Contains(out, "X=inherited") {
		t.Fatalf("tool did not inherit the server environment: %q", out)
	}
}

// TestToolWritesWorkspace proves a tool command can write inside the run
// workspace. Tool commands run as the server OS user; read-only stage intent is
// enforced at the ACP file-callback layer, not by an OS sandbox.
func TestToolWritesWorkspace(t *testing.T) {
	rw, ws := newTestToolManager(t, domain.StageExecute)
	runTool(t, rw, "/bin/sh", "-c", "echo x > allowed.txt")
	if _, err := os.Stat(filepath.Join(ws, "allowed.txt")); err != nil {
		t.Fatalf("tool could not write workspace: %v", err)
	}
}

// TestToolCancelKillsDescendants proves Kill terminates the whole tool tree.
func TestToolCancelKillsDescendants(t *testing.T) {
	tm, ws := newTestToolManager(t, domain.StageExecute)
	ctx := context.Background()
	res, err := tm.Create(ctx, acp.CreateTerminalParams{
		Command: "/bin/sh",
		Args:    []string{"-c", "sleep 300 >/dev/null 2>&1 & echo $! > gc.pid; wait"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var pid string
	for i := 0; i < 200; i++ {
		if b, err := os.ReadFile(filepath.Join(ws, "gc.pid")); err == nil {
			pid = strings.TrimSpace(string(b))
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if pid == "" {
		t.Fatal("descendant never started")
	}
	if err := tm.Kill(ctx, acp.TerminalIDParams{TerminalID: res.TerminalID}); err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(pid)
	for i := 0; i < 50; i++ {
		if err := syscall.Kill(n, 0); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("descendant %s survived tool cancellation", pid)
}
