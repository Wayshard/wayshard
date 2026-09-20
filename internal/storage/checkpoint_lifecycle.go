package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
)

// DefaultCheckpointRetain is how long checkpoint material for a terminal run is
// kept before it becomes reclaimable. Non-terminal runs are always pinned.
const DefaultCheckpointRetain = 7 * 24 * time.Hour

// ReclaimCheckpoints removes checkpoint material belonging to terminal runs
// older than retain and marks the metadata rows reclaimed. It never touches a
// checkpoint whose run is non-terminal, so a running, interrupted, blocked or
// failed run keeps its recoverable checkpoint.
func (s *Store) ReclaimCheckpoints(ctx context.Context, retain time.Duration) (int, error) {
	if retain <= 0 {
		retain = DefaultCheckpointRetain
	}
	cutoff := time.Now().Add(-retain)
	cps, err := s.ReclaimableCheckpoints(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, cp := range cps {
		if cp.CreatedAt.After(cutoff) {
			continue
		}
		if !underRunCheckpoints(s.Root, cp.RunID, cp.TreePath) {
			continue
		}
		if err := os.RemoveAll(cp.TreePath); err != nil {
			continue
		}
		if err := s.markCheckpointReclaimed(ctx, &cp); err != nil {
			continue
		}
		removed++
	}
	return removed, nil
}

func (s *Store) markCheckpointReclaimed(ctx context.Context, cp *domain.WorkspaceCheckpoint) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE workspace_checkpoints SET material_state = ? WHERE id = ? AND material_state = ?`,
			domain.CheckpointReclaimed, cp.ID, domain.CheckpointPresent); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "checkpoint.reclaimed", "", "", cp.RunID, map[string]any{
			"checkpointId": cp.ID, "stageId": cp.StageID, "attemptId": cp.AttemptID,
		})
		return err
	})
}

// CleanupCheckpointDebris removes recovery debris that is provably unreferenced:
// restore staging leftovers, checkpoint directories with no metadata row, and
// sandbox directories for terminal runs. It is intended for startup, before any
// process launch, so unreferenced directories cannot be in-flight checkpoints.
func (s *Store) CleanupCheckpointDebris(ctx context.Context) (int, error) {
	removed := 0
	_ = os.RemoveAll(filepath.Join(s.Root, "runtime", "restore-staging"))

	refs, err := s.ReferencedCheckpointPaths(ctx)
	if err != nil {
		return removed, err
	}
	root := filepath.Join(s.Root, "runtime", "workspaces")
	runDirs, err := os.ReadDir(root)
	if err != nil {
		return removed, nil
	}
	for _, rd := range runDirs {
		if !rd.IsDir() {
			continue
		}
		cpRoot := filepath.Join(root, rd.Name(), "checkpoints")
		stageDirs, err := os.ReadDir(cpRoot)
		if err != nil {
			continue
		}
		for _, sd := range stageDirs {
			if !sd.IsDir() {
				continue
			}
			stagePath := filepath.Join(cpRoot, sd.Name())
			entries, err := os.ReadDir(stagePath)
			if err != nil {
				continue
			}
			for _, e := range entries {
				p := filepath.Join(stagePath, e.Name())
				if _, ok := refs[filepath.Clean(p)]; ok {
					continue
				}
				if err := os.RemoveAll(p); err == nil {
					removed++
				}
			}
			if ents, _ := os.ReadDir(stagePath); len(ents) == 0 {
				_ = os.Remove(stagePath)
			}
		}
	}

	// Sandbox directories for terminal runs are never reused.
	sbRoot := filepath.Join(s.Root, "runtime", "sandbox")
	sbRuns, err := os.ReadDir(sbRoot)
	if err == nil {
		for _, rd := range sbRuns {
			if !rd.IsDir() {
				continue
			}
			r, err := s.GetRun(ctx, rd.Name())
			if err == nil && r.Status.Terminal() {
				if err := os.RemoveAll(filepath.Join(sbRoot, rd.Name())); err == nil {
					removed++
				}
			}
		}
	}
	return removed, nil
}

// underRunCheckpoints reports whether path is inside the runtime checkpoint
// root for runID. It is a deletion guard against a malformed stored path.
func underRunCheckpoints(root, runID, path string) bool {
	if path == "" {
		return false
	}
	base := filepath.Join(root, "runtime", "workspaces", runID, "checkpoints")
	base = filepath.Clean(base)
	p := filepath.Clean(path)
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		p = rp
	}
	if rb, err := filepath.EvalSymlinks(base); err == nil {
		base = rb
	}
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." || filepath.IsAbs(rel) {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
