//go:build linux

package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/testutil"
)

// readPid reads a pid recorded by a fixture helper.
func readPid(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && n > 0 {
				return n
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	return 0
}

// orphanToolScript backgrounds a writer that outlives the server. The
// backgrounded subshell inherits the attempt ownership token, so startup
// reconciliation must terminate it.
const orphanToolScript = `RUN=$(pwd); ( while true; do echo orphan >> "$RUN/orphan.txt"; echo home >> "$HOME/home-orphan.txt"; sleep 0.2; done ) & echo $! > "$RUN/orphan.pid"; wait`

func procAlive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }

func waitProcGone(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !procAlive(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("process %d still alive after %s", pid, timeout)
}

func fileSize(p string) int {
	fi, err := os.Stat(p)
	if err != nil {
		return -1
	}
	return int(fi.Size())
}

func orphanFixture(t *testing.T, root, fakeBin string) (fixtureEnv, []string) {
	t.Helper()
	signalDir := filepath.Join(root, "signal")
	_ = os.MkdirAll(signalDir, 0o755)
	fx := fixtureEnv{
		fakeBin:     fakeBin,
		signalDir:   signalDir,
		signalFile:  filepath.Join(signalDir, "execute.signal"),
		markerFile:  filepath.Join(signalDir, "success.marker"),
		hangStage:   "execute",
		partialFile: "partial.txt",
		trackedFile: "tracked.txt",
		toolFile:    "tool-partial.txt",
	}
	return fx, append(fx.serverEnv(), "WAYSHARD_FAKE_TOOL_CMD="+orphanToolScript)
}

// TestProcessBoundaryOrphanGrandchildReconciled proves the trusted PID-namespace
// supervisor tears a backgrounded grandchild down when the server dies (the
// death pipe closes), so no owned writer survives to race workspace restore.
func TestProcessBoundaryOrphanGrandchildReconciled(t *testing.T) {
	requireGit(t)
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"tracked.txt": "source-original", "keep.go": "package keep\n"})
	fx, env := orphanFixture(t, root, fakeBin)

	portA := freePort(t)
	psA := startServer(t, serverBin, dataDir, portA, env)
	cred := pairProcessServer(t, psA)
	proj := openProjectViaAPI(t, psA, cred, src, "orphan")
	conv := createConversation(t, psA, cred, proj)
	runID := sendMessage(t, psA, cred, conv, "add agent.go")
	waitFileRun(t, psA, cred, runID, fx.signalFile, 40*time.Second)
	rw := runWorkspacePath(dataDir, runID)

	orphanFile := filepath.Join(rw, "orphan.txt")
	if s := fileSize(orphanFile); s <= 0 {
		t.Fatalf("orphan writer not active before crash: %d", s)
	}

	// SIGKILL server A. The PID-namespace supervisor holds the read end of a
	// death pipe whose write end is owned by the server, so server death tears
	// the owned tree down: the backgrounded grandchild does not survive and its
	// writer stops.
	psA.kill(t)
	time.Sleep(1500 * time.Millisecond)
	stopped0 := fileSize(orphanFile)
	time.Sleep(700 * time.Millisecond)
	if after := fileSize(orphanFile); after != stopped0 {
		t.Fatalf("owned writer survived server SIGKILL: %d -> %d", stopped0, after)
	}

	_ = os.WriteFile(fx.markerFile, []byte("go\n"), 0o644)
	portB := freePort(t)
	psB := startServer(t, serverBin, dataDir, portB, env)
	if psA.pid() == psB.pid() {
		t.Fatalf("server PIDs not distinct: %d", psA.pid())
	}
	t.Logf("orphan success path: server A=%d server B=%d", psA.pid(), psB.pid())

	// Recovery restores the pre-attempt checkpoint; the torn-down writer cannot
	// keep writing.
	stopped := fileSize(orphanFile)
	time.Sleep(700 * time.Millisecond)
	if after := fileSize(orphanFile); after != stopped {
		t.Fatalf("orphan kept writing after recovery: %d -> %d", stopped, after)
	}

	final := waitRun(t, psB, cred, runID, func(r domain.Run) bool {
		return r.Status.Terminal() || r.Status == domain.RunIntegrationBlocked
	}, 90*time.Second)
	if final.Status != domain.RunComplete {
		t.Fatalf("run did not complete: %s %s\n%s", final.Status, final.BlockedDetail, psB.logs(t))
	}
	if _, err := os.Stat(filepath.Join(rw, "orphan.txt")); err == nil {
		t.Fatal("orphan debris survived checkpoint restore")
	}
	psB.kill(t)

	// Synthetic HOME/TEMP is per attempt: the retry cannot inherit the orphan's
	// old HOME.
	withStore(t, dataDir, func(st *storage.Store) {
		ctx := context.Background()
		stages, _ := st.ListStages(ctx, runID)
		var a1, a2 string
		for _, stg := range stages {
			if stg.Kind != domain.StageExecute {
				continue
			}
			atts, _ := st.ListAttempts(ctx, stg.ID)
			for _, a := range atts {
				if a.Status == domain.AttemptInterrupted {
					a1 = a.ID
				}
				if a.Status == domain.AttemptSucceeded {
					a2 = a.ID
				}
			}
		}
		if a1 == "" || a2 == "" || a1 == a2 {
			t.Fatalf("attempt lineage wrong: interrupted=%q succeeded=%q", a1, a2)
		}
		home1 := filepath.Join(dataDir, "runtime", "sandbox", runID, a1, "home")
		home2 := filepath.Join(dataDir, "runtime", "sandbox", runID, a2, "home")
		if home1 == home2 {
			t.Fatal("synthetic HOME is shared across attempts")
		}
		if _, err := os.Stat(filepath.Join(home2, "home-orphan.txt")); err == nil {
			t.Fatal("attempt 2 synthetic HOME contains attempt 1 orphan data")
		}
	})
}

// TestProcessBoundaryMultipleRunOrphans proves independent run process trees are
// reconciled without cross-killing.
func TestProcessBoundaryMultipleRunOrphans(t *testing.T) {
	requireGit(t)
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"tracked.txt": "source-original", "keep.go": "package keep\n"})
	fx, env := orphanFixture(t, root, fakeBin)

	portA := freePort(t)
	psA := startServer(t, serverBin, dataDir, portA, env)
	cred := pairProcessServer(t, psA)
	proj := openProjectViaAPI(t, psA, cred, src, "multi")
	conv1 := createConversation(t, psA, cred, proj)
	conv2 := createConversation(t, psA, cred, proj)
	run1 := sendMessage(t, psA, cred, conv1, "add agent.go")
	run2 := sendMessage(t, psA, cred, conv2, "add agent.go")

	rw1 := runWorkspacePath(dataDir, run1)
	rw2 := runWorkspacePath(dataDir, run2)
	// Wait for both backgrounded writers to start (each writes orphan.pid).
	_ = readPid(t, filepath.Join(rw1, "orphan.pid"))
	_ = readPid(t, filepath.Join(rw2, "orphan.pid"))

	// Server death tears both owned trees down: each run's PID-namespace
	// supervisor loses its death pipe when the server dies.
	psA.kill(t)
	time.Sleep(1500 * time.Millisecond)
	_ = os.WriteFile(fx.markerFile, []byte("go\n"), 0o644)
	portB := freePort(t)
	psB := startServer(t, serverBin, dataDir, portB, env)
	defer psB.kill(t)
	t.Logf("multi-run: A=%d B=%d", psA.pid(), psB.pid())

	// Both writers are gone, so neither can keep mutating its workspace.
	for _, rw := range []string{rw1, rw2} {
		orphanFile := filepath.Join(rw, "orphan.txt")
		stopped := fileSize(orphanFile)
		time.Sleep(500 * time.Millisecond)
		if after := fileSize(orphanFile); after != stopped {
			t.Fatalf("owned writer kept running in %s: %d -> %d", rw, stopped, after)
		}
	}

	// Both workspaces were restored by recovery.
	for _, rw := range []string{rw1, rw2} {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(rw, "partial.txt")); os.IsNotExist(err) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if _, err := os.Stat(filepath.Join(rw, "partial.txt")); err == nil {
			t.Fatalf("workspace %s not restored", rw)
		}
	}
}

// TestProcessBoundaryOrphanBlockedRecovery proves an owned writer is torn down
// on server death and cannot keep mutating the workspace even when checkpoint
// recovery fails and the run becomes BLOCKED/RECOVERY.
func TestProcessBoundaryOrphanBlockedRecovery(t *testing.T) {
	requireGit(t)
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"tracked.txt": "source-original", "keep.go": "package keep\n"})
	fx, env := orphanFixture(t, root, fakeBin)

	portA := freePort(t)
	psA := startServer(t, serverBin, dataDir, portA, env)
	cred := pairProcessServer(t, psA)
	proj := openProjectViaAPI(t, psA, cred, src, "orphanblock")
	conv := createConversation(t, psA, cred, proj)
	runID := sendMessage(t, psA, cred, conv, "add agent.go")
	waitFileRun(t, psA, cred, runID, fx.signalFile, 40*time.Second)
	rw := runWorkspacePath(dataDir, runID)
	psA.kill(t)
	time.Sleep(1500 * time.Millisecond)
	orphanFile := filepath.Join(rw, "orphan.txt")
	stopped0 := fileSize(orphanFile)
	time.Sleep(700 * time.Millisecond)
	if after := fileSize(orphanFile); after != stopped0 {
		t.Fatalf("owned writer survived server SIGKILL: %d -> %d", stopped0, after)
	}

	// Corrupt the checkpoint so recovery blocks and does not swap the workspace.
	withStore(t, dataDir, func(st *storage.Store) {
		ctx := context.Background()
		cps, _ := st.ListCheckpointsByRun(ctx, runID)
		if len(cps) == 0 {
			t.Fatal("no checkpoint")
		}
		_ = os.WriteFile(filepath.Join(cps[0].TreePath, "tree", "tracked.txt"), []byte("corrupted"), 0o644)
	})
	_ = os.WriteFile(fx.markerFile, []byte("go\n"), 0o644)

	portB := freePort(t)
	psB := startServer(t, serverBin, dataDir, portB, env)
	t.Logf("orphan blocked path: server A=%d server B=%d", psA.pid(), psB.pid())

	stopped := fileSize(orphanFile)
	time.Sleep(700 * time.Millisecond)
	if after := fileSize(orphanFile); after != stopped {
		t.Fatalf("orphan kept writing blocked workspace: %d -> %d", stopped, after)
	}

	run := getRunStatus(t, psB, cred, runID)
	if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedRecovery {
		t.Fatalf("expected blocked/RECOVERY, got %s %s", run.Status, run.BlockedReason)
	}
	// The partial workspace is untrusted and was not swapped.
	if got, _ := os.ReadFile(filepath.Join(rw, "tracked.txt")); string(got) != "partial-execute\n" {
		t.Fatalf("blocked workspace unexpectedly restored: %q", got)
	}
	psB.kill(t)
}

// TestProcessBoundaryUnrelatedProcessSurvives proves reconciliation only kills
// processes carrying an owned token.
func TestProcessBoundaryUnrelatedProcessSurvives(t *testing.T) {
	requireGit(t)
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"tracked.txt": "source-original", "keep.go": "package keep\n"})
	fx, env := orphanFixture(t, root, fakeBin)

	// Unrelated same-shaped writer with no ownership token.
	unrelatedFile := filepath.Join(root, "unrelated.txt")
	unrelated := spawnUnrelated(t, unrelatedFile)
	t.Cleanup(func() { _ = syscall.Kill(unrelated, syscall.SIGKILL) })

	portA := freePort(t)
	psA := startServer(t, serverBin, dataDir, portA, env)
	cred := pairProcessServer(t, psA)
	proj := openProjectViaAPI(t, psA, cred, src, "unrelated")
	conv := createConversation(t, psA, cred, proj)
	runID := sendMessage(t, psA, cred, conv, "add agent.go")
	waitFileRun(t, psA, cred, runID, fx.signalFile, 40*time.Second)

	psA.kill(t)
	time.Sleep(500 * time.Millisecond)
	_ = os.WriteFile(fx.markerFile, []byte("go\n"), 0o644)
	portB := freePort(t)
	psB := startServer(t, serverBin, dataDir, portB, env)
	defer psB.kill(t)

	time.Sleep(3 * time.Second)
	if !procAlive(unrelated) {
		t.Fatal("unrelated process was killed by orphan reconciliation")
	}
	before := fileSize(unrelatedFile)
	time.Sleep(700 * time.Millisecond)
	if after := fileSize(unrelatedFile); after <= before {
		t.Fatalf("unrelated process stopped writing: %d -> %d", before, after)
	}
	_ = runID
}

// TestProcessBoundaryGracefulShutdownReconciles proves SIGTERM performs a
// graceful shutdown that terminates the process tree and clears ownership.
func TestProcessBoundaryGracefulShutdownReconciles(t *testing.T) {
	requireGit(t)
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"tracked.txt": "source-original", "keep.go": "package keep\n"})
	fx, env := orphanFixture(t, root, fakeBin)

	portA := freePort(t)
	psA := startServer(t, serverBin, dataDir, portA, env)
	cred := pairProcessServer(t, psA)
	proj := openProjectViaAPI(t, psA, cred, src, "sigterm")
	conv := createConversation(t, psA, cred, proj)
	runID := sendMessage(t, psA, cred, conv, "add agent.go")
	waitFileRun(t, psA, cred, runID, fx.signalFile, 40*time.Second)
	orphanPid := readPid(t, filepath.Join(runWorkspacePath(dataDir, runID), "orphan.pid"))
	t.Cleanup(func() { _ = syscall.Kill(orphanPid, syscall.SIGKILL) })

	if err := psA.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = psA.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("server did not exit on SIGTERM")
	}
	waitProcGone(t, orphanPid, 10*time.Second)
	withStore(t, dataDir, func(st *storage.Store) {
		active, _ := st.ListActiveProcessOwners(context.Background())
		if len(active) != 0 {
			t.Fatalf("ownership not cleared after graceful shutdown: %+v", active)
		}
	})
}

func spawnUnrelated(t *testing.T, file string) int {
	t.Helper()
	script := "while true; do echo x >> " + file + "; sleep 0.2; done"
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = os.Environ() // no ownership token
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fileSize(file) > 0 {
			return cmd.Process.Pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("unrelated process never wrote")
	return 0
}
