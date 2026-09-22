//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/process"
)

// runDetachedHelper implements the setsid fixture modes. The "setsid" root
// spawns a child in its own session (escaping the root's process group); the
// child prints its own PID and a grandchild PID, both of which inherit
// WAYSHARD_OWNER_TOKEN. The root then stays alive so cancellation can run.
func runDetachedHelper(mode string) {
	switch mode {
	case "setsid":
		c := exec.Command(os.Args[0], "-test.run=TestSandboxHelperProcess")
		c.Env = append(os.Environ(), helperEnv+"=1", "SANDBOX_HELPER_MODE=setsid-child")
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := c.Start(); err != nil {
			fmt.Printf("SPAWN_ERR=%v\n", err)
			os.Exit(4)
		}
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	case "setsid-child":
		fmt.Printf("PID=%d\n", os.Getpid())
		g := exec.Command(os.Args[0], "-test.run=TestSandboxHelperProcess")
		g.Env = append(os.Environ(), helperEnv+"=1", "SANDBOX_HELPER_MODE=sleep")
		if err := g.Start(); err != nil {
			fmt.Printf("SPAWN_ERR=%v\n", err)
			os.Exit(4)
		}
		fmt.Printf("PID=%d\n", g.Process.Pid)
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	}
	os.Exit(2)
}

// TestNativeSetsidDescendantReconciled is the mandatory adversarial macOS test:
// a setsid descendant escapes the launch process group, and authoritative
// token-based ownership must still find and terminate it. It ties the advertised
// FeatureProcessTree to real behavior.
func TestNativeSetsidDescendantReconciled(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skipf("required isolation unavailable: %s", c.Report().Detail)
	}
	if !process.Supported() {
		t.Fatalf("FeatureProcessTree is advertised but authoritative ownership is unsupported")
	}
	token, err := process.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	tokenHash := process.HashToken(token)

	ws := t.TempDir()
	home := t.TempDir()
	p := sandboxTestPolicy(t, ws, home)

	cmd := exec.Command(helperExe(t), "-test.run=TestSandboxHelperProcess")
	cmd.Env = append(helperEnvFor("setsid", "1", ToolEnv(home, home, nil)), process.TokenEnv+"="+token)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := c.Constrain(cmd, p); err != nil {
		t.Fatalf("constrain: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	clean, err := c.Attach(cmd, p)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("attach: %v", err)
	}
	defer clean()
	root := cmd.Process.Pid

	detached := waitForPIDs(t, &stdout, 2)
	all := append([]int{root}, detached...)
	for _, pid := range all {
		if !processAlive(pid) {
			t.Fatalf("pid %d should be alive before cancellation", pid)
		}
	}

	// KillTree only terminates the launch process group; the setsid child and its
	// grandchild have escaped it and must be found by token.
	_ = c.KillTree(cmd)
	observed, remaining, supported, _ := process.ReconcileTokenHash(tokenHash, root, 15*time.Second)
	if !supported {
		t.Fatalf("ownership must be supported for this test")
	}
	if remaining != 0 {
		t.Fatalf("setsid descendant survived: observed=%d remaining=%d pids=%v", observed, remaining, detached)
	}
	_ = cmd.Wait()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		alive := false
		for _, pid := range all {
			if processAlive(pid) {
				alive = true
				break
			}
		}
		if !alive {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("a process survived cancellation: root=%d detached=%v", root, detached)
}

// TestNativeCancellationDoesNotKillUnrelated proves token-based reconciliation
// never kills an unrelated process (PID-reuse safety): an unrelated helper that
// does not carry the token must survive.
func TestNativeCancellationDoesNotKillUnrelated(t *testing.T) {
	if !process.Supported() {
		t.Skip("authoritative ownership unsupported")
	}
	unrelated := exec.Command(helperExe(t), "-test.run=TestSandboxHelperProcess")
	unrelated.Env = helperEnvFor("sleep", "", nil)
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unrelated.Process.Kill() }()

	// Reconcile with a token hash that no process carries.
	_, remaining, supported, _ := process.ReconcileTokenHash(process.HashToken("no-such-token"), 0, 2*time.Second)
	if !supported {
		t.Fatal("ownership must be supported")
	}
	if remaining != 0 {
		t.Fatalf("unexpected remaining=%d", remaining)
	}
	if !processAlive(unrelated.Process.Pid) {
		t.Fatalf("unrelated process was killed by reconciliation")
	}
}
