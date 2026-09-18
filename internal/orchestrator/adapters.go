package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/integration"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
)

type WorkspaceAdapter struct {
	Store   *storage.Store
	DataDir string
}

func (w *WorkspaceAdapter) Prepare(ctx context.Context, project domain.Project, run domain.Run) (*domain.WorkspaceRecord, *domain.Snapshot, error) {
	if existing, err := w.Store.GetWorkspaceByRun(ctx, run.ID); err == nil {
		return existing, nil, nil
	}
	b, err := workspace.Open(project.Path)
	if err != nil {
		return nil, nil, err
	}
	destRoot := filepath.Join(w.DataDir, "runtime", "workspaces", run.ID)
	snap, err := b.CaptureSnapshot(ctx, workspace.CaptureOptions{
		SnapshotDir:  filepath.Join(destRoot, "snapshot"),
		KnowledgeRev: project.KnowledgeRev,
		ID:           id.New(),
	})
	if err != nil {
		return nil, nil, err
	}
	runPath := filepath.Join(destRoot, "run")
	if err := b.Materialize(ctx, snap, runPath); err != nil {
		return nil, nil, err
	}
	rec := &domain.WorkspaceRecord{
		RunID:      run.ID,
		ProjectID:  project.ID,
		Kind:       b.Kind(),
		SourcePath: project.Path,
		RunPath:    runPath,
		SnapshotID: snap.ID,
		Branch:     snap.Branch,
		HEAD:       snap.HEAD,
	}
	if err := w.Store.InsertWorkspace(ctx, rec); err != nil {
		return nil, nil, err
	}
	ds, _ := json.Marshal(snap.Dirty)
	_ = w.Store.InsertSnapshot(ctx, &domain.Snapshot{
		ID: snap.ID, WorkspaceID: rec.ID, Branch: snap.Branch, HEAD: snap.HEAD,
		DirtyJSON: string(ds), KnowledgeRev: snap.KnowledgeRev, ObjectHash: snap.TreeHash,
	})
	return rec, &domain.Snapshot{ID: snap.ID, Branch: snap.Branch, HEAD: snap.HEAD}, nil
}

type IntegrateAdapter struct {
	Store *storage.Store
}

func (a *IntegrateAdapter) Integrate(ctx context.Context, run domain.Run, ws *domain.WorkspaceRecord) (*domain.Integration, error) {
	src, err := workspace.Open(ws.SourcePath)
	if err != nil {
		return nil, err
	}
	snap, err := workspace.LoadSnapshot(filepath.Join(filepath.Dir(ws.RunPath), "snapshot"))
	if err != nil {
		// reconstruct minimal snapshot from stored record
		snap = &workspace.Snapshot{ID: ws.SnapshotID, SourcePath: ws.SourcePath, Branch: ws.Branch, HEAD: ws.HEAD, TreePath: filepath.Join(filepath.Dir(ws.RunPath), "snapshot", "tree")}
	}
	res, err := integration.Integrate(ctx, integration.Request{
		RunID:        run.ID,
		ProjectID:    run.ProjectID,
		Source:       src,
		Snapshot:     snap,
		RunWorkspace: ws.RunPath,
		WorkDir:      filepath.Join(os.TempDir(), "wayshard-integrate-"+run.ID),
		JournalDir:   filepath.Join(a.Store.Root, "runtime", "journals", run.ID),
	})
	in := &domain.Integration{RunID: run.ID, ProjectID: run.ProjectID, BaseSnapshot: ws.SnapshotID}
	if res != nil {
		in.Status = res.Status
		in.Error = res.Reason
		in.CurrentBranch = ws.Branch
		in.TargetBranch = ws.Branch
	}
	if err != nil && in.Status == "" {
		in.Status = "blocked"
		in.Error = err.Error()
	}
	_ = a.Store.InsertIntegration(ctx, in)
	return in, err
}
