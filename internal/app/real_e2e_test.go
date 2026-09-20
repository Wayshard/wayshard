//go:build linux

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/workspace"
)

func realGitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
	)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}
	return string(b)
}

// TestRealHarnessSourceChangingE2E is the Pass 1C-2 milestone: a real installed
// ACP harness (OpenCode) completes a real source-changing task through the full
// Wayshard control plane with secure provider networking. Native/local only.
func TestRealHarnessSourceChangingE2E(t *testing.T) {
	if os.Getenv("WAYSHARD_REAL_HARNESS") != "1" {
		t.Skip("set WAYSHARD_REAL_HARNESS=1 to run the real-harness E2E")
	}
	requireGit(t)
	exe := os.Getenv("WAYSHARD_REAL_HARNESS_EXE")
	if exe == "" {
		p, err := exec.LookPath("opencode")
		if err != nil {
			t.Skip("opencode not on PATH")
		}
		exe = p
	}
	model := os.Getenv("WAYSHARD_REAL_HARNESS_MODEL")
	if model == "" {
		model = "opencode/mimo-v2.5-free"
	}

	canary := "WS_CTX_CANARY_" + time.Now().Format("150405")
	src := t.TempDir()
	initRepo(t, src, map[string]string{
		"keep.txt":    "baseline keep\n",
		"tracked.txt": "tracked base\n",
		"AGENTS.md":   "# Project instructions\n\nPROJECT_CANARY: " + canary + "\n",
	})
	// Staged user change.
	if err := os.WriteFile(filepath.Join(src, "staged.txt"), []byte("user staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	realGitRun(t, src, "add", "staged.txt")
	// Staged modification of a tracked file.
	if err := os.WriteFile(filepath.Join(src, "tracked.txt"), []byte("tracked staged edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	realGitRun(t, src, "add", "tracked.txt")
	// Unstaged modification of a tracked file.
	if err := os.WriteFile(filepath.Join(src, "keep.txt"), []byte("baseline keep\nunstaged user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Untracked file.
	if err := os.WriteFile(filepath.Join(src, "untracked.txt"), []byte("user untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagedBefore := realGitRun(t, src, "diff", "--cached")
	keepBefore, _ := os.ReadFile(filepath.Join(src, "keep.txt"))
	untrackedBefore, _ := os.ReadFile(filepath.Join(src, "untracked.txt"))

	objective := "Create a new file named AGENT_RESULT.txt in the repository root. " +
		"It must contain exactly two lines: first the value of PROJECT_CANARY found in the project instructions (AGENTS.md), " +
		"then the line WAYSHARD_REAL_E2E_OK. Do not modify any other file."

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	// Discovery must find OpenCode from the normal search path; no candidate is
	// injected. To keep this test focused on the independently audited OpenCode
	// path, disable the other shipped harnesses through the user catalog (the
	// supported CRUD interface) so a second installed provider harness cannot be
	// selected ahead of OpenCode.
	t.Setenv("PATH", filepath.Dir(exe)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "wayshard"), 0o755); err != nil {
		t.Fatal(err)
	}
	var cat strings.Builder
	cat.WriteString("schema_version = 1\n")
	for _, id := range []string{"codex", "claude", "grok", "gemini", "github-copilot", "cursor", "kiro",
		"junie", "goose", "cline", "qwen", "qoder", "mistral-vibe", "devin", "kilo", "factory-droid",
		"auggie", "amp", "pi", "omp", "wayshard-fake-acp"} {
		fmt.Fprintf(&cat, "[[harness]]\nid = %q\nenabled = false\n\n", id)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "wayshard", "harnesses.toml"), []byte(cat.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Open(ctx, Config{
		DataDir: t.TempDir(), Listen: "127.0.0.1:0",
		AllowProviderNetwork: true,
		ProviderDestinations: []domain.ProviderDestination{
			{Host: "opencode.ai", Port: 443},
			{Host: "models.opencode.ai", Port: 443},
		},
		ProviderModel: model,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	// Confirm discovery produced a route-viable OpenCode candidate.
	cands, _ := a.Engine.Candidates.Candidates(ctx)
	foundReady := false
	for _, c := range cands {
		t.Logf("discovered candidate: %s/%s health=%s network=%s transport=%s model=%s",
			c.Harness.DefinitionID, c.Harness.Executable, c.Harness.Health, c.Network, c.ProviderTransport, c.ModelID)
		if c.Harness.DefinitionID == "opencode" && c.Network == domain.NetworkProvider &&
			c.ProviderTransport == domain.TransportHTTPProxy && c.ModelID == model {
			foundReady = true
		}
	}
	if !foundReady {
		t.Fatalf("discovery did not produce a route-viable OpenCode candidate: %+v", cands)
	}
	spy := &preIntegrateCheck{inner: a.Engine.Integrate, src: src}
	a.Engine.Integrate = spy

	p := &domain.Project{Name: "real-e2e", Path: src, SourceKind: "git"}
	if err := a.Store.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "real"}
	_ = a.Store.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: objective}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: objective}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := a.Store.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- a.Engine.ProcessRun(ctx, run.ID) }()
	// Simulate an authenticated user resolving approval requests through the
	// real durable approval path. Nothing is auto-approved by the server.
	stopApprovals := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopApprovals:
				return
			case <-ticker.C:
				apps, _ := a.Store.ListPendingApprovals(ctx)
				for _, ap := range apps {
					t.Logf("user resolving approval %s: %s", ap.ID, ap.Resource)
					_ = a.Store.ResolveApproval(context.WithoutCancel(ctx), ap.ID, "allowed", "e2e-user")
				}
			}
		}
	}()
	defer close(stopApprovals)
	progress := time.NewTicker(20 * time.Second)
	defer progress.Stop()
	var runErr error
loop:
	for {
		select {
		case runErr = <-done:
			break loop
		case <-progress.C:
			r, _ := a.Store.GetRun(ctx, run.ID)
			stages, _ := a.Store.ListStages(ctx, run.ID)
			t.Logf("progress: status=%s stages=%d", r.Status, len(stages))
			for _, st := range stages {
				atts, _ := a.Store.ListAttempts(ctx, st.ID)
				t.Logf("  stage %s %s attempts=%d", st.Kind, st.Status, len(atts))
			}
		case <-ctx.Done():
			runErr = ctx.Err()
			break loop
		}
	}
	if runErr != nil {
		dumpRunState(t, a, run.ID)
		t.Fatalf("ProcessRun: %v", runErr)
	}
	if !spy.sourceClean {
		t.Fatal("source contained AGENT_RESULT.txt before integration")
	}
	got, _ := a.Store.GetRun(ctx, run.ID)
	t.Logf("run status=%s reason=%s detail=%s", got.Status, got.BlockedReason, got.BlockedDetail)

	// Source-safety: intended agent change present; user state preserved.
	agentBody, err := os.ReadFile(filepath.Join(src, "AGENT_RESULT.txt"))
	if err != nil {
		t.Fatalf("AGENT_RESULT.txt missing after integration: %v (run=%s %s)", err, got.Status, got.BlockedDetail)
	}
	t.Logf("AGENT_RESULT.txt:\n%s", string(agentBody))
	if !strings.Contains(string(agentBody), canary) || !strings.Contains(string(agentBody), "WAYSHARD_REAL_E2E_OK") {
		t.Fatalf("agent file missing canary/marker: %q", string(agentBody))
	}
	keepAfter, _ := os.ReadFile(filepath.Join(src, "keep.txt"))
	if string(keepAfter) != string(keepBefore) {
		t.Fatalf("pre-existing unstaged edit was overwritten: %q", string(keepAfter))
	}
	untrackedAfter, _ := os.ReadFile(filepath.Join(src, "untracked.txt"))
	if string(untrackedAfter) != string(untrackedBefore) {
		t.Fatalf("untracked user file changed: %q", string(untrackedAfter))
	}
	stagedAfter := realGitRun(t, src, "diff", "--cached")
	if stagedAfter != stagedBefore {
		t.Fatalf("pre-existing staged state changed:\nbefore:\n%s\nafter:\n%s", stagedBefore, stagedAfter)
	}
	if !strings.Contains(stagedAfter, "staged.txt") || !strings.Contains(stagedAfter, "tracked staged edit") {
		t.Fatalf("pre-existing staged state lost:\n%s", stagedAfter)
	}
	// No auto-stage of the agent file.
	if strings.Contains(stagedAfter, "AGENT_RESULT.txt") {
		t.Fatalf("agent file was auto-staged:\n%s", stagedAfter)
	}

	// RunDelta: agent file only; user files not attributed to the agent.
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
		if f.Path == "AGENT_RESULT.txt" {
			foundAgent = true
		}
		if f.Path == "keep.txt" || f.Path == "untracked.txt" || f.Path == "staged.txt" {
			t.Fatalf("run delta includes pre-existing user file: %s", f.Path)
		}
	}
	if !foundAgent {
		t.Fatalf("run delta missing AGENT_RESULT.txt: %+v", delta.Files)
	}

	if got.Status != domain.RunComplete {
		t.Fatalf("expected COMPLETE, got %s (%s)", got.Status, got.BlockedDetail)
	}
	stages, _ := a.Store.ListStages(ctx, run.ID)
	seen := map[domain.StageKind]bool{}
	for _, st := range stages {
		seen[st.Kind] = true
	}
	for _, k := range []domain.StageKind{domain.StagePlan, domain.StageExecute, domain.StageValidate, domain.StageReview} {
		if !seen[k] {
			t.Fatalf("missing stage %s", k)
		}
	}

	// Cleanup: no active owners/approvals/running attempts, no harness processes.
	if owners, _ := a.Store.ListActiveProcessOwners(ctx); len(owners) != 0 {
		t.Fatalf("active process owners remain: %+v", owners)
	}
	if apps, _ := a.Store.ListPendingApprovals(ctx); len(apps) != 0 {
		t.Fatalf("pending approvals remain: %+v", apps)
	}
	for _, st := range stages {
		if st.Status == domain.AttemptRunning {
			t.Fatalf("stage still running: %s", st.Kind)
		}
		atts, _ := a.Store.ListAttempts(ctx, st.ID)
		for _, at := range atts {
			if at.Status == domain.AttemptRunning {
				t.Fatalf("attempt still running: %s", at.ID)
			}
		}
	}
	if out, _ := exec.Command("pgrep", "-f", "--", "--wayshard-provider-shim").Output(); len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("provider shim still alive: %s", out)
	}
}

func dumpRunState(t *testing.T, a *App, runID string) {
	t.Helper()
	ctx := context.Background()
	r, err := a.Store.GetRun(ctx, runID)
	if err != nil {
		t.Logf("run: %v", err)
		return
	}
	t.Logf("run status=%s reason=%s detail=%s", r.Status, r.BlockedReason, r.BlockedDetail)
	stages, _ := a.Store.ListStages(ctx, runID)
	for _, st := range stages {
		atts, _ := a.Store.ListAttempts(ctx, st.ID)
		t.Logf("stage %s ordinal=%d status=%s attempts=%d", st.Kind, st.Ordinal, st.Status, len(atts))
		for _, at := range atts {
			t.Logf("  attempt %s status=%s class=%s detail=%s", at.ID, at.Status, at.FailureClass, at.Error)
		}
	}
	arts, _ := a.Store.ListArtifacts(ctx, runID)
	for _, ar := range arts {
		t.Logf("artifact kind=%s valid=%v json=%.300s", ar.Kind, ar.Valid, ar.JSON)
	}
}
