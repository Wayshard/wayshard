package recovery

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
)

func setupInterruptedWrite(t *testing.T) (st *storage.Store, runID, runPath string) {
	t.Helper()
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	runPath = t.TempDir()
	if err := os.WriteFile(filepath.Join(runPath, "tracked.txt"), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
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
	if err := st.UpdateRunStatus(ctx, run.ID, domain.RunExecuting, "", ""); err != nil {
		t.Fatal(err)
	}
	rec := &domain.WorkspaceRecord{RunID: run.ID, ProjectID: p.ID, Kind: "filesystem", SourcePath: runPath, RunPath: runPath}
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
	cpDir := filepath.Join(st.Root, "runtime", "workspaces", run.ID, "checkpoints", stg.ID, "1")
	snap, err := b.CaptureSnapshot(ctx, workspace.CaptureOptions{SnapshotDir: cpDir, ID: "cp1"})
	if err != nil {
		t.Fatal(err)
	}
	canon, err := workspace.CanonicalTreeHashV3(snap.TreePath)
	if err != nil {
		t.Fatal(err)
	}
	cp := &domain.WorkspaceCheckpoint{WorkspaceID: rec.ID, RunID: run.ID, StageID: stg.ID, AttemptID: att.ID, Name: "pre-attempt", TreeHash: canon, HashVersion: 3, TreePath: cpDir}
	if err := st.InsertCheckpoint(ctx, cp); err != nil {
		t.Fatal(err)
	}
	_ = st.SetAttemptCheckpoint(ctx, att.ID, cp.ID)
	// Simulate an interrupted write: partial modification + new file.
	if err := os.WriteFile(filepath.Join(runPath, "tracked.txt"), []byte("half-written"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runPath, "partial.txt"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	return st, run.ID, runPath
}

func TestRecoveryRestoresInterruptedWriteCheckpoint(t *testing.T) {
	st, runID, runPath := setupInterruptedWrite(t)
	if err := Reconcile(context.Background(), st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt"))
	if string(got) != "before" {
		t.Fatalf("tracked.txt not restored: %q", got)
	}
	if _, err := os.Stat(filepath.Join(runPath, "partial.txt")); err == nil {
		t.Fatal("partial write survived checkpoint restore")
	}
	// Attempt history preserved as interrupted.
	stages, _ := st.ListStages(context.Background(), runID)
	atts, _ := st.ListAttempts(context.Background(), stages[0].ID)
	if len(atts) != 1 || atts[0].Status != domain.AttemptInterrupted {
		t.Fatalf("attempt history wrong: %+v", atts)
	}
	evs, _ := st.EventsSince(context.Background(), 0, "", runID, 1000)
	found := false
	for _, e := range evs {
		if e.Type == "checkpoint.restored" {
			found = true
		}
	}
	if !found {
		t.Fatal("no checkpoint.restored event")
	}
}

func TestRecoveryDetectsCheckpointTreeCorruption(t *testing.T) {
	st, runID, runPath := setupInterruptedWrite(t)
	cps, _ := st.ListCheckpointsByRun(context.Background(), runID)
	if len(cps) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(cps))
	}
	// Corrupt a file inside the checkpoint tree (metadata untouched).
	target := filepath.Join(cps[0].TreePath, "tree", "tracked.txt")
	if err := os.WriteFile(target, []byte("corrupted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(context.Background(), st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(context.Background(), runID)
	if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedRecovery {
		t.Fatalf("expected blocked recovery on corrupt tree, got %s %s", run.Status, run.BlockedReason)
	}
	// Partial workspace must not have been trusted or restored.
	if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "half-written" {
		t.Fatalf("corrupt checkpoint unexpectedly changed workspace: %q", got)
	}
}

func TestRecoveryRejectsCheckpointOutsideRuntimeRoot(t *testing.T) {
	st, runID, _ := setupInterruptedWrite(t)
	ctx := context.Background()
	stages, _ := st.ListStages(ctx, runID)
	atts, _ := st.ListAttempts(ctx, stages[0].ID)
	ws, err := st.GetWorkspaceByRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	bad := &domain.WorkspaceCheckpoint{
		WorkspaceID: ws.ID, RunID: runID, StageID: stages[0].ID, AttemptID: atts[0].ID,
		Name: "bad", TreeHash: "deadbeef", HashVersion: 3, TreePath: t.TempDir(),
	}
	if err := st.InsertCheckpoint(ctx, bad); err != nil {
		t.Fatal(err)
	}
	// Associate the out-of-root checkpoint with the interrupted attempt: a
	// stored path must never be able to redirect restoration elsewhere.
	if err := st.SetAttemptCheckpoint(ctx, atts[0].ID, bad.ID); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedRecovery {
		t.Fatalf("expected blocked recovery for out-of-root checkpoint, got %s %s", run.Status, run.BlockedReason)
	}
}

func TestRecoveryBlocksOnCorruptCheckpoint(t *testing.T) {
	st, runID, runPath := setupInterruptedWrite(t)
	// Corrupt the checkpoint snapshot metadata.
	cps, _ := st.ListCheckpointsByRun(context.Background(), runID)
	if len(cps) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(cps))
	}
	if err := os.Remove(filepath.Join(cps[0].TreePath, "meta.json")); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(context.Background(), st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(context.Background(), runID)
	if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedRecovery {
		t.Fatalf("expected blocked recovery, got %s %s", run.Status, run.BlockedReason)
	}
	// Must not have silently restored or continued on unknown state.
	if got, _ := os.ReadFile(filepath.Join(runPath, "tracked.txt")); string(got) != "half-written" {
		t.Fatalf("corrupt checkpoint unexpectedly changed workspace: %q", got)
	}
}
