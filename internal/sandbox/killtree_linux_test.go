//go:build linux

package sandbox

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestCapabilityProbeTimeoutIsBounded proves a wedged capability probe cannot
// hang Compile/Report: runProbeTimeout force-kills it after the bound.
func TestCapabilityProbeTimeoutIsBounded(t *testing.T) {
	if _, err := exec.LookPath("/bin/sleep"); err != nil {
		t.Skip("sleep unavailable")
	}
	start := time.Now()
	if runProbeTimeout("/bin/sleep", nil, "300", 200*time.Millisecond) {
		t.Fatal("hanging probe unexpectedly succeeded")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("probe timeout not bounded: %s", elapsed)
	}
	if !runProbeTimeout("/bin/true", nil, "", 2*time.Second) {
		t.Fatal("fast probe should succeed")
	}
}

// openFDCount returns the number of open file descriptors for the test process.
func openFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("read /proc/self/fd: %v", err)
	}
	return len(entries)
}

// retainedDeathPipes returns how many supervisor death-pipe entries are still
// held in linuxDeathPipes.
func retainedDeathPipes() int {
	n := 0
	linuxDeathPipes.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// TestKillTreeAfterReapDoesNotSignalReusedPID proves KillTree is inert once the
// command has been reaped: a recycled PID/PGID must never be signaled, so an
// unrelated process can never be terminated.
func TestKillTreeAfterReapDoesNotSignalReusedPID(t *testing.T) {
	// A live, unrelated process stands in for a process that reused the PID.
	victim := exec.Command("/bin/sleep", "300")
	if err := victim.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	defer func() {
		_ = victim.Process.Kill()
		_, _ = victim.Process.Wait()
	}()

	// A command that has already run to completion and been reaped.
	reaped := exec.Command("/bin/true")
	if err := reaped.Run(); err != nil {
		t.Fatalf("run /bin/true: %v", err)
	}
	if reaped.ProcessState == nil {
		t.Fatal("expected ProcessState to be set after Run")
	}
	// Simulate PID reuse: the reaped command's Process handle now refers to the
	// live, unrelated process.
	reaped.Process = victim.Process

	if err := (LinuxBackend{}).KillTree(reaped); err != nil {
		t.Fatalf("KillTree after reap returned error: %v", err)
	}
	if err := syscall.Kill(victim.Process.Pid, 0); err != nil {
		t.Fatalf("KillTree signaled a live unrelated process after the command was reaped: %v", err)
	}
}

// TestSupervisorDeathPipeReleasedAfterAttach proves the production
// Start/Attach/Wait cycle releases the supervisor death pipe and its
// linuxDeathPipes entry, so repeated required launches retain neither FDs nor
// map entries.
func TestSupervisorDeathPipeReleasedAfterAttach(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	if !c.Report().Available {
		t.Skipf("required isolation unavailable: %s", c.Report().Detail)
	}
	ws := t.TempDir()
	home := t.TempDir()
	p := ToolPolicy(ws, home, NetNone)

	runOnce := func() {
		t.Helper()
		cmd := exec.Command("/bin/true")
		if err := c.Constrain(cmd, p); err != nil {
			t.Fatalf("constrain: %v", err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		cleanup, aerr := c.Attach(cmd, p)
		if aerr != nil {
			_ = c.KillTree(cmd)
			t.Fatalf("attach: %v", aerr)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("wait: %v", err)
		}
		// Mirror production: run the Attach cleanup after the command completes.
		if cleanup != nil {
			cleanup()
		}
	}

	runOnce() // warm up runtime caches before measuring
	entriesBefore := retainedDeathPipes()
	fdsBefore := openFDCount(t)
	for i := 0; i < 25; i++ {
		runOnce()
	}
	if after := retainedDeathPipes(); after > entriesBefore {
		t.Fatalf("retained death-pipe entries grew from %d to %d across 25 launches", entriesBefore, after)
	}
	if after := openFDCount(t); after > fdsBefore+8 {
		t.Fatalf("open FDs grew from %d to %d across 25 launches (supervisor death-pipe leak)", fdsBefore, after)
	}
}
