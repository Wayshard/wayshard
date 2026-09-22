package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/testutil"
	"github.com/Wayshard/wayshard/internal/workspace"
)

func TestSourceChangingOrchestrationThroughIntegrate(t *testing.T) {
	testutil.RequireNativeIsolation(t)
	requireGit(t)
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "success")
	t.Setenv("WAYSHARD_FAKE_WRITE_FILE", "agent.go")

	src := t.TempDir()
	initRepo(t, src, map[string]string{"keep.go": "package keep\n"})
	if err := os.WriteFile(filepath.Join(src, "user.txt"), []byte("pre-existing user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeKeep, _ := os.ReadFile(filepath.Join(src, "keep.go"))
	beforeUser, _ := os.ReadFile(filepath.Join(src, "user.txt"))

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	p := &domain.Project{Name: "srcproj", Path: src, SourceKind: "git"}
	if err := a.Store.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "work"}
	if err := a.Store.InsertConversation(ctx, c); err != nil {
		t.Fatal(err)
	}
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "add agent.go"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "add agent.go", ArtifactOnly: false}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := a.Store.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}

	origInt := a.Engine.Integrate
	spy := &preIntegrateCheck{inner: origInt, src: src}
	a.Engine.Integrate = spy

	if err := a.Engine.ProcessRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if !spy.sourceClean {
		t.Fatal("source already contained agent.go before integration")
	}
	got, err := a.Store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.RunComplete {
		t.Fatalf("status=%s reason=%s detail=%s", got.Status, got.BlockedReason, got.BlockedDetail)
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

	deltaJSON, err := a.Store.LatestRunDelta(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var delta workspace.Delta
	if err := json.Unmarshal([]byte(deltaJSON), &delta); err != nil {
		t.Fatal(err)
	}
	for _, f := range delta.Files {
		if f.Path == "user.txt" && f.AgentModified {
			t.Fatal("user.txt attributed to agent")
		}
	}
	foundAgent := false
	for _, f := range delta.AgentFiles() {
		if f.Path == "agent.go" {
			foundAgent = true
		}
		if f.Path == "user.txt" {
			t.Fatal("run delta includes user file")
		}
	}
	if !foundAgent {
		t.Fatalf("run delta missing agent.go: %+v", delta.Files)
	}

	in, err := a.Store.GetIntegrationByRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if in.Status != "published" && in.Status != "complete" {
		// integration package uses StatusPublished
		if in.Status != "published" {
			t.Fatalf("integration status %s err=%s", in.Status, in.Error)
		}
	}
	journal, err := a.Store.ListJournal(ctx, in.ID)
	if err == nil && len(journal) == 0 {
		// file journal is also acceptable
		jdir := filepath.Join(a.Store.Root, "runtime", "journals", run.ID)
		if _, err := os.Stat(jdir); err != nil {
			t.Fatalf("no journal rows and no journal dir: %v", err)
		}
	}

	stages, err := a.Store.ListStages(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[domain.StageKind]bool{}
	for _, st := range stages {
		seen[st.Kind] = true
	}
	for _, k := range []domain.StageKind{domain.StagePlan, domain.StageExecute, domain.StageValidate, domain.StageReview} {
		if !seen[k] {
			t.Fatalf("missing stage %s in %+v", k, stages)
		}
	}
	arts, _ := a.Store.ListArtifacts(ctx, run.ID)
	if len(arts) < 3 {
		t.Fatalf("expected plan/impl/review artifacts, got %d", len(arts))
	}
	ev, _ := a.Store.EventsSince(ctx, 0, p.ID, run.ID, 500)
	if len(ev) == 0 {
		t.Fatal("no durable events")
	}
}

func TestOrchestratorIntegrationConflictBlocks(t *testing.T) {
	testutil.RequireNativeIsolation(t)
	requireGit(t)
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "success")
	t.Setenv("WAYSHARD_FAKE_WRITE_FILE", "agent.go")

	src := t.TempDir()
	initRepo(t, src, map[string]string{"keep.go": "package keep\n"})

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	p := &domain.Project{Name: "conflict", Path: src, SourceKind: "git"}
	_ = a.Store.InsertProject(ctx, p)
	c := &domain.Conversation{ProjectID: p.ID, Title: "c"}
	_ = a.Store.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "add agent.go"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "add agent.go"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	_ = a.Store.CreateTaskRun(ctx, msg, task, run)

	// Race the source tree: after a short delay, write a conflicting agent.go
	// while the run is in flight. ProcessRun is synchronous, so instead we
	// intercept at ready_to_integrate by wrapping integrate via a pre-seeded
	// conflicting file after execute by using a custom loop.
	eng := a.Engine
	orig := eng.Integrate
	eng.Integrate = conflictBefore{inner: orig, src: src}

	if err := eng.ProcessRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := a.Store.GetRun(ctx, run.ID)
	if got.Status != domain.RunIntegrationBlocked {
		t.Fatalf("status=%s reason=%s detail=%s", got.Status, got.BlockedReason, got.BlockedDetail)
	}
	if got.BlockedReason != domain.BlockedIntegration {
		t.Fatalf("reason %s", got.BlockedReason)
	}
	gotBody, _ := os.ReadFile(filepath.Join(src, "agent.go"))
	if !strings.Contains(string(gotBody), "concurrent user add") {
		t.Fatalf("failed integrate mutated source: %s", gotBody)
	}
	if strings.Contains(string(gotBody), "fake ACP") {
		t.Fatal("agent content published despite conflict")
	}
}

type preIntegrateCheck struct {
	inner interface {
		Integrate(context.Context, domain.Run, *domain.WorkspaceRecord) (*domain.Integration, error)
	}
	src         string
	sourceClean bool
}

func (p *preIntegrateCheck) Integrate(ctx context.Context, run domain.Run, ws *domain.WorkspaceRecord) (*domain.Integration, error) {
	_, err := os.Stat(filepath.Join(p.src, "agent.go"))
	p.sourceClean = os.IsNotExist(err)
	return p.inner.Integrate(ctx, run, ws)
}

type conflictBefore struct {
	inner interface {
		Integrate(context.Context, domain.Run, *domain.WorkspaceRecord) (*domain.Integration, error)
	}
	src string
}

func (c conflictBefore) Integrate(ctx context.Context, run domain.Run, ws *domain.WorkspaceRecord) (*domain.Integration, error) {
	if err := os.WriteFile(filepath.Join(c.src, "agent.go"), []byte("package user\n// concurrent user add\n"), 0o644); err != nil {
		return nil, err
	}
	return c.inner.Integrate(ctx, run, ws)
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func initRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t",
			"GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_CONFIG_SYSTEM="+os.DevNull,
		)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	run("config", "commit.gpgsign", "false")
	run("config", "core.autocrlf", "false")
	for p, body := range files {
		fp := filepath.Join(dir, p)
		if err := os.WriteFile(fp, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "-A")
	run("commit", "-m", "init")
}
