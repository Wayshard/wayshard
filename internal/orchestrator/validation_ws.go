package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// validationWorkspace materializes a disposable, isolated copy of the state to
// be validated. Validation commands may freely mutate it; it is discarded
// afterwards and never feeds RunDelta or integration.
//
// The copy is produced by the same workspace materialization used for run
// workspaces (isolated git clone with --no-hardlinks, or a byte copy for
// non-git trees), so it never shares mutable inodes or git metadata with the
// authoritative run/source workspace.
type validationWorkspace struct {
	Dir     string
	rootDir string
}

func (v *validationWorkspace) Cleanup() {
	if v == nil || v.rootDir == "" {
		return
	}
	_ = os.RemoveAll(v.rootDir)
}

// baselineValidationWorkspace materializes the exact task-start state from the
// immutable snapshot, independent of any later run-workspace mutation.
func (e *Engine) baselineValidationWorkspace(ctx context.Context, runID string) (*validationWorkspace, error) {
	ws, err := e.Store.GetWorkspaceByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	run, err := e.Store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	proj, err := e.Store.GetProject(ctx, run.ProjectID)
	if err != nil {
		return nil, err
	}
	runDir := filepath.Dir(ws.RunPath)
	snap, err := workspace.LoadSnapshot(filepath.Join(runDir, "snapshot"))
	if err != nil {
		return nil, fmt.Errorf("baseline snapshot: %w", err)
	}
	backend, err := workspace.Open(proj.Path)
	if err != nil {
		return nil, err
	}
	rootDir := filepath.Join(runDir, "validation", "baseline")
	_ = os.RemoveAll(rootDir)
	dir := filepath.Join(rootDir, "ws")
	if err := backend.Materialize(ctx, snap, dir); err != nil {
		_ = os.RemoveAll(rootDir)
		return nil, fmt.Errorf("baseline validation workspace: %w", err)
	}
	return &validationWorkspace{Dir: dir, rootDir: rootDir}, nil
}

// finalValidationWorkspace materializes the authoritative candidate run
// workspace into a disposable copy.
func (e *Engine) finalValidationWorkspace(ctx context.Context, runID string) (*validationWorkspace, error) {
	ws, err := e.Store.GetWorkspaceByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	runDir := filepath.Dir(ws.RunPath)
	backend, err := workspace.Open(ws.RunPath)
	if err != nil {
		return nil, err
	}
	rootDir := filepath.Join(runDir, "validation", "final")
	_ = os.RemoveAll(rootDir)
	snap, err := backend.CaptureSnapshot(ctx, workspace.CaptureOptions{
		SnapshotDir: filepath.Join(rootDir, "snap"),
		ID:          id.New(),
	})
	if err != nil {
		_ = os.RemoveAll(rootDir)
		return nil, fmt.Errorf("final validation snapshot: %w", err)
	}
	dir := filepath.Join(rootDir, "ws")
	if err := backend.Materialize(ctx, snap, dir); err != nil {
		_ = os.RemoveAll(rootDir)
		return nil, fmt.Errorf("final validation workspace: %w", err)
	}
	return &validationWorkspace{Dir: dir, rootDir: rootDir}, nil
}
