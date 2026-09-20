package recovery

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/integration"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// setupJournalRun prepares a source-changing run whose integration left a
// durable journal. crashAfter=0 publishes everything (pre-final); >0 stops
// after that many verified writes.
func setupJournalRun(t *testing.T, st *storage.Store, crashAfter int) (runID, src string, j *integration.Journal) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	src = filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "mod.txt"), "old\n")
	writeFile(t, filepath.Join(src, "del.txt"), "gone\n")
	writeFile(t, filepath.Join(src, "keep.txt"), "keep\n")

	b, err := workspace.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := b.CaptureSnapshot(ctx, workspace.CaptureOptions{SnapshotDir: filepath.Join(root, "snap")})
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(root, "run")
	if err := b.Materialize(ctx, snap, runDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(runDir, "mod.txt"), "new\n")
	writeFile(t, filepath.Join(runDir, "added.txt"), "added\n")
	if err := os.Remove(filepath.Join(runDir, "del.txt")); err != nil {
		t.Fatal(err)
	}

	p := &domain.Project{Name: "p", Path: src, SourceKind: b.Kind()}
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
	if err := st.UpdateRunStatus(ctx, run.ID, domain.RunIntegrating, "", ""); err != nil {
		t.Fatal(err)
	}
	rec := &domain.WorkspaceRecord{RunID: run.ID, ProjectID: p.ID, Kind: b.Kind(), SourcePath: src, RunPath: runDir}
	if err := st.InsertWorkspace(ctx, rec); err != nil {
		t.Fatal(err)
	}

	jdir := filepath.Join(st.Root, "runtime", "journals", run.ID)
	res, err := integration.Integrate(ctx, integration.Request{
		RunID: run.ID, ProjectID: p.ID, Source: b, Snapshot: snap, RunWorkspace: runDir,
		WorkDir: filepath.Join(root, "work"), JournalDir: jdir, CrashAfter: crashAfter,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Journal == nil {
		t.Fatalf("no journal produced (status=%s)", res.Status)
	}
	return run.ID, src, res.Journal
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readBody(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// TestReconcilePublicationPreFinalFinalizes proves a journal whose targets all
// match the intended after state is finalized without republishing.
func TestReconcilePublicationPreFinalFinalizes(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runID, src, j := setupJournalRun(t, st, 0)

	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunComplete {
		t.Fatalf("run status = %s, want complete", run.Status)
	}
	in, err := st.GetIntegrationByID(ctx, j.IntegrationID)
	if err != nil || in.Status != integration.StatusPublished {
		t.Fatalf("integration = %+v err=%v", in, err)
	}
	if readBody(t, filepath.Join(src, "mod.txt")) != "new\n" {
		t.Fatal("mod.txt wrong")
	}
	if _, err := os.Stat(filepath.Join(src, "del.txt")); err == nil {
		t.Fatal("del.txt not deleted")
	}
	if _, err := os.Stat(j.Path()); err == nil {
		t.Fatal("journal not cleaned after finalize")
	}
	// Idempotent second pass.
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run2, _ := st.GetRun(ctx, runID)
	if run2.Status != domain.RunComplete {
		t.Fatalf("second reconcile changed run: %s", run2.Status)
	}
}

// TestReconcilePublicationResumesPartial proves an incomplete journal resumes
// the remaining writes and finalizes.
func TestReconcilePublicationResumesPartial(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runID, src, _ := setupJournalRun(t, st, 1)

	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunComplete {
		t.Fatalf("run status = %s, want complete", run.Status)
	}
	if readBody(t, filepath.Join(src, "mod.txt")) != "new\n" || readBody(t, filepath.Join(src, "added.txt")) != "added\n" {
		t.Fatal("resume did not complete publication")
	}
	if _, err := os.Stat(filepath.Join(src, "del.txt")); err == nil {
		t.Fatal("del.txt not deleted after resume")
	}
}

// TestReconcilePublicationBlocksOnUserEdit proves an unexpected user edit
// blocks integration and is preserved.
func TestReconcilePublicationBlocksOnUserEdit(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runID, src, j := setupJournalRun(t, st, 1)

	// Edit a pending target (the delete).
	writeFile(t, filepath.Join(src, "del.txt"), "user-edit\n")

	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunIntegrationBlocked || run.BlockedReason != domain.BlockedIntegration {
		t.Fatalf("run = %s %s, want integration_blocked", run.Status, run.BlockedReason)
	}
	if readBody(t, filepath.Join(src, "del.txt")) != "user-edit\n" {
		t.Fatal("user edit overwritten")
	}
	in, _ := st.GetIntegrationByID(ctx, j.IntegrationID)
	if in == nil || in.Status != integration.StatusBlocked {
		t.Fatalf("integration = %+v, want blocked", in)
	}
	// Journal retained for a later retry.
	if _, err := os.Stat(j.Path()); err != nil {
		t.Fatal("journal removed despite blocked recovery")
	}
}

// TestReconcilePublicationIdentityMismatchBlocks proves a relocated source does
// not receive an old journal.
func TestReconcilePublicationIdentityMismatchBlocks(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runID, _, j := setupJournalRun(t, st, 0)

	// Point the journal at a different source path.
	j.SourcePath = filepath.Join(t.TempDir(), "elsewhere")
	if err := j.Save(); err != nil {
		t.Fatal(err)
	}
	if err := Reconcile(ctx, st, slog.Default()); err != nil {
		t.Fatal(err)
	}
	run, _ := st.GetRun(ctx, runID)
	if run.Status != domain.RunIntegrationBlocked {
		t.Fatalf("run = %s, want integration_blocked", run.Status)
	}
}
