package gitutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if !Available() {
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

func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@wayshard.dev")
	git(t, dir, "config", "user.name", "Wayshard")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "README")
	git(t, dir, "commit", "-m", "init")
}

func TestDiscoverHEADBranchStatus(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	dir := t.TempDir()
	initRepo(t, dir)

	repo, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if repo.WorkTree != dir && filepath.Clean(repo.WorkTree) != filepath.Clean(dir) {
		t.Fatalf("worktree = %s want %s", repo.WorkTree, dir)
	}
	head, err := repo.HEAD(ctx)
	if err != nil || len(head) < 7 {
		t.Fatalf("HEAD = %q err=%v", head, err)
	}
	branch, err := repo.Branch(ctx)
	if err != nil || branch != "main" {
		t.Fatalf("branch = %q err=%v", branch, err)
	}
	st, err := repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 0 {
		t.Fatalf("clean status = %+v", st)
	}

	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("u\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err = repo.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]StatusEntry{}
	for _, e := range st {
		found[e.Path] = e
	}
	if e, ok := found["README"]; !ok || !e.Unstaged() {
		t.Fatalf("README status = %+v", e)
	}
	if e, ok := found["new.txt"]; !ok || !e.Untracked() {
		t.Fatalf("new.txt status = %+v", found["new.txt"])
	}
	if _, ok := found["ignored.bin"]; ok {
		t.Fatal("ignored.bin should not appear in status")
	}

	before := git(t, dir, "status", "--porcelain")
	dest := t.TempDir()
	cap, err := repo.CaptureDirty(ctx, dest, filepath.Join(dest, ".staged"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cap.Entries) == 0 {
		t.Fatal("expected dirty entries")
	}
	if _, err := os.Stat(filepath.Join(dest, "README")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "new.txt")); err != nil {
		t.Fatal(err)
	}
	after := git(t, dir, "status", "--porcelain")
	if before != after {
		t.Fatalf("capture mutated source status\nbefore=%q\nafter=%q", before, after)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "README"))
	if string(got) != "dirty\n" {
		t.Fatalf("source README mutated: %q", got)
	}
}

func TestDiscoverNotRepo(t *testing.T) {
	requireGit(t)
	_, err := Discover(t.TempDir())
	if err != ErrNotRepo {
		t.Fatalf("err = %v", err)
	}
}

func TestParsePorcelainZRename(t *testing.T) {
	raw := []byte("R  old.txt\x00new.txt\x00 M tracked.txt\x00?? untracked.txt\x00")
	got, err := parsePorcelainZ(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Path != "new.txt" || got[0].OrigPath != "old.txt" || got[0].Index != 'R' {
		t.Fatalf("rename = %+v", got[0])
	}
	if !got[1].Unstaged() || got[1].Path != "tracked.txt" {
		t.Fatalf("unstaged = %+v", got[1])
	}
	if !got[2].Untracked() {
		t.Fatalf("untracked = %+v", got[2])
	}
}
