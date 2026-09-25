package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/jev"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/storage"
)

type recordExec struct {
	mu      sync.Mutex
	bundles map[domain.StageKind]string
}

func (r *recordExec) Execute(_ context.Context, req StageRequest) (StageResult, error) {
	r.mu.Lock()
	if r.bundles == nil {
		r.bundles = map[domain.StageKind]string{}
	}
	r.bundles[req.Stage.Kind] = req.Bundle
	r.mu.Unlock()
	switch req.Stage.Kind {
	case domain.StagePlan, domain.StageReplan:
		b, _ := artifacts.Marshal(artifacts.PlanArtifact{Kind: "plan", Objective: "o", AcceptanceCriteria: []string{"AC-CANARY"}, ValidationPlan: []string{"true"}})
		return StageResult{ArtifactJSON: b}, nil
	case domain.StageExecute, domain.StageRepair:
		b, _ := artifacts.Marshal(artifacts.ImplementationReport{Kind: "implementation_report", Summary: "impl"})
		return StageResult{ArtifactJSON: b}, nil
	case domain.StageReview:
		b, _ := artifacts.Marshal(artifacts.ReviewArtifact{Kind: "review", Verdict: "pass"})
		return StageResult{ArtifactJSON: b}, nil
	default:
		b, _ := artifacts.Marshal(artifacts.InvestigationArtifact{Kind: "investigation", Question: "q"})
		return StageResult{ArtifactJSON: b}, nil
	}
}

// TestStageContextIsInjected proves the real orchestration path delivers a
// non-empty, stage-specific bundle (including project instructions and prior
// artifacts) rather than only the task objective.
func TestStageContextIsInjected(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "AGENTS.md"), []byte("# Rules\nPROJECT-INSTRUCTION-CANARY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &domain.Project{Name: "p", Path: src, SourceKind: "filesystem"}
	if err := st.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	_ = st.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "add a thing"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "add a thing", ArtifactOnly: true}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := st.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	insertTestWorkspace(t, st, run, src)

	rec := &recordExec{}
	eng := &Engine{
		Store:         st,
		Jev:           jev.DeterministicEngine{},
		Router:        &routing.Router{},
		Exec:          rec,
		Context:       nil,
		ContextBudget: 12000,
		Candidates: candidateList{{
			Harness: domain.HarnessInstallation{ID: "fake", DisplayName: "fake", Health: domain.HarnessReady, Compatibility: domain.CompatRoutable},
			ModelID: "fake",
		}},
	}
	if err := eng.ProcessRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	rec.mu.Lock()
	bundles := map[domain.StageKind]string{}
	for k, v := range rec.bundles {
		bundles[k] = v
	}
	rec.mu.Unlock()

	for _, k := range []domain.StageKind{domain.StagePlan, domain.StageExecute, domain.StageReview} {
		if strings.TrimSpace(bundles[k]) == "" {
			t.Fatalf("stage %s received an empty bundle", k)
		}
	}
	if !strings.Contains(bundles[domain.StagePlan], "PROJECT-INSTRUCTION-CANARY") {
		t.Fatalf("planner bundle missing project instruction:\n%s", bundles[domain.StagePlan])
	}
	if !strings.Contains(bundles[domain.StagePlan], "add a thing") {
		t.Fatalf("planner bundle missing user request")
	}
	if !strings.Contains(bundles[domain.StageExecute], "AC-CANARY") {
		t.Fatalf("executor bundle missing plan acceptance criteria:\n%s", bundles[domain.StageExecute])
	}
	if bundles[domain.StagePlan] == bundles[domain.StageExecute] {
		t.Fatal("planner and executor bundles are identical; context is not stage-specific")
	}
	// A durable manifest of the actual bundle must be persisted.
	arts, _ := st.ListArtifacts(ctx, run.ID)
	found := false
	for _, a := range arts {
		if a.Kind == domain.ArtifactContextManifest {
			found = true
		}
	}
	if !found {
		t.Fatal("no context manifest artifact persisted")
	}
}
