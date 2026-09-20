package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// checkpointBeforeWrite captures a durable, restorable checkpoint of the
// authoritative run workspace. It must complete before the write attempt is
// allowed to start so recovery can always discard a partial write.
func (e *Engine) checkpointBeforeWrite(ctx context.Context, run *domain.Run, st *domain.Stage, att *domain.StageAttempt) error {
	ws, err := e.Store.GetWorkspaceByRun(ctx, run.ID)
	if err != nil {
		return err
	}
	b, err := workspace.Open(ws.RunPath)
	if err != nil {
		return err
	}
	cpDir := filepath.Join(filepath.Dir(ws.RunPath), "checkpoints", st.ID, strconv.Itoa(att.Ordinal))
	_ = os.RemoveAll(cpDir)
	snap, err := b.CaptureSnapshot(ctx, workspace.CaptureOptions{SnapshotDir: cpDir, ID: id.New()})
	if err != nil {
		return err
	}
	canon, err := workspace.CanonicalTreeHash(snap.TreePath)
	if err != nil {
		_ = os.RemoveAll(cpDir)
		return err
	}
	cp := &domain.WorkspaceCheckpoint{
		WorkspaceID: ws.ID,
		RunID:       run.ID,
		StageID:     st.ID,
		AttemptID:   att.ID,
		Name:        "pre-attempt",
		TreeHash:    canon,
		HashVersion: 2,
		TreePath:    cpDir,
	}
	if err := e.Store.InsertCheckpoint(ctx, cp); err != nil {
		return err
	}
	_ = e.Store.SetAttemptCheckpoint(ctx, att.ID, cp.ID)
	att.CheckpointID = cp.ID
	return nil
}
