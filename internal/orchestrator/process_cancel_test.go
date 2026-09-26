package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/jev"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/storage"
)

// newArtifactOnlyEngine builds a ready-to-run synthetic engine for an
// artifact-only task, so cancellation paths can be driven without a harness.
func newArtifactOnlyEngine(t *testing.T) (*storage.Store, *Engine, domain.Run) {
	t.Helper()
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := st.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	if err := st.InsertConversation(ctx, c); err != nil {
		t.Fatal(err)
	}
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "brainstorm"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "brainstorm", ArtifactOnly: true}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := st.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	insertTestWorkspace(t, st, run, p.Path)
	eng := &Engine{
		Store:  st,
		Jev:    jev.DeterministicEngine{},
		Router: &routing.Router{Engine: jev.DeterministicEngine{}},
		Exec:   stubExec{},
		Candidates: candidateList{{
			Harness: domain.HarnessInstallation{ID: "fake", DisplayName: "fake", Health: domain.HarnessReady, Compatibility: domain.CompatRoutable},
			ModelID: "fake",
		}},
	}
	return st, eng, *run
}

func requireCancelled(t *testing.T, st *storage.Store, runID string) {
	t.Helper()
	got, err := st.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.RunCancelled {
		t.Fatalf("run status = %s (reason=%s detail=%q), want %s", got.Status, got.BlockedReason, got.BlockedDetail, domain.RunCancelled)
	}
}

// cancellingExec cancels the run context from inside the stage and reports the
// cancellation, reproducing cancellation observed mid-stage.
type cancellingExec struct{ cancel context.CancelFunc }

func (c cancellingExec) Execute(ctx context.Context, req StageRequest) (StageResult, error) {
	c.cancel()
	return StageResult{Err: ctx.Err(), Class: domain.FailUser}, ctx.Err()
}

// failingExec reports a genuine infrastructure failure that is not cancellation.
type failingExec struct{}

func (failingExec) Execute(context.Context, StageRequest) (StageResult, error) {
	err := errors.New("infrastructure boom")
	return StageResult{Err: err, Class: domain.FailInfrastructure}, err
}

// TestProcessRunCancellationDuringStatusReadConverges reproduces the pre-existing
// race deterministically: cancellation is forced at the exact point between a
// completed stage and the run-status read, so the status read (and any other
// storage call taking the canceled context) fails with context.Canceled. The run
// must still converge to CANCELLED rather than being left non-terminal.
func TestProcessRunCancellationDuringStatusReadConverges(t *testing.T) {
	st, eng, run := newArtifactOnlyEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	forced := false
	processRunObserver = func(step string) {
		if step == "status" && !forced {
			forced = true
			cancel()
		}
	}
	t.Cleanup(func() { processRunObserver = nil })

	if err := eng.ProcessRun(ctx, run.ID); err != nil {
		t.Fatalf("ProcessRun returned %v; cancellation must converge cleanly", err)
	}
	if !forced {
		t.Fatal("cancellation point was never reached")
	}
	requireCancelled(t, st, run.ID)
}

// TestProcessRunCancellationInsideStageConverges forces cancellation from inside
// the stage operation itself.
func TestProcessRunCancellationInsideStageConverges(t *testing.T) {
	st, eng, run := newArtifactOnlyEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eng.Exec = cancellingExec{cancel: cancel}

	if err := eng.ProcessRun(ctx, run.ID); err != nil {
		t.Fatalf("ProcessRun returned %v; cancellation must converge cleanly", err)
	}
	requireCancelled(t, st, run.ID)
}

// TestProcessRunPreCanceledContextConverges forces cancellation before the run
// does any work.
func TestProcessRunPreCanceledContextConverges(t *testing.T) {
	st, eng, run := newArtifactOnlyEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := eng.ProcessRun(ctx, run.ID); err != nil {
		t.Fatalf("ProcessRun returned %v; cancellation must converge cleanly", err)
	}
	requireCancelled(t, st, run.ID)
}

// TestProcessRunPreservesNonCancellationFailure guards the other direction: a
// genuine infrastructure failure with no cancellation must keep its existing
// outcome (run FAILED, not CANCELLED).
func TestProcessRunPreservesNonCancellationFailure(t *testing.T) {
	st, eng, run := newArtifactOnlyEngine(t)
	eng.Exec = failingExec{}
	eng.Budget = Budgets{MaxStageAttempts: 1}

	if err := eng.ProcessRun(context.Background(), run.ID); err != nil {
		t.Fatalf("ProcessRun: %v", err)
	}
	got, err := st.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == domain.RunCancelled {
		t.Fatal("non-cancellation failure was reported as cancelled")
	}
	if got.Status != domain.RunFailed {
		t.Fatalf("run status = %s, want %s", got.Status, domain.RunFailed)
	}
}
