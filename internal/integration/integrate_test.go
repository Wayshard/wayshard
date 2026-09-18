package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/gitutil"
	"github.com/Wayshard/wayshard/internal/workspace"
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

func captureRun(t *testing.T, src string) (workspace.WorkspaceBackend, *workspace.Snapshot, string, string) {
	t.Helper()
	ctx := context.Background()
	root := filepath.Dir(src)
	b, err := workspace.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := b.CaptureSnapshot(ctx, workspace.CaptureOptions{SnapshotDir: filepath.Join(root, "snap-"+t.Name())})
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(root, "run-"+t.Name())
	if err := b.Materialize(ctx, snap, runDir); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work-"+t.Name())
	return b, snap, runDir, work
}

func TestConcurrentSourceEditDuringRun(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"a.txt": "A\n", "b.txt": "B\n"})
	b, snap, runDir, work := captureRun(t, src)

	if err := os.WriteFile(filepath.Join(runDir, "a.txt"), []byte("A-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "b.txt"), []byte("B-user\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Integrate(ctx, Request{
		RunID:        "run-concurrent",
		Source:       b,
		Snapshot:     snap,
		RunWorkspace: runDir,
		WorkDir:      work,
		JournalDir:   filepath.Join(work, "j"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusPublished {
		t.Fatalf("status = %s reason=%s conflicts=%+v", res.Status, res.Reason, res.Conflicts)
	}
	a, _ := os.ReadFile(filepath.Join(src, "a.txt"))
	bb, _ := os.ReadFile(filepath.Join(src, "b.txt"))
	if string(a) != "A-agent\n" {
		t.Fatalf("a.txt = %q", a)
	}
	if string(bb) != "B-user\n" {
		t.Fatalf("b.txt = %q", bb)
	}
	st := git(t, src, "status", "--porcelain")
	if !strings.Contains(st, "a.txt") {
		t.Fatalf("expected unstaged a.txt in %q", st)
	}
	for _, line := range strings.Split(st, "\n") {
		if !strings.Contains(line, "a.txt") || len(line) < 2 {
			continue
		}
		if line[0] != ' ' && line[0] != '?' {
			t.Fatalf("a.txt was auto-staged: %q", st)
		}
	}
}

func TestBranchSwitchBlocksSilentIntegrate(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"a.txt": "A\n"})
	b, snap, runDir, work := captureRun(t, src)
	if err := os.WriteFile(filepath.Join(runDir, "a.txt"), []byte("A-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, src, "checkout", "-b", "other")
	before, _ := os.ReadFile(filepath.Join(src, "a.txt"))

	res, err := Integrate(ctx, Request{
		RunID:        "run-branch",
		Source:       b,
		Snapshot:     snap,
		RunWorkspace: runDir,
		WorkDir:      work,
		JournalDir:   filepath.Join(work, "j"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusBlocked || res.Reason != ReasonBranchChanged {
		t.Fatalf("status=%s reason=%s", res.Status, res.Reason)
	}
	after, _ := os.ReadFile(filepath.Join(src, "a.txt"))
	if string(after) != string(before) {
		t.Fatalf("source mutated on branch block: %q -> %q", before, after)
	}
	br := strings.TrimSpace(git(t, src, "rev-parse", "--abbrev-ref", "HEAD"))
	if br != "other" {
		t.Fatalf("branch = %s", br)
	}
}

func TestConflictLeavesSourceUntouched(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"a.txt": "base\n"})
	b, snap, runDir, work := captureRun(t, src)
	if err := os.WriteFile(filepath.Join(runDir, "a.txt"), []byte("agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("user\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Integrate(ctx, Request{
		RunID:        "run-conflict",
		Source:       b,
		Snapshot:     snap,
		RunWorkspace: runDir,
		WorkDir:      work,
		JournalDir:   filepath.Join(work, "j"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusBlocked || res.Reason != ReasonConflict {
		t.Fatalf("status=%s reason=%s conflicts=%+v", res.Status, res.Reason, res.Conflicts)
	}
	if len(res.Conflicts) == 0 || res.Conflicts[0].Path != "a.txt" {
		t.Fatalf("conflicts = %+v", res.Conflicts)
	}
	got, _ := os.ReadFile(filepath.Join(src, "a.txt"))
	if string(got) != "user\n" {
		t.Fatalf("source overwritten: %q", got)
	}
}

func TestInterruptedPublicationRecovery(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"one.txt": "1\n", "two.txt": "2\n"})
	b, snap, runDir, work := captureRun(t, src)
	if err := os.WriteFile(filepath.Join(runDir, "one.txt"), []byte("1-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "two.txt"), []byte("2-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Integrate(ctx, Request{
		RunID:        "run-crash",
		Source:       b,
		Snapshot:     snap,
		RunWorkspace: runDir,
		WorkDir:      work,
		JournalDir:   filepath.Join(work, "j"),
		CrashAfter:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusIncomplete {
		t.Fatalf("status = %s reason=%s", res.Status, res.Reason)
	}
	rec, err := ClassifyJournal(ctx, src, res.Journal)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != RecoveryIncomplete {
		t.Fatalf("recovery class = %s files=%+v", rec.Status, rec.Files)
	}

	one, _ := os.ReadFile(filepath.Join(src, "one.txt"))
	two, _ := os.ReadFile(filepath.Join(src, "two.txt"))
	if string(one) == "1-agent\n" && string(two) == "2-agent\n" {
		t.Fatal("crash after 1 still published both files")
	}

	rec2, resume, err := RecoverPublication(ctx, src, res.Journal)
	if err != nil {
		t.Fatal(err)
	}
	if rec2.Status != RecoveryCompleted {
		t.Fatalf("after resume class = %s", rec2.Status)
	}
	if resume == nil || resume.Status != StatusPublished {
		t.Fatalf("resume = %+v", resume)
	}
	one, _ = os.ReadFile(filepath.Join(src, "one.txt"))
	two, _ = os.ReadFile(filepath.Join(src, "two.txt"))
	if string(one) != "1-agent\n" || string(two) != "2-agent\n" {
		t.Fatalf("after resume one=%q two=%q", one, two)
	}
}

func TestInterruptedPublicationExternallyDiverged(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"one.txt": "1\n", "two.txt": "2\n"})
	b, snap, runDir, work := captureRun(t, src)
	writeAll(t, runDir, map[string]string{"one.txt": "1-agent\n", "two.txt": "2-agent\n"})

	res, err := Integrate(ctx, Request{
		RunID:        "run-div",
		Source:       b,
		Snapshot:     snap,
		RunWorkspace: runDir,
		WorkDir:      work,
		JournalDir:   filepath.Join(work, "j"),
		CrashAfter:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusIncomplete {
		t.Fatalf("status = %s", res.Status)
	}
	pending := ""
	for _, e := range res.Journal.Entries {
		if e.Status != EntryVerified {
			pending = e.Path
			break
		}
	}
	if pending == "" {
		t.Fatal("no pending entry")
	}
	if err := os.WriteFile(filepath.Join(src, pending), []byte("external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := ClassifyJournal(ctx, src, res.Journal)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != RecoveryDiverged {
		t.Fatalf("class = %s files=%+v", rec.Status, rec.Files)
	}
}

func TestFilesystemBackendIntegrate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	writeAll(t, src, map[string]string{"a.txt": "A\n", "b.txt": "B\n"})
	b, err := workspace.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	if b.Kind() != workspace.KindFilesystem {
		t.Fatalf("kind = %s", b.Kind())
	}
	snap, err := b.CaptureSnapshot(ctx, workspace.CaptureOptions{SnapshotDir: filepath.Join(root, "snap")})
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(root, "run")
	if err := b.Materialize(ctx, snap, runDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "a.txt"), []byte("A-agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "b.txt"), []byte("B-user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Integrate(ctx, Request{
		RunID:        "run-fs",
		Source:       b,
		Snapshot:     snap,
		RunWorkspace: runDir,
		WorkDir:      filepath.Join(root, "work"),
		JournalDir:   filepath.Join(root, "work", "j"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusPublished {
		t.Fatalf("status=%s reason=%s conflicts=%+v", res.Status, res.Reason, res.Conflicts)
	}
	a, _ := os.ReadFile(filepath.Join(src, "a.txt"))
	bb, _ := os.ReadFile(filepath.Join(src, "b.txt"))
	if string(a) != "A-agent\n" || string(bb) != "B-user\n" {
		t.Fatalf("a=%q b=%q", a, bb)
	}
}

func TestIdentitySerializesBySourcePathNotProjectID(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"a.txt": "A\n"})
	b, err := workspace.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	id1 := b.Identity()
	id2 := b.Identity()
	if id1.Key() != id2.Key() {
		t.Fatalf("identity unstable: %s %s", id1.Key(), id2.Key())
	}
	if !strings.Contains(id1.Key(), src) && !strings.Contains(id1.Path, src) {
		t.Fatalf("identity should include source path: %+v", id1)
	}
}
