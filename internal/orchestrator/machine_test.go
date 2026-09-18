package orchestrator

import (
	"context"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/jev"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/storage"
)

type stubExec struct{}

func (stubExec) Execute(ctx context.Context, req StageRequest) (StageResult, error) {
	body, err := syntheticArtifact(req.Stage.Kind, req.Task.Objective)
	return StageResult{ArtifactJSON: body}, err
}

func TestFullSyntheticRunArtifactOnly(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := st.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	if err := st.InsertConversation(ctx, c); err != nil {
		t.Fatal(err)
	}
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "brainstorm architecture"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "brainstorm architecture", ArtifactOnly: true}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := st.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Store:  st,
		Jev:    jev.DeterministicEngine{},
		Router: &routing.Router{Engine: jev.DeterministicEngine{}},
		Exec:   stubExec{},
		Candidates: candidateList{{
			Harness: domain.HarnessInstallation{ID: "fake", DisplayName: "fake", Health: domain.HarnessReady, Compatibility: domain.CompatRoutable, Isolation: domain.IsolationOuterOnly},
			ModelID: "fake",
		}},
	}
	if err := eng.ProcessRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.RunComplete {
		t.Fatalf("status=%s detail=%s", got.Status, got.BlockedDetail)
	}
	arts, _ := st.ListArtifacts(ctx, run.ID)
	if len(arts) == 0 {
		t.Fatal("expected artifacts")
	}
}

type candidateList []routing.Candidate

func (c candidateList) Candidates(context.Context) ([]routing.Candidate, error) { return c, nil }

func TestNoViableRouteBlocks(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	_ = st.InsertProject(ctx, p)
	c := &domain.Conversation{ProjectID: p.ID}
	_ = st.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "x"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "x"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	_ = st.CreateTaskRun(ctx, msg, task, run)
	eng := &Engine{Store: st, Jev: jev.DeterministicEngine{}, Router: &routing.Router{}, Exec: stubExec{}, Candidates: candidateList{}}
	if err := eng.ProcessRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(ctx, run.ID)
	if got.Status != domain.RunBlocked || got.BlockedReason != domain.BlockedNoViableRoute {
		t.Fatalf("got %s %s", got.Status, got.BlockedReason)
	}
}

func TestFailedAttemptNotRewritten(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	_ = st.InsertProject(ctx, p)
	stage := &domain.Stage{RunID: "r", Kind: domain.StageExecute, Ordinal: 1}
	// insert run first
	c := &domain.Conversation{ProjectID: p.ID}
	_ = st.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "x"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "x"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID}
	_ = st.CreateTaskRun(ctx, msg, task, run)
	stage.RunID = run.ID
	_ = st.AppendStage(ctx, stage)
	a1 := &domain.StageAttempt{StageID: stage.ID, RunID: run.ID, Ordinal: 1, Status: domain.AttemptFailed, Error: "crash"}
	_ = st.AppendAttempt(ctx, a1)
	a2 := &domain.StageAttempt{StageID: stage.ID, RunID: run.ID, Ordinal: 2, Status: domain.AttemptSucceeded}
	_ = st.AppendAttempt(ctx, a2)
	list, err := st.ListAttempts(ctx, stage.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Status != domain.AttemptFailed || list[1].Status != domain.AttemptSucceeded {
		t.Fatalf("%+v", list)
	}
}
