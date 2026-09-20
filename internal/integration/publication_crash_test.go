package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func mustNotExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("%s should not exist", p)
	}
}

// TestPublicationPreparedOnlyRecovery interrupts before the first source write.
func TestPublicationPreparedOnlyRecovery(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"one.txt": "1\n", "two.txt": "2\n"})
	b, snap, runDir, work := captureRun(t, src)
	writeAll(t, runDir, map[string]string{"one.txt": "1-agent\n", "two.txt": "2-agent\n"})

	sentinel := errors.New("stop before first write")
	res, err := Integrate(ctx, Request{
		RunID: "run-prepared", Source: b, Snapshot: snap, RunWorkspace: runDir,
		WorkDir: work, JournalDir: filepath.Join(work, "j"),
		PublishStep: func(int) error { return sentinel },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusIncomplete {
		t.Fatalf("status=%s reason=%s", res.Status, res.Reason)
	}
	if readFile(t, filepath.Join(src, "one.txt")) != "1\n" || readFile(t, filepath.Join(src, "two.txt")) != "2\n" {
		t.Fatal("source mutated before first write")
	}
	rec, resume, err := RecoverPublication(ctx, src, res.Journal)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != RecoveryCompleted || resume == nil || resume.Status != StatusPublished {
		t.Fatalf("recovery=%+v resume=%+v", rec, resume)
	}
	if readFile(t, filepath.Join(src, "one.txt")) != "1-agent\n" || readFile(t, filepath.Join(src, "two.txt")) != "2-agent\n" {
		t.Fatal("recovery did not complete publication")
	}
}

// TestPublicationPartialAddModifyDeleteRecovery covers a multi-file delta with
// add, modify and delete interrupted after the first write.
func TestPublicationPartialAddModifyDeleteRecovery(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"mod.txt": "old\n", "del.txt": "gone\n"})
	b, snap, runDir, work := captureRun(t, src)
	if err := os.WriteFile(filepath.Join(runDir, "mod.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "added.txt"), []byte("added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, "del.txt")); err != nil {
		t.Fatal(err)
	}

	res, err := Integrate(ctx, Request{
		RunID: "run-amd", Source: b, Snapshot: snap, RunWorkspace: runDir,
		WorkDir: work, JournalDir: filepath.Join(work, "j"), CrashAfter: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusIncomplete {
		t.Fatalf("status=%s", res.Status)
	}
	// Sorted: added.txt (create) publishes first.
	if readFile(t, filepath.Join(src, "added.txt")) != "added\n" {
		t.Fatal("first entry was not published")
	}
	if readFile(t, filepath.Join(src, "mod.txt")) != "old\n" {
		t.Fatal("mod.txt published too early")
	}
	if readFile(t, filepath.Join(src, "del.txt")) != "gone\n" {
		t.Fatal("del.txt deleted too early")
	}
	rec, resume, err := RecoverPublication(ctx, src, res.Journal)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != RecoveryCompleted || resume == nil || resume.Status != StatusPublished {
		t.Fatalf("recovery=%+v resume=%+v", rec, resume)
	}
	if readFile(t, filepath.Join(src, "added.txt")) != "added\n" {
		t.Fatal("added.txt wrong after recovery")
	}
	if readFile(t, filepath.Join(src, "mod.txt")) != "new\n" {
		t.Fatal("mod.txt wrong after recovery")
	}
	mustNotExist(t, filepath.Join(src, "del.txt"))
}

// TestPublicationUserEditUnprocessedTargetBlocks preserves an unexpected user
// edit and never completes the operation over it.
func TestPublicationUserEditUnprocessedTargetBlocks(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"mod.txt": "old\n", "del.txt": "gone\n"})
	b, snap, runDir, work := captureRun(t, src)
	writeAll(t, runDir, map[string]string{"mod.txt": "new\n", "added.txt": "added\n"})
	if err := os.Remove(filepath.Join(runDir, "del.txt")); err != nil {
		t.Fatal(err)
	}
	res, err := Integrate(ctx, Request{
		RunID: "run-user1", Source: b, Snapshot: snap, RunWorkspace: runDir,
		WorkDir: work, JournalDir: filepath.Join(work, "j"), CrashAfter: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// User edits an unprocessed target (the pending delete).
	if err := os.WriteFile(filepath.Join(src, "del.txt"), []byte("user-edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, _, err := RecoverPublication(ctx, src, res.Journal)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != RecoveryDiverged {
		t.Fatalf("expected diverged, got %s", rec.Status)
	}
	if readFile(t, filepath.Join(src, "del.txt")) != "user-edit\n" {
		t.Fatal("user edit was overwritten")
	}
}

// TestPublicationUserEditPublishedTargetBlocks preserves an edit to an
// already-published target.
func TestPublicationUserEditPublishedTargetBlocks(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"mod.txt": "old\n", "other.txt": "x\n"})
	b, snap, runDir, work := captureRun(t, src)
	writeAll(t, runDir, map[string]string{"added.txt": "added\n", "mod.txt": "new\n"})
	res, err := Integrate(ctx, Request{
		RunID: "run-user2", Source: b, Snapshot: snap, RunWorkspace: runDir,
		WorkDir: work, JournalDir: filepath.Join(work, "j"), CrashAfter: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(src, "added.txt")) != "added\n" {
		t.Fatal("published target missing")
	}
	if err := os.WriteFile(filepath.Join(src, "added.txt"), []byte("user-edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, _, err := RecoverPublication(ctx, src, res.Journal)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != RecoveryDiverged {
		t.Fatalf("expected diverged, got %s", rec.Status)
	}
	if readFile(t, filepath.Join(src, "added.txt")) != "user-edit\n" {
		t.Fatal("published target was repaired over user data")
	}
}

// TestPublicationSymlinkParentEscapeBlocked proves a symlinked parent that
// appears after preparation cannot redirect a write outside the source tree.
func TestPublicationSymlinkParentEscapeBlocked(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"keep.txt": "keep\n"})
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, snap, runDir, work := captureRun(t, src)
	writeAll(t, runDir, map[string]string{"sub/evil.txt": "escape\n"})

	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	swapped := false
	res, err := Integrate(ctx, Request{
		RunID: "run-escape", Source: b, Snapshot: snap, RunWorkspace: runDir,
		WorkDir: work, JournalDir: filepath.Join(work, "j"),
		PublishStep: func(int) error {
			if swapped {
				return nil
			}
			swapped = true
			_ = os.RemoveAll(filepath.Join(src, "sub"))
			return os.Symlink(outside, filepath.Join(src, "sub"))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status == StatusPublished {
		t.Fatal("publication escaped through symlinked parent")
	}
	mustNotExist(t, filepath.Join(outside, "evil.txt"))
}

// TestPublicationReconcileIdempotent proves repeated reconciliation is stable.
func TestPublicationReconcileIdempotent(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"one.txt": "1\n", "two.txt": "2\n"})
	b, snap, runDir, work := captureRun(t, src)
	writeAll(t, runDir, map[string]string{"one.txt": "1-agent\n", "two.txt": "2-agent\n"})
	res, err := Integrate(ctx, Request{
		RunID: "run-idem", Source: b, Snapshot: snap, RunWorkspace: runDir,
		WorkDir: work, JournalDir: filepath.Join(work, "j"), CrashAfter: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		rec, _, err := RecoverPublication(ctx, src, res.Journal)
		if err != nil {
			t.Fatal(err)
		}
		if rec.Status != RecoveryCompleted {
			t.Fatalf("pass %d class = %s", i, rec.Status)
		}
	}
	if readFile(t, filepath.Join(src, "one.txt")) != "1-agent\n" || readFile(t, filepath.Join(src, "two.txt")) != "2-agent\n" {
		t.Fatal("idempotent recovery changed source")
	}
}
