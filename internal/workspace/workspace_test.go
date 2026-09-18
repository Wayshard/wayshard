package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/gitutil"
)

func requireGit(t *testing.T) {
	t.Helper()
	if !gitutil.Available() {
		t.Skip("git binary not found")
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Wayshard",
		"GIT_AUTHOR_EMAIL=test@wayshard.dev",
		"GIT_COMMITTER_NAME=Wayshard",
		"GIT_COMMITTER_EMAIL=test@wayshard.dev",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func initRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@wayshard.dev")
	git(t, dir, "config", "user.name", "Wayshard")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "config", "core.autocrlf", "false")
	writeAll(t, dir, files)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", "init")
}

func writeAll(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		fp := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fp, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDirtyBaselineNotAttributedToAgent(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{
		"keep.go":  "package keep\n",
		"dirty.go": "package dirty\n// original\n",
	})
	if err := os.WriteFile(filepath.Join(src, ".gitignore"), []byte("ignore.me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, src, "add", ".gitignore")
	git(t, src, "commit", "-m", "ignore")
	if err := os.WriteFile(filepath.Join(src, "ignore.me"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(src, "dirty.go"), []byte("package dirty\n// user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "untracked.go"), []byte("package untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceStatus := git(t, src, "status", "--porcelain")

	b, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	if b.Kind() != KindGit {
		t.Fatalf("kind = %s", b.Kind())
	}
	snap, err := b.CaptureSnapshot(ctx, CaptureOptions{
		SnapshotDir:  filepath.Join(root, "snap"),
		KnowledgeRev: "krev1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Branch != "main" || snap.HEAD == "" {
		t.Fatalf("branch/HEAD = %s %s", snap.Branch, snap.HEAD)
	}
	if snap.KnowledgeRev != "krev1" {
		t.Fatalf("knowledge = %s", snap.KnowledgeRev)
	}
	if git(t, src, "status", "--porcelain") != sourceStatus {
		t.Fatal("capture mutated source git status")
	}
	gotDirty, _ := os.ReadFile(filepath.Join(src, "dirty.go"))
	if string(gotDirty) != "package dirty\n// user edit\n" {
		t.Fatal("capture mutated dirty.go")
	}
	if _, ok := snap.Files["ignore.me"]; ok {
		t.Fatal("gitignore file should not be in snapshot")
	}
	if _, ok := snap.Files["untracked.go"]; !ok {
		t.Fatal("meaningful untracked missing from snapshot")
	}
	pre := snap.DirtySet()
	if _, ok := pre["dirty.go"]; !ok {
		t.Fatalf("dirty.go should be pre-existing, dirty=%+v", snap.Dirty)
	}
	if _, ok := pre["untracked.go"]; !ok {
		t.Fatal("untracked.go should be pre-existing")
	}

	runDir := filepath.Join(root, "run")
	if err := b.Materialize(ctx, snap, runDir); err != nil {
		t.Fatal(err)
	}
	runDirty, _ := os.ReadFile(filepath.Join(runDir, "dirty.go"))
	if string(runDirty) != "package dirty\n// user edit\n" {
		t.Fatalf("run workspace lost user baseline: %q", runDirty)
	}
	if _, err := os.Stat(filepath.Join(runDir, "untracked.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "agent.go")); !os.IsNotExist(err) {
		t.Fatal("source should not have agent.go")
	}

	if err := os.WriteFile(filepath.Join(runDir, "agent.go"), []byte("package agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "keep.go"), []byte("package keep\n// agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	delta, err := ComputeDelta(snap, runDir)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]FileDelta{}
	for _, f := range delta.Files {
		byPath[f.Path] = f
	}
	if _, ok := byPath["dirty.go"]; ok {
		t.Fatalf("pre-existing dirty.go attributed to agent: %+v", byPath["dirty.go"])
	}
	if _, ok := byPath["untracked.go"]; ok {
		t.Fatalf("pre-existing untracked.go attributed to agent")
	}
	if f, ok := byPath["agent.go"]; !ok || !f.AgentModified || f.Kind != ChangeAdded {
		t.Fatalf("agent.go delta = %+v", f)
	}
	if f, ok := byPath["keep.go"]; !ok || !f.AgentModified || f.PreExisting {
		t.Fatalf("keep.go delta = %+v", f)
	}
}

func TestStagedAndUnstagedRestoredInRunWorkspace(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"file.go": "head\n"})
	if err := os.WriteFile(filepath.Join(src, "file.go"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, src, "add", "file.go")
	if err := os.WriteFile(filepath.Join(src, "file.go"), []byte("unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := b.CaptureSnapshot(ctx, CaptureOptions{SnapshotDir: filepath.Join(root, "snap")})
	if err != nil {
		t.Fatal(err)
	}
	d := snap.DirtySet()["file.go"]
	if !d.Staged || !d.Unstaged {
		t.Fatalf("dirty entry = %+v", d)
	}
	runDir := filepath.Join(root, "run")
	if err := b.Materialize(ctx, snap, runDir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(runDir, "file.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "unstaged\n" {
		t.Fatalf("worktree = %q", got)
	}
	cached := git(t, runDir, "show", ":file.go")
	if cached != "staged\n" {
		t.Fatalf("index = %q", cached)
	}
}

func TestFilesystemBackendSnapshotAndDelta(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	writeAll(t, src, map[string]string{
		"a.txt": "A\n",
		"b.txt": "B\n",
	})
	if err := os.Symlink("a.txt", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	b, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	if b.Kind() != KindFilesystem {
		t.Fatalf("kind = %s", b.Kind())
	}
	snap, err := b.CaptureSnapshot(ctx, CaptureOptions{SnapshotDir: filepath.Join(root, "snap")})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Files["link"].Type != TypeSymlink || snap.Files["link"].Target != "a.txt" {
		t.Fatalf("symlink meta = %+v", snap.Files["link"])
	}
	runDir := filepath.Join(root, "run")
	if err := b.Materialize(ctx, snap, runDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "a.txt"), []byte("A-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	delta, err := ComputeDelta(snap, runDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(delta.AgentFiles()) != 1 || delta.AgentFiles()[0].Path != "a.txt" {
		t.Fatalf("delta = %+v", delta.Files)
	}
	srcA, _ := os.ReadFile(filepath.Join(src, "a.txt"))
	if string(srcA) != "A\n" {
		t.Fatalf("source mutated: %q", srcA)
	}
}

func TestSafeRelRejectsGitAndDotDot(t *testing.T) {
	if _, err := SafeRel("../x"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := SafeRel(".git/config"); err == nil {
		t.Fatal("expected .git error")
	}
	if _, err := SafeRel("/abs"); err == nil {
		t.Fatal("expected abs error")
	}
	got, err := SafeRel("pkg/a.go")
	if err != nil || got != "pkg/a.go" {
		t.Fatalf("got %s %v", got, err)
	}
}
