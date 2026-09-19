package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/workspace"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitTry(t, dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

func gitTry(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	b, err := cmd.CombinedOutput()
	return string(b), err
}

func writeContaminatingRepo(t *testing.T, dir string, gitMutations bool) {
	t.Helper()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/contam\n\ngo 1.21\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "main.go"), []byte("package agent\nfunc Add(a, b int) int { return a + b }\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("original\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "deleteme.txt"), []byte("delete me\n"), 0o644))
	git := ""
	if gitMutations {
		git = `	if err := exec.Command("git", "update-ref", "refs/heads/evil", "HEAD").Run(); err != nil { t.Log("git update-ref:", err) }
	if err := exec.Command("git", "config", "wayshard.evil", "1").Run(); err != nil { t.Log("git config:", err) }
`
	}
	test := `package agent
import ("os";"os/exec";"testing")
func TestContaminate(t *testing.T){
	_ = os.WriteFile("generated.txt", []byte("from-validation"), 0o644)
	_ = os.WriteFile("tracked.txt", []byte("mutated-by-validation"), 0o644)
	_ = os.Remove("deleteme.txt")
	_ = os.MkdirAll("buildout/sub", 0o755)
	_ = os.WriteFile("buildout/out.o", []byte("obj"), 0o644)
	_ = os.Symlink("tracked.txt", "vlink")
	_ = os.Chmod("main.go", 0o755)
` + git + `}
`
	must(os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(test), 0o644))
}

func initContamRepo(t *testing.T, dir string) {
	t.Helper()
	requireGit(t)
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "t@t")
	gitRun(t, dir, "config", "user.name", "t")
	writeContaminatingRepo(t, dir, true)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "init")
}

// TestValidationDoesNotContaminateWhenHarnessWritesNothing reproduces the
// second-audit failure: validation writes must not reach RunDelta or source.
func TestValidationDoesNotContaminateWhenHarnessWritesNothing(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("validation sandbox execution is verified on Linux")
	}
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "success")
	t.Setenv("WAYSHARD_FAKE_WRITE_FILE", "")

	src := t.TempDir()
	initContamRepo(t, src)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	runID := startRun(t, a, src, "no-op")

	got, _ := a.Store.GetRun(ctx, runID)
	if got.Status != domain.RunComplete {
		t.Fatalf("status=%s detail=%s", got.Status, got.BlockedDetail)
	}
	assertDeltaEmpty(t, a, runID)
	for _, leaked := range []string{"generated.txt", "buildout", "vlink"} {
		if _, err := os.Stat(filepath.Join(src, leaked)); err == nil {
			t.Fatalf("validation artifact %q leaked into source", leaked)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(src, "tracked.txt")); string(b) != "original\n" {
		t.Fatalf("validation mutated source tracked.txt: %q", b)
	}
	if _, err := os.Stat(filepath.Join(src, "deleteme.txt")); err != nil {
		t.Fatal("validation deleted a source file")
	}
	assertGitIsolated(t, src)
}

// TestValidationDoesNotContaminateAgentDelta runs a controlled agent change
// plus a validation command that writes/modifies/deletes; only the agent change
// may appear.
func TestValidationDoesNotContaminateAgentDelta(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("validation sandbox execution is verified on Linux")
	}
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "success")
	t.Setenv("WAYSHARD_FAKE_WRITE_FILE", "agent.go")

	src := t.TempDir()
	initContamRepo(t, src)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	runID := startRun(t, a, src, "add agent.go")

	got, _ := a.Store.GetRun(ctx, runID)
	if got.Status != domain.RunComplete {
		t.Fatalf("status=%s detail=%s", got.Status, got.BlockedDetail)
	}
	deltaJSON, err := a.Store.LatestRunDelta(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	var delta workspace.Delta
	_ = json.Unmarshal([]byte(deltaJSON), &delta)
	paths := map[string]bool{}
	for _, f := range delta.Files {
		paths[f.Path] = true
	}
	if !paths["agent.go"] {
		t.Fatalf("agent.go missing from delta: %+v", paths)
	}
	if len(paths) != 1 {
		t.Fatalf("delta contains non-agent changes: %+v", paths)
	}
	if _, err := os.Stat(filepath.Join(src, "generated.txt")); err == nil {
		t.Fatal("generated.txt leaked into source")
	}
	if b, _ := os.ReadFile(filepath.Join(src, "tracked.txt")); string(b) != "original\n" {
		t.Fatalf("validation mutated source tracked.txt: %q", b)
	}
	if _, err := os.Stat(filepath.Join(src, "agent.go")); err != nil {
		t.Fatal("agent.go not integrated")
	}
	assertGitIsolated(t, src)
}

// TestValidationFailureDoesNotContaminate proves a validation command that
// writes files and then fails as a NEW regression cannot leak its side effects
// into RunDelta or source.
func TestValidationFailureDoesNotContaminate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("validation sandbox execution is verified on Linux")
	}
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "success")
	t.Setenv("WAYSHARD_FAKE_WRITE_FILE", "agent.go")

	src := t.TempDir()
	initContamRepo(t, src)
	// Baseline passes; final fails once the agent introduces agent.go.
	regressing := `package agent
import ("os";"testing")
func TestContaminate(t *testing.T){
	_ = os.WriteFile("generated.txt", []byte("x"), 0o644)
	if _, err := os.Stat("agent.go"); err == nil { t.Fatal("agent.go present") }
}
`
	if err := os.WriteFile(filepath.Join(src, "main_test.go"), []byte(regressing), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, src, "add", "-A")
	gitRun(t, src, "commit", "-m", "regress")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	runID := startRun(t, a, src, "add agent.go")
	got, _ := a.Store.GetRun(ctx, runID)
	if got.Status != domain.RunBlocked && got.Status != domain.RunFailed {
		t.Fatalf("expected blocked/failed, got %s", got.Status)
	}
	if deltaJSON, err := a.Store.LatestRunDelta(ctx, runID); err == nil {
		var delta workspace.Delta
		_ = json.Unmarshal([]byte(deltaJSON), &delta)
		for _, f := range delta.Files {
			if f.Path == "generated.txt" {
				t.Fatal("validation-generated file entered RunDelta")
			}
		}
	}
	for _, leaked := range []string{"generated.txt", "agent.go"} {
		if _, err := os.Stat(filepath.Join(src, leaked)); err == nil {
			t.Fatalf("blocked run leaked %q into source", leaked)
		}
	}
}

func assertDeltaEmpty(t *testing.T, a *App, runID string) {
	t.Helper()
	deltaJSON, err := a.Store.LatestRunDelta(context.Background(), runID)
	if err != nil {
		// No delta row means no agent change.
		return
	}
	var delta workspace.Delta
	_ = json.Unmarshal([]byte(deltaJSON), &delta)
	if len(delta.Files) != 0 {
		t.Fatalf("RunDelta not empty: %+v", delta.Files)
	}
}

func assertGitIsolated(t *testing.T, src string) {
	t.Helper()
	out := gitRun(t, src, "for-each-ref", "--format=%(refname)", "refs/heads")
	if strings.Contains(out, "evil") {
		t.Fatal("validation mutated source git refs")
	}
	cfg, _ := gitTry(t, src, "config", "--get", "wayshard.evil")
	if strings.TrimSpace(cfg) != "" {
		t.Fatal("validation mutated source git config")
	}
}

func startRun(t *testing.T, a *App, src, objective string) string {
	t.Helper()
	ctx := context.Background()
	p := &domain.Project{Name: "contam", Path: src, SourceKind: "git"}
	if err := a.Store.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "c"}
	if err := a.Store.InsertConversation(ctx, c); err != nil {
		t.Fatal(err)
	}
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: objective}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: objective}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := a.Store.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	if err := a.Engine.ProcessRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	return run.ID
}
