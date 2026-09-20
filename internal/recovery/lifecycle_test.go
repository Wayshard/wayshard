//go:build linux

package recovery

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/process"
)

// TestCheckpointPathOwnership covers the F2 hardening matrix.
func TestCheckpointPathOwnership(t *testing.T) {
	ctx := context.Background()
	st, runID, _ := setupInterruptedWrite(t)
	run, _ := st.GetRun(ctx, runID)
	stages, _ := st.ListStages(ctx, runID)
	stg := stages[0]
	cps, _ := st.ListCheckpointsByRun(ctx, runID)
	cp := cps[0]

	if err := validateCheckpointPath(st, *run, stg, &cp); err != nil {
		t.Fatalf("normal checkpoint rejected: %v", err)
	}

	root := filepath.Join(st.Root, "runtime", "workspaces")
	other := filepath.Join(root, "01a00000-0000-7000-8000-000000000000", "checkpoints", stg.ID, "1")
	_ = os.MkdirAll(other, 0o755)

	cases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"outside absolute", "/etc/passwd", true},
		{"sibling runtime dir", filepath.Join(st.Root, "runtime", "journals", runID), true},
		{"parent escape", filepath.Join(root, runID, "checkpoints", stg.ID, "..", "..", "..", "escape"), true},
		{"other run", other, true},
		{"same run other stage", filepath.Join(root, runID, "checkpoints", "other-stage", "1"), true},
	}
	for _, c := range cases {
		if err := validateCheckpointPath(st, *run, stg, &domain.WorkspaceCheckpoint{TreePath: c.path}); (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", c.name, err, c.wantErr)
		}
	}

	// Cross-run symlink must be rejected.
	link := filepath.Join(root, runID, "checkpoints", stg.ID, "evil")
	_ = os.RemoveAll(link)
	if err := os.Symlink(other, link); err == nil {
		if err := validateCheckpointPath(st, *run, stg, &domain.WorkspaceCheckpoint{TreePath: link}); err == nil {
			t.Error("cross-run symlink accepted")
		}
	}

	// Same-run different-stage symlink must be rejected.
	sameRunOtherStage := filepath.Join(root, runID, "checkpoints", "other-stage", "1")
	_ = os.MkdirAll(sameRunOtherStage, 0o755)
	link2 := filepath.Join(root, runID, "checkpoints", stg.ID, "evil2")
	_ = os.RemoveAll(link2)
	if err := os.Symlink(sameRunOtherStage, link2); err == nil {
		if err := validateCheckpointPath(st, *run, stg, &domain.WorkspaceCheckpoint{TreePath: link2}); err == nil {
			t.Error("same-run other-stage symlink accepted")
		}
	}

	// Substring run id must not pass.
	sub := filepath.Join(root, "abcdef", "checkpoints", stg.ID, "1")
	if err := validateCheckpointPath(st, *run, stg, &domain.WorkspaceCheckpoint{TreePath: sub}); err == nil {
		t.Error("substring run id accepted")
	}
}

// TestReconcileTerminatesOwnedProcess proves startup reconciliation kills a
// token-identified process before restoring the workspace.
func TestReconcileTerminatesOwnedProcess(t *testing.T) {
	ctx := context.Background()
	st, runID, runPath := setupInterruptedWrite(t)
	stages, _ := st.ListStages(ctx, runID)
	atts, _ := st.ListAttempts(ctx, stages[0].ID)

	token, err := process.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "-c", "exec sleep 300")
	cmd.Env = append(os.Environ(), process.TokenEnv+"="+token)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	owner := &domain.ProcessOwner{
		RunID: runID, StageID: stages[0].ID, AttemptID: atts[0].ID,
		TokenHash: process.HashToken(token), State: domain.ProcessOwnerActive,
	}
	if err := st.InsertProcessOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	// The process was started by this test, so SIGKILL leaves it a zombie until
	// reaped. Wait proves it was terminated rather than still running.
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("owned process survived reconciliation")
	}
	active, _ := st.ListActiveProcessOwners(ctx)
	if len(active) != 0 {
		t.Fatalf("owner record not reconciled: %+v", active)
	}
	if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "before" {
		t.Fatalf("workspace not restored after process reconciliation: %q", got)
	}
}

// TestBlockForStaleProcessesFailClosed proves an unreconcilable tree blocks the
// run and closes running attempts.
func TestBlockForStaleProcessesFailClosed(t *testing.T) {
	ctx := context.Background()
	st, runID, runPath := setupInterruptedWrite(t)
	stages, _ := st.ListStages(ctx, runID)
	atts, _ := st.ListAttempts(ctx, stages[0].ID)

	blockForStaleProcesses(ctx, st, slog.Default(), domain.ProcessOwner{
		RunID: runID, StageID: stages[0].ID, AttemptID: atts[0].ID,
	}, "stale process tree could not be reconciled")

	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedRecovery {
		t.Fatalf("run not blocked: %s %s", run.Status, run.BlockedReason)
	}
	got, _ := st.ListAttempts(ctx, stages[0].ID)
	if got[0].Status != domain.AttemptInterrupted {
		t.Fatalf("attempt not interrupted: %s", got[0].Status)
	}
	if body, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(body) != "half-written" {
		t.Fatalf("partial workspace disturbed: %q", body)
	}
}

// TestReconcileRejectsReclaimedCheckpoint proves recovery fails closed when the
// checkpoint material has been reclaimed.
func TestReconcileRejectsReclaimedCheckpoint(t *testing.T) {
	ctx := context.Background()
	st, runID, runPath := setupInterruptedWrite(t)
	cps, _ := st.ListCheckpointsByRun(ctx, runID)
	if err := st.MarkCheckpointReclaimed(ctx, cps[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedRecovery {
		t.Fatalf("reclaimed checkpoint did not fail closed: %s %s", run.Status, run.BlockedReason)
	}
	if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "half-written" {
		t.Fatalf("partial workspace trusted: %q", got)
	}
}

// TestReconcileCancelsStalePendingApproval proves a permission pending at crash
// is invalidated on recovery (never auto-approved, never left dangling).
func TestReconcileCancelsStalePendingApproval(t *testing.T) {
	ctx := context.Background()
	st, runID, _ := setupInterruptedWrite(t)
	ap := &domain.Approval{RunID: runID, Kind: "acp_permission", Resource: "edit", Reason: "needs approval", Status: "pending"}
	if err := st.InsertApproval(ctx, ap); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetApproval(ctx, ap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == "pending" {
		t.Fatal("stale pending approval survived recovery")
	}
	stages, _ := st.ListStages(ctx, runID)
	atts, _ := st.ListAttempts(ctx, stages[0].ID)
	if atts[0].Status != domain.AttemptInterrupted {
		t.Fatalf("attempt = %s, want interrupted", atts[0].Status)
	}
}

// TestReconcileCancelledRunAttemptConsistency covers the F3 crash window: a
// durably CANCELLED run with a leftover running attempt.
func TestReconcileCancelledRunAttemptConsistency(t *testing.T) {
	ctx := context.Background()
	st, runID, _ := setupInterruptedWrite(t)
	if err := st.UpdateRunStatus(ctx, runID, domain.RunCancelled, domain.BlockedUser, "cancelled by client"); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunCancelled {
		t.Fatalf("cancelled run changed: %s", run.Status)
	}
	stages, _ := st.ListStages(ctx, runID)
	atts, _ := st.ListAttempts(ctx, stages[0].ID)
	if atts[0].Status != domain.AttemptCancelled {
		t.Fatalf("cancelled run attempt = %s, want cancelled", atts[0].Status)
	}
	evs, _ := st.EventsSince(ctx, 0, "", runID, 1000)
	for _, e := range evs {
		if e.Type == "checkpoint.restored" {
			t.Fatal("cancelled run was reactivated")
		}
	}
}
