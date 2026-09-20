//go:build linux

package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/testutil"
)

// serverEnvWith returns the current environment with the given KEY=value
// entries overriding any existing key (so PATH replacement actually takes
// effect).
func serverEnvWith(extra ...string) []string { return mergeEnv(os.Environ(), extra) }

// mergeEnv overlays override entries onto base, replacing the first occurrence
// of each key.
func mergeEnv(base, override []string) []string {
	out := append([]string{}, base...)
	index := map[string]int{}
	key := func(kv string) string {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			return kv[:i]
		}
		return kv
	}
	for i, kv := range out {
		if _, ok := index[key(kv)]; !ok {
			index[key(kv)] = i
		}
	}
	for _, kv := range override {
		k := key(kv)
		if i, ok := index[k]; ok {
			out[i] = kv
		} else {
			index[k] = len(out)
			out = append(out, kv)
		}
	}
	return out
}

// pgrepPids returns the pids whose full command line matches pattern.
func pgrepPids(pattern string) []int {
	out, _ := exec.Command("pgrep", "-f", pattern).Output()
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		var n int
		if _, err := fmt.Sscanf(f, "%d", &n); err == nil && n > 0 {
			pids = append(pids, n)
		}
	}
	return pids
}

// TestProcessBoundaryProbeDescendantReconciled proves that a discovery probe
// which daemonizes a setsid descendant survives a server SIGKILL only until the
// next server starts: startup reconciliation terminates the probe descendant by
// ownership token before discovery runs, while an unrelated same-shaped process
// survives.
func TestProcessBoundaryProbeDescendantReconciled(t *testing.T) {
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep not installed")
	}
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)
	fakeDir := filepath.Dir(fakeBin)
	marker := "WS_PROBE_DAEMON_" + strings.ReplaceAll(t.Name(), "/", "_")

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")

	// Unrelated same-shaped process with no ownership token: it must survive.
	unrelatedFile := filepath.Join(root, "unrelated.txt")
	unrelated := spawnUnrelated(t, unrelatedFile)
	t.Cleanup(func() { _ = syscall.Kill(unrelated, syscall.SIGKILL) })

	// Server A: discovery probe spawns a setsid descendant that inherits the
	// probe ownership token, then the probe hangs so the server can be killed
	// while the descendant is still alive.
	portA := freePort(t)
	logA := filepath.Join(root, "a.log")
	fa, err := os.Create(logA)
	if err != nil {
		t.Fatal(err)
	}
	origPATH := os.Getenv("PATH")
	cmdA := exec.Command(serverBin, "--listen", fmt.Sprintf("127.0.0.1:%d", portA), "--data", dataDir)
	cmdA.Env = serverEnvWith(
		"WAYSHARD_FAKE_PROBE_DAEMON="+marker,
		"WAYSHARD_FAKE_PROBE_DAEMON_HANG=1",
		"PATH="+fakeDir+string(os.PathListSeparator)+origPATH,
	)
	cmdA.Stdout = fa
	cmdA.Stderr = fa
	if err := cmdA.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmdA.Process != nil {
			_ = cmdA.Process.Kill()
			_ = cmdA.Wait()
		}
	})

	// Wait for the daemonized probe descendant. The window is short: the
	// version probe timeout is at most 3s.
	var probePids []int
	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) {
		probePids = pgrepPids(marker)
		if len(probePids) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(probePids) == 0 {
		b, _ := os.ReadFile(logA)
		t.Fatalf("probe descendant never appeared\n%s", b)
	}
	probePid := probePids[0]

	// The probe ownership record must be durable before the crash.
	withStore(t, dataDir, func(st *storage.Store) {
		active, _ := st.ListActiveProbeOwners(context.Background())
		if len(active) == 0 {
			t.Fatal("no active probe owner recorded before crash")
		}
	})

	// SIGKILL server A and confirm the descendant survived it.
	_ = cmdA.Process.Kill()
	_ = cmdA.Wait()
	time.Sleep(500 * time.Millisecond)
	if !procAlive(probePid) {
		t.Fatalf("probe descendant %d did not survive server SIGKILL", probePid)
	}
	t.Logf("probe crash path: server A=%d server B pending probe=%d unrelated=%d", cmdA.Process.Pid, probePid, unrelated)

	// Server B: no daemon knob, so discovery cannot spawn a replacement. Startup
	// reconciliation must terminate the surviving probe descendant.
	portB := freePort(t)
	envB := []string{"PATH=" + fakeDir + string(os.PathListSeparator) + origPATH}
	psB := startServer(t, serverBin, dataDir, portB, envB)
	defer psB.kill(t)

	waitProcGone(t, probePid, 15*time.Second)
	withStore(t, dataDir, func(st *storage.Store) {
		active, _ := st.ListActiveProbeOwners(context.Background())
		if len(active) != 0 {
			t.Fatalf("probe ownership not reconciled: %+v", active)
		}
	})

	// The unrelated process must be untouched by probe reconciliation.
	if !procAlive(unrelated) {
		t.Fatal("unrelated process was killed by probe reconciliation")
	}
	before := fileSize(unrelatedFile)
	time.Sleep(700 * time.Millisecond)
	if after := fileSize(unrelatedFile); after <= before {
		t.Fatalf("unrelated process stopped writing: %d -> %d", before, after)
	}
}
