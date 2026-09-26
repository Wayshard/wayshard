package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// TestNativeTrustedLocalExecution exercises the real product path end to end on
// the host OS using the deterministic wayshard-fake-acp harness:
//
//	discovery -> ACP initialize -> PLAN -> EXECUTE -> VALIDATE -> REVIEW ->
//	INTEGRATE -> COMPLETE
//
// It proves Execute changes only the isolated run workspace from Wayshard's point
// of view (the source tree is untouched before integration), validation runs,
// integration publishes only the run delta, and durable SQLite state
// (stages/attempts/artifacts/route decisions/events) is correct.
func TestNativeTrustedLocalExecution(t *testing.T) {
	requireGit(t)
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "success")
	t.Setenv("WAYSHARD_FAKE_WRITE_FILE", "agent.go")

	src := t.TempDir()
	initRepo(t, src, map[string]string{"keep.go": "package keep\n"})
	// Pre-existing untracked user edit that must survive as user baseline.
	if err := os.WriteFile(filepath.Join(src, "user.txt"), []byte("user baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeKeep, _ := os.ReadFile(filepath.Join(src, "keep.go"))
	beforeUser, _ := os.ReadFile(filepath.Join(src, "user.txt"))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	// Discovery + ACP initialize happen at startup.
	installs, err := a.Store.ListHarnessInstallations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range installs {
		if h.DefinitionID == "wayshard-fake-acp" {
			found = true
			if h.ACPStatus != "ok" {
				t.Fatalf("fake harness acpStatus = %q, want ok", h.ACPStatus)
			}
			if h.Health != domain.HarnessReady {
				t.Fatalf("fake harness health = %s, want ready", h.Health)
			}
		}
	}
	if !found {
		t.Fatalf("discovery did not find wayshard-fake-acp: %+v", installs)
	}

	p := &domain.Project{Name: "native", Path: src, SourceKind: "git"}
	if err := a.Store.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "native"}
	if err := a.Store.InsertConversation(ctx, c); err != nil {
		t.Fatal(err)
	}
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "add agent.go"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "add agent.go"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := a.Store.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}

	spy := &preIntegrateCheck{inner: a.Engine.Integrate, src: src}
	a.Engine.Integrate = spy

	if err := a.Engine.ProcessRun(ctx, run.ID); err != nil {
		t.Fatalf("ProcessRun: %v", err)
	}
	if !spy.sourceClean {
		t.Fatal("source tree changed before integration: Execute escaped the run workspace")
	}

	got, err := a.Store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.RunComplete {
		t.Fatalf("run status = %s reason=%s detail=%s", got.Status, got.BlockedReason, got.BlockedDetail)
	}

	// Run workspace is isolated and holds the agent's change; the source tree
	// keeps its baseline until integration publishes.
	ws, err := a.Store.GetWorkspaceByRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(ws.RunPath) == filepath.Clean(src) {
		t.Fatal("run workspace is the source tree")
	}
	if _, err := os.Stat(filepath.Join(ws.RunPath, "agent.go")); err != nil {
		t.Fatalf("run workspace missing agent.go: %v", err)
	}

	afterKeep, _ := os.ReadFile(filepath.Join(src, "keep.go"))
	afterUser, _ := os.ReadFile(filepath.Join(src, "user.txt"))
	if string(afterKeep) != string(beforeKeep) {
		t.Fatal("keep.go changed; only agent.go should be published")
	}
	if string(afterUser) != string(beforeUser) {
		t.Fatal("pre-existing user.txt was overwritten")
	}
	agent, err := os.ReadFile(filepath.Join(src, "agent.go"))
	if err != nil {
		t.Fatal("agent.go missing from source after integrate")
	}
	if !strings.Contains(string(agent), "fake ACP") {
		t.Fatalf("agent.go content: %s", agent)
	}

	// Validation runs.
	stages, err := a.Store.ListStages(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[domain.StageKind]bool{}
	for _, st := range stages {
		seen[st.Kind] = true
		if st.Status == domain.AttemptRunning {
			t.Fatalf("stage %s still running after COMPLETE", st.Kind)
		}
		atts, _ := a.Store.ListAttempts(ctx, st.ID)
		for _, at := range atts {
			if at.Status == domain.AttemptRunning {
				t.Fatalf("attempt %s/%s still running after COMPLETE", st.Kind, at.ID)
			}
		}
	}
	for _, k := range []domain.StageKind{domain.StagePlan, domain.StageExecute, domain.StageValidate, domain.StageReview} {
		if !seen[k] {
			t.Fatalf("missing stage %s in %+v", k, stages)
		}
	}

	// Durable artifacts, route decisions and events.
	arts, _ := a.Store.ListArtifacts(ctx, run.ID)
	if len(arts) < 3 {
		t.Fatalf("expected plan/implementation/review artifacts, got %d", len(arts))
	}
	rds, err := a.Store.ListRouteDecisions(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rds) == 0 {
		t.Fatal("no RouteDecision persisted")
	}
	for _, rd := range rds {
		if rd.HarnessID == "" {
			t.Fatalf("RouteDecision without a harness: %+v", rd)
		}
	}
	ev, _ := a.Store.EventsSince(ctx, 0, p.ID, run.ID, 1000)
	if len(ev) == 0 {
		t.Fatal("no durable events recorded")
	}

	// Integration published the run delta, and the delta excludes user baseline.
	in, err := a.Store.GetIntegrationByRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if in.Status != "published" {
		t.Fatalf("integration status = %s (err=%s), want published", in.Status, in.Error)
	}
	deltaJSON, err := a.Store.LatestRunDelta(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var delta workspace.Delta
	if err := json.Unmarshal([]byte(deltaJSON), &delta); err != nil {
		t.Fatal(err)
	}
	foundAgent := false
	for _, f := range delta.AgentFiles() {
		if f.Path == "agent.go" {
			foundAgent = true
		}
		if f.Path == "user.txt" {
			t.Fatal("run delta attributes pre-existing user.txt to the agent")
		}
	}
	if !foundAgent {
		t.Fatalf("run delta missing agent.go: %+v", delta.Files)
	}

	if err := a.Store.IntegrityCheck(ctx); err != nil {
		t.Fatalf("store integrity after run: %v", err)
	}
}

// TestNativeRoutingOnHealthAndCapability proves route viability depends on
// harness health/negotiated capability and policy: with the fake harness
// discovered and ready, candidate enumeration and routing succeed with a zero
// routing config.
func TestNativeRoutingOnHealthAndCapability(t *testing.T) {
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "success")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	cands, err := a.Engine.Candidates.Candidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("no candidates: routing depends on harness health and capability")
	}
	var fake bool
	for _, c := range cands {
		if c.Harness.DefinitionID == "wayshard-fake-acp" && c.Harness.Health == domain.HarnessReady {
			fake = true
		}
	}
	if !fake {
		t.Fatalf("ready fake harness not routable: %+v", cands)
	}

	dec := (&routing.Router{}).Route(ctx, domain.StageExecute, routing.Config{}, cands, nil)
	if dec.Blocked != "" {
		t.Fatalf("routing blocked for a ready harness: %s %s", dec.Blocked, dec.Detail)
	}
	if dec.Candidate.Harness.DefinitionID != "wayshard-fake-acp" {
		t.Fatalf("routed to unexpected harness: %+v", dec.Candidate.Harness)
	}
	if dec.Candidate.ModelID != "" {
		// Model selection is harness-owned; nothing forces a provider model.
		t.Logf("routed model (harness-advertised): %s", dec.Candidate.ModelID)
	}
}
