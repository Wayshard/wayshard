package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/storage"
)

type budgetCandidates struct{}

func (budgetCandidates) Candidates(context.Context) ([]routing.Candidate, error) {
	return []routing.Candidate{{
		Harness: domain.HarnessInstallation{ID: "h1", DisplayName: "stub", Health: domain.HarnessReady, Compatibility: domain.CompatRoutable},
		ModelID: "m1",
	}}, nil
}

type budgetExec struct {
	infra bool
}

func (s budgetExec) Execute(_ context.Context, req StageRequest) (StageResult, error) {
	if s.infra {
		return StageResult{Class: domain.FailInfrastructure, Err: errors.New("boom")}, errors.New("boom")
	}
	switch req.Stage.Kind {
	case domain.StagePlan, domain.StageReplan:
		b, _ := artifacts.Marshal(artifacts.PlanArtifact{Kind: "plan", Objective: "o", AcceptanceCriteria: []string{"c"}, ValidationPlan: []string{"true"}})
		return StageResult{ArtifactJSON: b}, nil
	case domain.StageExecute, domain.StageRepair:
		b, _ := artifacts.Marshal(artifacts.ImplementationReport{Kind: "implementation_report", Summary: "s"})
		return StageResult{ArtifactJSON: b}, nil
	case domain.StageReview:
		b, _ := artifacts.Marshal(artifacts.ReviewArtifact{Kind: "review", Verdict: "fail", Findings: []artifacts.ReviewFinding{{Severity: "blocking", Explanation: "bad"}}})
		return StageResult{ArtifactJSON: b}, nil
	case domain.StageExplore:
		b, _ := artifacts.Marshal(artifacts.InvestigationArtifact{Kind: "investigation", Question: "q"})
		return StageResult{ArtifactJSON: b}, nil
	default:
		return StageResult{ArtifactJSON: `{"kind":"failure"}`}, nil
	}
}

func newEngine(t *testing.T, exec StageExec) (*Engine, *storage.Store, string) {
	t.Helper()
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := st.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "c"}
	if err := st.InsertConversation(ctx, c); err != nil {
		t.Fatal(err)
	}
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "x"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "x"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := st.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	insertTestWorkspace(t, st, run, p.Path)
	e := &Engine{Store: st, Candidates: budgetCandidates{}, Exec: exec, Budget: DefaultBudgets()}
	return e, st, run.ID
}

func assertBoundedTerminal(t *testing.T, st *storage.Store, runID string) domain.Run {
	t.Helper()
	ctx := context.Background()
	run, err := st.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if !run.Status.Terminal() && run.Status != domain.RunBlocked {
		t.Fatalf("run did not reach a terminal/blocked outcome: %s", run.Status)
	}
	stages, _ := st.ListStages(ctx, runID)
	if len(stages) > DefaultBudgets().MaxStages {
		t.Fatalf("stage budget exceeded: %d stages", len(stages))
	}
	for _, s := range stages {
		if s.Status == domain.AttemptRunning {
			t.Fatalf("stage %s left running after terminal outcome", s.Kind)
		}
	}
	evs, _ := st.EventsSince(ctx, 0, "", runID, 100000)
	if len(evs) > 500 {
		t.Fatalf("unbounded event growth: %d events", len(evs))
	}
	return *run
}

// TestRepairBudgetTerminates proves a permanently failing review cannot loop
// forever.
func TestRepairBudgetTerminates(t *testing.T) {
	e, st, runID := newEngine(t, budgetExec{})
	if err := e.ProcessRun(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	run := assertBoundedTerminal(t, st, runID)
	if run.Status != domain.RunBlocked || run.BlockedReason != domain.BlockedBudget {
		t.Fatalf("status=%s reason=%s detail=%s", run.Status, run.BlockedReason, run.BlockedDetail)
	}
	repairs, _ := st.CountStagesByKind(context.Background(), runID, domain.StageRepair)
	if repairs > DefaultBudgets().MaxRepair {
		t.Fatalf("repair budget exceeded: %d", repairs)
	}
}

// TestInfrastructureBudgetTerminates proves repeated harness failure stops.
func TestInfrastructureBudgetTerminates(t *testing.T) {
	e, st, runID := newEngine(t, budgetExec{infra: true})
	if err := e.ProcessRun(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	run := assertBoundedTerminal(t, st, runID)
	if run.Status != domain.RunFailed {
		t.Fatalf("status=%s detail=%s", run.Status, run.BlockedDetail)
	}
}

// TestWriteAttemptCreatesCheckpoint proves a write attempt persists a real,
// restorable checkpoint before executing.
func TestWriteAttemptCreatesCheckpoint(t *testing.T) {
	e, st, runID := newEngine(t, budgetExec{})
	if err := e.ProcessRun(context.Background(), runID); err != nil {
		t.Fatal(err)
	}
	cps, err := st.ListCheckpointsByRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cps) == 0 {
		t.Fatal("no checkpoint created for write attempt")
	}
	for _, cp := range cps {
		if cp.TreePath == "" {
			t.Fatalf("checkpoint has no tree path: %+v", cp)
		}
	}
}

// TestCompletionPolicyHonoursBaseline proves a pre-existing failure is not a
// blocking regression.
func TestCompletionPolicyHonoursBaseline(t *testing.T) {
	plan := &artifacts.PlanArtifact{Objective: "o", AcceptanceCriteria: []string{"c"}, ValidationPlan: []string{"true"}}
	val := &artifacts.ValidationArtifact{Kind: "validation", Checks: []artifacts.ValidationCheck{
		{Name: "go-test", Required: true, Status: string(domain.CheckFail), Baseline: string(domain.CheckFail)},
	}}
	rev := &artifacts.ReviewArtifact{Kind: "review", Verdict: "pass"}
	out := CompletionPolicy(false, plan, val, rev, 0, false)
	if out == domain.OutcomeRepair {
		t.Fatalf("pre-existing baseline failure triggered repair: %s", out)
	}
	val2 := &artifacts.ValidationArtifact{Kind: "validation", Checks: []artifacts.ValidationCheck{
		{Name: "go-test", Required: true, Status: string(domain.CheckFail), Baseline: string(domain.CheckPass)},
	}}
	if got := CompletionPolicy(false, plan, val2, rev, 0, false); got != domain.OutcomeRepair {
		t.Fatalf("new regression did not trigger repair: %s", got)
	}
}
