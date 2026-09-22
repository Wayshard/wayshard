//go:build darwin

package harness

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/process"
	"github.com/Wayshard/wayshard/internal/recovery"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/sandbox"
	"github.com/Wayshard/wayshard/internal/storage"
)

// TestHarnessOwnerHelper is the fixture for ownership lifecycle tests. It is
// inert unless GO_WANT_OWNER_HELPER=1.
func TestHarnessOwnerHelper(t *testing.T) {
	if os.Getenv("GO_WANT_OWNER_HELPER") != "1" {
		t.Skip("not an owner helper invocation")
	}
	switch os.Getenv("OWNER_HELPER_MODE") {
	case "exit":
		os.Exit(0)
	case "sleep":
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	case "setsid":
		c := exec.Command(os.Args[0], "-test.run=TestHarnessOwnerHelper")
		c.Env = append(os.Environ(), "GO_WANT_OWNER_HELPER=1", "OWNER_HELPER_MODE=setsid-child")
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := c.Start(); err != nil {
			os.Exit(4)
		}
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	case "setsid-child":
		os.Stdout.WriteString("PID=" + strconv.Itoa(os.Getpid()) + "\n")
		g := exec.Command(os.Args[0], "-test.run=TestHarnessOwnerHelper")
		g.Env = append(os.Environ(), "GO_WANT_OWNER_HELPER=1", "OWNER_HELPER_MODE=sleep")
		g.Stdout = os.Stdout
		g.Stderr = os.Stderr
		if err := g.Start(); err != nil {
			os.Exit(4)
		}
		os.Stdout.WriteString("PID=" + strconv.Itoa(g.Process.Pid) + "\n")
		time.Sleep(5 * time.Minute)
		os.Exit(0)
	}
	os.Exit(2)
}

func startOwnerHelper(t *testing.T, mode, token string) (*exec.Cmd, *strings.Builder) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHarnessOwnerHelper")
	cmd.Env = append(os.Environ(), "GO_WANT_OWNER_HELPER=1", "OWNER_HELPER_MODE="+mode, process.TokenEnv+"="+token)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	return cmd, &out
}

func waitOwnerPIDs(t *testing.T, out *strings.Builder, want int) []int {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var pids []int
		for _, line := range strings.Split(out.String(), "\n") {
			if strings.HasPrefix(line, "PID=") {
				n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PID=")))
				if err == nil && n > 0 {
					pids = append(pids, n)
				}
			}
		}
		if len(pids) >= want {
			return pids
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("helper did not report %d pids; output:\n%s", want, out.String())
	return nil
}

func openStore(t *testing.T, root string) *storage.Store {
	t.Helper()
	st, err := storage.Open(context.Background(), root)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// TestDarwinFinishProcessOwnerReconcilesDetached proves the production attempt
// close path terminates a setsid descendant and only then marks the owner
// reconciled. A descendant must never survive while the owner is reconciled.
func TestDarwinFinishProcessOwnerReconcilesDetached(t *testing.T) {
	if !process.Supported() {
		t.Fatal("authoritative ownership must be supported where FeatureProcessTree is advertised")
	}
	ctx := context.Background()
	st := openStore(t, t.TempDir())
	token, err := process.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	cmd, out := startOwnerHelper(t, "setsid", token)
	defer func() { _ = cmd.Process.Kill() }()
	detached := waitOwnerPIDs(t, out, 2)

	owner := &domain.ProcessOwner{
		RunID: "run_1", StageID: "stg_1", AttemptID: "att_1",
		TokenHash: process.HashToken(token), State: domain.ProcessOwnerActive,
	}
	if err := st.InsertProcessOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	e := &ACPExec{Store: st}
	e.finishProcessOwner(ctx, owner, token)

	active, _ := st.ListActiveProcessOwners(ctx)
	if len(active) != 0 {
		t.Fatalf("owner must be reconciled only after the tree is gone; active=%+v", active)
	}
	for _, pid := range detached {
		if ownerProcessAlive(pid) {
			t.Fatalf("setsid descendant %d survived while owner was reconciled", pid)
		}
	}
}

// TestDarwinCleanProbeLeavesNoActiveOwner proves a clean, successful probe does
// not leave an active durable owner (which would poison the next startup).
func TestDarwinCleanProbeLeavesNoActiveOwner(t *testing.T) {
	if !process.Supported() {
		t.Fatal("authoritative ownership must be supported")
	}
	ctx := context.Background()
	st := openStore(t, t.TempDir())
	sink := StoreProbeOwnerSink{Store: st}
	lease, err := sink.BeginProbe(ctx, "version")
	if err != nil {
		t.Fatalf("BeginProbe: %v", err)
	}
	if lease == nil {
		t.Fatal("nil lease")
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestHarnessOwnerHelper")
	cmd.Env = append(os.Environ(), "GO_WANT_OWNER_HELPER=1", "OWNER_HELPER_MODE=exit", process.TokenEnv+"="+lease.Token())
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper: %v", err)
	}
	lease.Done()
	active, _ := st.ListActiveProbeOwners(ctx)
	if len(active) != 0 {
		t.Fatalf("clean probe left an active owner: %+v", active)
	}
}

// TestDarwinDetachedProbeReconciledOnRestart proves a probe that left a setsid
// descendant is reconciled by token at the next startup, without poisoning probe
// ownership.
func TestDarwinDetachedProbeReconciledOnRestart(t *testing.T) {
	if !process.Supported() {
		t.Fatal("authoritative ownership must be supported")
	}
	ctx := context.Background()
	root := t.TempDir()
	st := openStore(t, root)
	sink := StoreProbeOwnerSink{Store: st}
	lease, err := sink.BeginProbe(ctx, "version")
	if err != nil {
		t.Fatal(err)
	}
	cmd, out := startOwnerHelper(t, "setsid", lease.Token())
	detached := waitOwnerPIDs(t, out, 2)
	// Simulate a probe root that exited (or was killed) while its detached
	// descendant survived: leave the lease unreconciled.
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	active, _ := st.ListActiveProbeOwners(ctx)
	if len(active) == 0 {
		t.Fatal("expected an active probe owner before recovery")
	}
	recovery.Reconcile(ctx, st, slog.Default())
	active2, _ := st.ListActiveProbeOwners(ctx)
	if len(active2) != 0 {
		t.Fatalf("detached probe owner not reconciled: %+v", active2)
	}
	for _, pid := range detached {
		if ownerProcessAlive(pid) {
			t.Fatalf("detached probe descendant %d survived reconciliation", pid)
		}
	}
	if !recovery.ProbeOwnershipReconciled() {
		t.Fatal("detached probe must not poison probe ownership after authoritative reconciliation")
	}
}

// TestDarwinCleanProbeTwoStart proves a clean probing session does not disable
// probing after the next server start.
func TestDarwinCleanProbeTwoStart(t *testing.T) {
	if !process.Supported() {
		t.Fatal("authoritative ownership must be supported")
	}
	ctx := context.Background()
	root := t.TempDir()

	// Server A: clean probe.
	stA := openStore(t, root)
	sinkA := StoreProbeOwnerSink{Store: stA}
	lease, err := sinkA.BeginProbe(ctx, "version")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestHarnessOwnerHelper")
	cmd.Env = append(os.Environ(), "GO_WANT_OWNER_HELPER=1", "OWNER_HELPER_MODE=exit", process.TokenEnv+"="+lease.Token())
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	lease.Done()
	if active, _ := stA.ListActiveProbeOwners(ctx); len(active) != 0 {
		t.Fatalf("clean probe left active owner: %+v", active)
	}
	_ = stA.Close()

	// Server B: reopen the same state and reconcile.
	stB := openStore(t, root)
	recovery.Reconcile(ctx, stB, slog.Default())
	if !recovery.ProbeOwnershipReconciled() {
		t.Fatal("clean probing session poisoned probe ownership for the next startup")
	}
	sinkB := StoreProbeOwnerSink{Store: stB}
	if _, err := sinkB.BeginProbe(ctx, "version"); err != nil {
		t.Fatalf("probing disabled after clean restart: %v", err)
	}
}

func ownerProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// TestDarwinACPExecReconcilesProcessOwner exercises the production launch path
// (ACPExec -> ACP launch -> driver close -> finishProcessOwner) on macOS and
// proves a clean attempt leaves no active process owner.
func TestDarwinACPExecReconcilesProcessOwner(t *testing.T) {
	if !process.Supported() {
		t.Fatal("authoritative ownership must be supported")
	}
	ctx := context.Background()
	st := openStore(t, t.TempDir())
	bin := buildFake(t)
	ex := &ACPExec{Store: st, Sandbox: &sandbox.Manager{Backend: sandbox.DefaultBackend()}}
	res, err := ex.Execute(ctx, orchestrator.StageRequest{
		Task:    domain.Task{Objective: "plan a change"},
		Run:     domain.Run{ID: "run_1"},
		Stage:   domain.Stage{ID: "stg_1", Kind: domain.StagePlan},
		Attempt: domain.StageAttempt{ID: "att_1"},
		Route: routing.Candidate{Harness: domain.HarnessInstallation{
			DefinitionID: "wayshard-fake-acp", Executable: bin, Adapter: "generic", DisplayName: "fake",
		}},
	})
	if err != nil || res.Err != nil {
		t.Fatalf("execute: err=%v res.Err=%v", err, res.Err)
	}
	active, _ := st.ListActiveProcessOwners(ctx)
	if len(active) != 0 {
		t.Fatalf("process owner not reconciled after clean production close: %+v", active)
	}
}
