package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrCheckpointHashMismatch reports that a checkpoint tree did not match the
// digest persisted when the checkpoint was created. Recovery must fail closed.
var ErrCheckpointHashMismatch = errors.New("checkpoint tree hash mismatch")

// RestoreOptions controls a verified restore.
//
// The restore never trusts the checkpoint directory directly. It first copies
// the checkpoint into a private staging tree under StagingRoot, hashes exactly
// what it staged, compares that digest with the persisted ExpectedHash, and
// only then materializes the RunWorkspace from the verified staged tree. A
// mutation of the original checkpoint after verification therefore cannot
// affect the restored result.
type RestoreOptions struct {
	// ExpectedHash is the canonical v3 tree digest persisted with the
	// checkpoint. It is required.
	ExpectedHash string
	// StagingRoot is trusted Wayshard runtime state, not visible or writable to
	// a harness, validation, or project process.
	StagingRoot string
	// AfterEntry, when set, is invoked after each checkpoint entry has been
	// copied into staging but before verification. It is a deterministic test
	// seam used to prove that a source mutation during staging is detected.
	AfterEntry func(stagedTree string) error
	// AfterVerified, when set, is invoked after the staged digest matches the
	// persisted hash and before the RunWorkspace swap. Returning an error
	// aborts the restore before the authoritative workspace is touched.
	AfterVerified func(stagedTree string) error
}

// RestoreVerified replaces dest with the state captured in snap using the
// verify-then-use flow described by RestoreOptions.
func RestoreVerified(ctx context.Context, b WorkspaceBackend, snap *Snapshot, dest string, opts RestoreOptions) error {
	if snap == nil || snap.TreePath == "" {
		return fmt.Errorf("checkpoint snapshot is required")
	}
	if opts.ExpectedHash == "" {
		return fmt.Errorf("expected checkpoint hash is required")
	}
	if opts.StagingRoot == "" {
		return fmt.Errorf("restore staging root is required")
	}
	if err := os.MkdirAll(opts.StagingRoot, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(opts.StagingRoot, "restore-stage-")
	if err != nil {
		return err
	}
	// The staged tree is private recovery state and must never leak.
	defer os.RemoveAll(stage)

	checkpointDir := filepath.Dir(snap.TreePath)
	if err := copyDirPreserve(checkpointDir, stage, opts.AfterEntry); err != nil {
		return fmt.Errorf("stage checkpoint: %w", err)
	}
	stagedTree := filepath.Join(stage, "tree")
	stagedHash, err := CanonicalTreeHashV3(stagedTree)
	if err != nil {
		return fmt.Errorf("hash staged checkpoint: %w", err)
	}
	if stagedHash != opts.ExpectedHash {
		return fmt.Errorf("%w: staged=%s expected=%s", ErrCheckpointHashMismatch, stagedHash, opts.ExpectedHash)
	}
	if opts.AfterVerified != nil {
		if err := opts.AfterVerified(stagedTree); err != nil {
			return err
		}
	}

	stagedSnap, err := LoadSnapshot(stage)
	if err != nil {
		return fmt.Errorf("load staged snapshot: %w", err)
	}
	// Consume exactly the verified staged bytes, never the original checkpoint.
	stagedSnap.TreePath = stagedTree
	if snap.StagedPath != "" {
		stagedSnap.StagedPath = filepath.Join(stage, "staged")
	}
	return RestoreSnapshot(ctx, b, stagedSnap, dest)
}

// copyDirPreserve copies a directory tree preserving directory membership,
// regular-file modes and symlink identity. afterEntry, when non-nil, runs after
// every copied entry with the staged "tree" directory path.
func copyDirPreserve(src, dst string, afterEntry func(stagedTree string) error) error {
	stagedTree := filepath.Join(dst, "tree")
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := dst
		if rel != "." {
			target = filepath.Join(dst, rel)
		}
		if d.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			return nil
		}
		if err := copyPath(path, target); err != nil {
			return err
		}
		if afterEntry != nil {
			if err := afterEntry(stagedTree); err != nil {
				return err
			}
		}
		return nil
	})
}
