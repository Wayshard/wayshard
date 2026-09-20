package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// checkpointFixture captures a filesystem snapshot into a checkpoint dir and
// returns the snapshot plus its persisted v3 digest.
func checkpointFixture(t *testing.T, files map[string]string) (b WorkspaceBackend, cpDir string, snap *Snapshot, digest string) {
	t.Helper()
	ctx := context.Background()
	src := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b = NewFilesystemBackend(src)
	cpDir = filepath.Join(t.TempDir(), "cp")
	snap, err := b.CaptureSnapshot(ctx, CaptureOptions{SnapshotDir: cpDir, ID: "cp"})
	if err != nil {
		t.Fatal(err)
	}
	digest, err = CanonicalTreeHashV3(snap.TreePath)
	if err != nil {
		t.Fatal(err)
	}
	return b, cpDir, snap, digest
}

func stagedDirs(t *testing.T, root string) []string {
	t.Helper()
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// TestRestoreVerifiedRejectsSourceMutationDuringStaging mutates the checkpoint
// source after some entries are copied but before verification. The staged tree
// is a hybrid, so the digest cannot match and recovery must fail closed rather
// than restore a mixture.
func TestRestoreVerifiedRejectsSourceMutationDuringStaging(t *testing.T) {
	ctx := context.Background()
	b, cpDir, snap, digest := checkpointFixture(t, map[string]string{"aa.txt": "one", "zz.txt": "two"})

	dest := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "keep.txt"), []byte("crashed-partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagingRoot := t.TempDir()

	mutated := false
	err := RestoreVerified(ctx, b, snap, dest, RestoreOptions{
		ExpectedHash: digest,
		StagingRoot:  stagingRoot,
		AfterEntry: func(string) error {
			if mutated {
				return nil
			}
			mutated = true
			// Mutate a checkpoint file that has not been staged yet.
			return os.WriteFile(filepath.Join(cpDir, "tree", "zz.txt"), []byte("tampered"), 0o644)
		},
	})
	if !errors.Is(err, ErrCheckpointHashMismatch) {
		t.Fatalf("expected hash mismatch, got %v", err)
	}
	if !mutated {
		t.Fatal("test seam never fired")
	}
	// The authoritative workspace must not have been touched by a failed verify.
	if got, _ := os.ReadFile(filepath.Join(dest, "keep.txt")); string(got) != "crashed-partial" {
		t.Fatalf("dest changed on failed verify: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "aa.txt")); err == nil {
		t.Fatal("hybrid tree was partially restored")
	}
	if dirs := stagedDirs(t, stagingRoot); len(dirs) != 0 {
		t.Fatalf("staging leaked: %v", dirs)
	}
}

// TestRestoreVerifiedUsesStagedTreeAfterVerification mutates the ORIGINAL
// checkpoint after the staged tree has been verified but before the swap. The
// restored workspace must reflect the already-verified staged bytes, proving
// the verify/use race is closed.
func TestRestoreVerifiedUsesStagedTreeAfterVerification(t *testing.T) {
	ctx := context.Background()
	b, cpDir, snap, digest := checkpointFixture(t, map[string]string{"aa.txt": "one", "zz.txt": "two"})

	dest := filepath.Join(t.TempDir(), "run")
	stagingRoot := t.TempDir()

	err := RestoreVerified(ctx, b, snap, dest, RestoreOptions{
		ExpectedHash: digest,
		StagingRoot:  stagingRoot,
		AfterVerified: func(string) error {
			return os.WriteFile(filepath.Join(cpDir, "tree", "zz.txt"), []byte("tampered-after-verify"), 0o644)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "zz.txt")); string(got) != "two" {
		t.Fatalf("restored result changed after verification: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "aa.txt")); string(got) != "one" {
		t.Fatalf("aa.txt wrong: %q", got)
	}
	if dirs := stagedDirs(t, stagingRoot); len(dirs) != 0 {
		t.Fatalf("staging leaked: %v", dirs)
	}
}

// TestRestoreVerifiedAbortsBeforeSwap proves an injected failure after
// verification but before the swap leaves the authoritative workspace and its
// partial state untouched and cleans staging.
func TestRestoreVerifiedAbortsBeforeSwap(t *testing.T) {
	ctx := context.Background()
	b, _, snap, digest := checkpointFixture(t, map[string]string{"aa.txt": "one"})

	dest := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "partial.txt"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagingRoot := t.TempDir()

	sentinel := errors.New("injected failure before swap")
	err := RestoreVerified(ctx, b, snap, dest, RestoreOptions{
		ExpectedHash:  digest,
		StagingRoot:   stagingRoot,
		AfterVerified: func(string) error { return sentinel },
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected injected failure, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "partial.txt")); err != nil {
		t.Fatalf("partial workspace was disturbed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "aa.txt")); err == nil {
		t.Fatal("swap happened despite injected failure")
	}
	if dirs := stagedDirs(t, stagingRoot); len(dirs) != 0 {
		t.Fatalf("staging leaked: %v", dirs)
	}
}
