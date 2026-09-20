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
	home := t.TempDir()
	req := orchestrator.StageRequest{Stage: domain.Stage{Kind: kind}}
	return newToolManager(req, ws, home, nil, ""), ws
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

// TestToolRunsInSandbox proves ACP tool commands execute in the run workspace
// under NetworkNone with an allowlisted environment.
func TestToolRunsInSandbox(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID_CANARY", "secret-canary")
	tm, ws := newTestToolManager(t, domain.StageExecute)

	out := runTool(t, tm, "/bin/sh", "-c", "echo hello; pwd; echo X=$AWS_ACCESS_KEY_ID_CANARY")
	if !strings.Contains(out, "hello") {
		t.Fatalf("tool did not run: %q", out)
	}
	if !strings.Contains(out, ws) {
		t.Fatalf("tool cwd not run workspace: %q (want %s)", out, ws)
	}
	if strings.Contains(out, "secret-canary") {
		t.Fatalf("ambient secret leaked into tool env: %q", out)
	}

	net := runTool(t, tm, "/usr/bin/python3", "-c", `import socket
try:
 socket.socket(socket.AF_INET, socket.SOCK_STREAM); print("NET=OK")
except OSError as e: print("NET=ERRNO%d"%e.errno)`)
	if !strings.Contains(net, "NET=ERRNO1") {
		t.Fatalf("tool network not denied: %q", net)
	}
}

// TestToolWritePermission proves read-only stages reject tool writes and write
// stages allow workspace writes.
func TestToolWritePermission(t *testing.T) {
	ro, _ := newTestToolManager(t, domain.StageReview)
	writeTool := runTool(t, ro, "/bin/sh", "-c", "echo x > blocked.txt 2>&1; echo done")
	if !strings.Contains(writeTool, "Permission denied") {
		t.Fatalf("read-only stage tool write was not denied: %q", writeTool)
	}

	rw, ws := newTestToolManager(t, domain.StageExecute)
	runTool(t, rw, "/bin/sh", "-c", "echo x > allowed.txt")
	if _, err := os.Stat(filepath.Join(ws, "allowed.txt")); err != nil {
		t.Fatalf("write-stage tool could not write workspace: %v", err)
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
