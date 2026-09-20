//go:build linux

package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func waitPid(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && n > 0 {
				return n
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pid file %s never appeared", path)
	return 0
}

// TestReconcileKillsOwnedDescendantsAndSparesUnrelated proves that token-based
// reconciliation terminates a daemonized (setsid) descendant of an owned
// process while leaving an unrelated same-shaped process untouched.
func TestReconcileKillsOwnedDescendantsAndSparesUnrelated(t *testing.T) {
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not installed")
	}
	dir := t.TempDir()
	token, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}

	// Owned parent that daemonizes a grandchild via setsid.
	ownedPidFile := filepath.Join(dir, "owned.pid")
	grandPidFile := filepath.Join(dir, "grand.pid")
	ownedScript := "setsid /bin/sh -c 'echo $$ > " + grandPidFile + "; exec sleep 300' & echo $! > " + ownedPidFile + "; wait"
	owned := exec.Command("/bin/sh", "-c", ownedScript)
	owned.Env = append(os.Environ(), TokenEnv+"="+token)
	if err := owned.Start(); err != nil {
		t.Fatal(err)
	}
	defer owned.Process.Kill()
	ownedPid := waitPid(t, ownedPidFile)
	grandPid := waitPid(t, grandPidFile)
	if !alive(ownedPid) || !alive(grandPid) {
		t.Fatalf("owned tree not running: owned=%d grand=%d", ownedPid, grandPid)
	}

	// Unrelated process with the same shape but no ownership token.
	unrelatedPidFile := filepath.Join(dir, "unrelated.pid")
	unrelatedScript := "echo $$ > " + unrelatedPidFile + "; exec sleep 300"
	unrelated := exec.Command("/bin/sh", "-c", unrelatedScript)
	unrelated.Env = os.Environ()
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer unrelated.Process.Kill()
	unrelatedPid := waitPid(t, unrelatedPidFile)

	observed, remaining, supported, err := ReconcileTokenHash(HashToken(token), 0, 5*time.Second)
	if err != nil || !supported {
		t.Fatalf("reconcile: err=%v supported=%v", err, supported)
	}
	if observed < 2 {
		t.Fatalf("expected at least 2 owned processes observed, got %d", observed)
	}
	if remaining != 0 {
		t.Fatalf("owned processes remain: %d", remaining)
	}
	if alive(ownedPid) {
		t.Errorf("owned parent survived reconciliation")
	}
	if alive(grandPid) {
		t.Errorf("setsid grandchild survived reconciliation")
	}
	if !alive(unrelatedPid) {
		t.Errorf("unrelated process was killed")
	}
}

// TestReconcileUnknownTokenIsNoop proves an unowned token does not kill anything.
func TestReconcileUnknownTokenIsNoop(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "p.pid")
	cmd := exec.Command("/bin/sh", "-c", "echo $$ > "+pidFile+"; exec sleep 300")
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	pid := waitPid(t, pidFile)

	unknown, _ := NewToken()
	if _, remaining, _, _ := ReconcileTokenHash(HashToken(unknown), 0, time.Second); remaining != 0 {
		t.Fatalf("unexpected remaining for unknown token: %d", remaining)
	}
	if !alive(pid) {
		t.Fatal("unrelated process killed by unknown token")
	}
}
