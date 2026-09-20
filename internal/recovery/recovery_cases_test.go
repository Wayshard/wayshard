package recovery

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// makeCheckpoint captures a checkpoint with tracked.txt=content for a specific
// attempt and links it, returning the persisted record.
func makeCheckpoint(t *testing.T, ctx context.Context, st *storage.Store, b workspace.WorkspaceBackend, runID, stageID, attemptID, content string) *domain.WorkspaceCheckpoint {
	t.Helper()
	ws, err := st.GetWorkspaceByRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(st.Root, "runtime", "workspaces", runID, "checkpoints", stageID, "cp-"+content)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "tracked.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	fb := workspace.NewFilesystemBackend(src)
	snap, err := fb.CaptureSnapshot(ctx, workspace.CaptureOptions{SnapshotDir: base, ID: "cp"})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := workspace.CanonicalTreeHashV3(snap.TreePath)
	if err != nil {
		t.Fatal(err)
	}
	cp := &domain.WorkspaceCheckpoint{
		WorkspaceID: ws.ID, RunID: runID, StageID: stageID, AttemptID: attemptID,
		Name: "pre-attempt", TreeHash: digest, HashVersion: 3, TreePath: base,
	}
	if err := st.InsertCheckpoint(ctx, cp); err != nil {
		t.Fatal(err)
	}
	// Space out created_at so "latest for stage" would pick the later one.
	time.Sleep(2 * time.Millisecond)
	return cp
}

// TestRecoverySelectsAttemptCheckpointNotStageLatest proves checkpoint lookup is
// tied to the interrupted attempt. A later checkpoint belonging to a different
// attempt of the same stage must not be selected.
func TestRecoverySelectsAttemptCheckpointNotStageLatest(t *testing.T) {
	ctx := context.Background()
	st, runID, runPath := setupInterruptedWrite(t)
	stages, _ := st.ListStages(ctx, runID)
	stg := stages[0]
	atts, _ := st.ListAttempts(ctx, stg.ID)
	att1 := atts[0]

	// A later, terminal attempt in the same stage with a distinct checkpoint.
	att2 := &domain.StageAttempt{StageID: stg.ID, RunID: runID, Ordinal: 2, Status: domain.AttemptSucceeded}
	if err := st.AppendAttempt(ctx, att2); err != nil {
		t.Fatal(err)
	}
	b, err := workspace.Open(runPath)
	if err != nil {
		t.Fatal(err)
	}
	c2 := makeCheckpoint(t, ctx, st, b, runID, stg.ID, att1.ID, "c2-value")
	c3 := makeCheckpoint(t, ctx, st, b, runID, stg.ID, att2.ID, "c3-value")
	if err := st.SetAttemptCheckpoint(ctx, att1.ID, c2.ID); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt"))
	if string(got) != "c2-value" {
		t.Fatalf("wrong checkpoint lineage: got %q (c3=%s c2=%s)", got, c3.ID, c2.ID)
	}
}

// TestRecoveryLegacyNoCheckpointBlocks proves a durable interrupted write
// attempt with no checkpoint fails closed instead of trusting the partial
// workspace.
func TestRecoveryLegacyNoCheckpointBlocks(t *testing.T) {
	ctx := context.Background()
	st, runID, runPath := setupInterruptedWrite(t)
	if _, err := st.DB.ExecContext(ctx, `UPDATE stage_attempts SET checkpoint_id = '' WHERE run_id = ?`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `DELETE FROM workspace_checkpoints WHERE run_id = ?`, runID); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedRecovery {
		t.Fatalf("expected blocked recovery, got %s %s", run.Status, run.BlockedReason)
	}
	if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "half-written" {
		t.Fatalf("partial workspace was trusted: %q", got)
	}
}

func TestRecoveryHashVersionFailsClosed(t *testing.T) {
	for _, ver := range []int{2, 99} {
		t.Run("version", func(t *testing.T) {
			ctx := context.Background()
			st, runID, runPath := setupInterruptedWrite(t)
			if _, err := st.DB.ExecContext(ctx, `UPDATE workspace_checkpoints SET hash_version = ? WHERE run_id = ?`, ver, runID); err != nil {
				t.Fatal(err)
			}
			if err := Reconcile(ctx, st, slog.Default()); err != nil {
				t.Fatal(err)
			}
			run, _ := st.GetRun(ctx, runID)
			if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedRecovery {
				t.Fatalf("hash_version %d did not fail closed: %s %s", ver, run.Status, run.BlockedReason)
			}
			if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "half-written" {
				t.Fatalf("hash_version %d restored anyway: %q", ver, got)
			}
		})
	}
}

// TestRecoveryIdempotent proves a second reconciliation neither restores again
// nor duplicates the logical checkpoint.restored transition.
func TestRecoveryIdempotent(t *testing.T) {
	ctx := context.Background()
	st, runID, runPath := setupInterruptedWrite(t)
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "before" {
		t.Fatalf("workspace wrong after idempotent recovery: %q", got)
	}
	evs, _ := st.EventsSince(ctx, 0, "", runID, 1000)
	restored := 0
	for _, e := range evs {
		if e.Type == "checkpoint.restored" {
			restored++
		}
	}
	if restored != 1 {
		t.Fatalf("expected exactly one checkpoint.restored, got %d", restored)
	}
}

// TestRecoveryRestoreFailureBlocksAndCleans proves an injected failure after
// verification but before the RunWorkspace swap blocks recovery, leaves the
// partial workspace untrusted, records no successful restoration, and cleans
// the private staging tree.
func TestRecoveryRestoreFailureBlocksAndCleans(t *testing.T) {
	ctx := context.Background()
	st, runID, runPath := setupInterruptedWrite(t)
	sentinel := errors.New("injected failure before swap")
	restoreOptionsForTest = func(o *workspace.RestoreOptions) {
		o.AfterVerified = func(string) error { return sentinel }
	}
	defer func() { restoreOptionsForTest = nil }()

	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedRecovery {
		t.Fatalf("expected blocked recovery, got %s %s", run.Status, run.BlockedReason)
	}
	if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "half-written" {
		t.Fatalf("partial workspace was trusted after failed restore: %q", got)
	}
	stages, _ := st.ListStages(ctx, runID)
	atts, _ := st.ListAttempts(ctx, stages[0].ID)
	if atts[0].Status != domain.AttemptInterrupted {
		t.Fatalf("attempt not marked interrupted: %s", atts[0].Status)
	}
	evs, _ := st.EventsSince(ctx, 0, "", runID, 1000)
	for _, e := range evs {
		if e.Type == "checkpoint.restored" {
			t.Fatal("DB claimed a successful restoration that did not happen")
		}
	}
	if ents, err := os.ReadDir(filepath.Join(st.Root, "runtime", "restore-staging")); err == nil && len(ents) > 0 {
		t.Fatalf("staging leaked after failed restore: %v", ents)
	}
	// A second reconciliation must stay blocked and must not create attempts.
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	stages2, _ := st.ListStages(ctx, runID)
	atts2, _ := st.ListAttempts(ctx, stages2[0].ID)
	if len(atts2) != len(atts) {
		t.Fatalf("recovery created duplicate attempts: %d -> %d", len(atts), len(atts2))
	}
}

// TestReconcileLeavesCancelledRun proves explicit cancellation is durable and is
// never reinterpreted as a crash-interrupted attempt.
func TestReconcileLeavesCancelledRun(t *testing.T) {
	ctx := context.Background()
	st, runID, runPath := setupInterruptedWrite(t)
	if err := st.UpdateRunStatus(ctx, runID, domain.RunCancelled, domain.BlockedUser, "cancelled by client"); err != nil {
		t.Fatal(err)
	}
	if err := st.CancelRunningAttempts(ctx, runID); err != nil {
		t.Fatal(err)
	}
	before, _ := st.GetRun(ctx, runID)
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	after, _ := st.GetRun(ctx, runID)
	if after.Status != domain.RunCancelled {
		t.Fatalf("cancelled run changed: %s", after.Status)
	}
	stages, _ := st.ListStages(ctx, runID)
	atts, _ := st.ListAttempts(ctx, stages[0].ID)
	for _, a := range atts {
		if a.Status == domain.AttemptInterrupted {
			t.Fatal("cancelled attempt was rewritten to interrupted")
		}
	}
	evs, _ := st.EventsSince(ctx, 0, "", runID, 1000)
	for _, e := range evs {
		if e.Type == "checkpoint.restored" {
			t.Fatal("cancelled run was reactivated by recovery")
		}
	}
	if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "half-written" {
		t.Fatalf("cancelled run workspace was modified: %q", got)
	}
	_ = before
}

// TestRecoveryDoesNotTouchSourceWorkspace proves checkpoint restoration only
// rewrites the RunWorkspace and never the live source tree.
func TestRecoveryDoesNotTouchSourceWorkspace(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "source.txt"), []byte("source-original"), 0o644); err != nil {
		t.Fatal(err)
	}
	runPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(runPath, "tracked.txt"), []byte("baseline"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &domain.Project{Name: "p", Path: src, SourceKind: "filesystem"}
	if err := st.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	_ = st.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "x"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "x"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := st.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	_ = st.UpdateRunStatus(ctx, run.ID, domain.RunExecuting, "", "")
	rec := &domain.WorkspaceRecord{RunID: run.ID, ProjectID: p.ID, Kind: "filesystem", SourcePath: src, RunPath: runPath}
	if err := st.InsertWorkspace(ctx, rec); err != nil {
		t.Fatal(err)
	}
	b, err := workspace.Open(runPath)
	if err != nil {
		t.Fatal(err)
	}
	stg := &domain.Stage{RunID: run.ID, Kind: domain.StageExecute, Ordinal: 1, Status: domain.AttemptRunning}
	if err := st.AppendStage(ctx, stg); err != nil {
		t.Fatal(err)
	}
	att := &domain.StageAttempt{StageID: stg.ID, RunID: run.ID, Ordinal: 1, Status: domain.AttemptRunning}
	if err := st.AppendAttempt(ctx, att); err != nil {
		t.Fatal(err)
	}
	cp := makeCheckpoint(t, ctx, st, b, run.ID, stg.ID, att.ID, "baseline")
	if err := st.SetAttemptCheckpoint(ctx, att.ID, cp.ID); err != nil {
		t.Fatal(err)
	}
	// Partial crash state in the run workspace.
	_ = os.WriteFile(filepath.Join(runPath, "tracked.txt"), []byte("partial"), 0o644)
	_ = os.WriteFile(filepath.Join(runPath, "partial.txt"), []byte("partial"), 0o644)

	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "source.txt")); string(got) != "source-original" {
		t.Fatalf("recovery modified the live source: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "baseline" {
		t.Fatalf("run workspace not restored: %q", got)
	}
}
