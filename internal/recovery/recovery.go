package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/integration"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// validateCheckpointPath ensures a persisted checkpoint tree path stays inside
// the Wayshard runtime workspace root and belongs to the expected run, so a
// stored path cannot redirect restoration outside trusted state.
func validateCheckpointPath(st *storage.Store, runID string, cp *domain.WorkspaceCheckpoint) error {
	root := filepath.Join(st.Root, "runtime", "workspaces")
	clean := filepath.Clean(cp.TreePath)
	resolved := clean
	if rp, err := filepath.EvalSymlinks(clean); err == nil {
		resolved = rp
	}
	rootResolved := root
	if rp, err := filepath.EvalSymlinks(root); err == nil {
		rootResolved = rp
	}
	if resolved != rootResolved && !strings.HasPrefix(resolved, rootResolved+string(os.PathSeparator)) {
		return fmt.Errorf("checkpoint path escapes runtime root")
	}
	if !strings.Contains(clean, runID) {
		return fmt.Errorf("checkpoint path does not belong to run %s", runID)
	}
	return nil
}

// restoreWriteCheckpoint restores the authoritative run workspace from the
// pre-attempt checkpoint of an interrupted write attempt, discarding partial
// writes. It never trusts the current partially-written workspace.
func restoreWriteCheckpoint(ctx context.Context, st *storage.Store, r domain.Run, stg domain.Stage, a domain.StageAttempt, log *slog.Logger) error {
	cp, err := st.LatestCheckpointForAttempt(ctx, a.ID)
	if err != nil {
		cp, err = st.LatestCheckpointForStage(ctx, stg.ID)
	}
	if err != nil || cp == nil {
		return fmt.Errorf("no checkpoint for interrupted write attempt")
	}
	snap, err := workspace.LoadSnapshot(cp.TreePath)
	if err != nil {
		return fmt.Errorf("load checkpoint: %w", err)
	}
	if cp.HashVersion != 3 {
		return fmt.Errorf("checkpoint hash version %d is not verifiable", cp.HashVersion)
	}
	if err := validateCheckpointPath(st, r.ID, cp); err != nil {
		return err
	}
	canon, err := workspace.CanonicalTreeHashV3(snap.TreePath)
	if err != nil {
		return fmt.Errorf("hash checkpoint tree: %w", err)
	}
	if canon != cp.TreeHash {
		return fmt.Errorf("checkpoint tree hash mismatch")
	}
	ws, err := st.GetWorkspaceByRun(ctx, r.ID)
	if err != nil {
		return err
	}
	b, err := workspace.Open(ws.RunPath)
	if err != nil {
		return err
	}
	if err := workspace.RestoreSnapshot(ctx, b, snap, ws.RunPath); err != nil {
		return err
	}
	if log != nil {
		log.Info("restored workspace from checkpoint", "run", r.ID, "stage", stg.ID, "checkpoint", cp.ID)
	}
	_ = st.EmitEvent(ctx, "checkpoint.restored", r.ID, map[string]any{"checkpointId": cp.ID, "stageId": stg.ID})
	return nil
}

// Reconcile runs at startup before the scheduler accepts new work. It marks
// interrupted attempts/stages and reconciles incomplete publication journals.
func Reconcile(ctx context.Context, st *storage.Store, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	if err := st.IntegrityCheck(ctx); err != nil {
		log.Error("storage integrity", "err", err)
		return err
	}
	runs, err := st.ListActiveRuns(ctx)
	if err != nil {
		return err
	}
	for _, r := range runs {
		stages, _ := st.ListStages(ctx, r.ID)
		for _, stg := range stages {
			atts, _ := st.ListAttempts(ctx, stg.ID)
			for _, a := range atts {
				if a.Status == domain.AttemptRunning || a.Status == domain.AttemptPending {
					_ = st.UpdateAttemptStatus(ctx, a.ID, domain.AttemptInterrupted, domain.FailInfrastructure, "server restart")
					if stg.Kind.WritesWorkspace() {
						if err := restoreWriteCheckpoint(ctx, st, r, stg, a, log); err != nil {
							log.Error("checkpoint restore failed", "run", r.ID, "stage", stg.ID, "err", err)
							_ = st.UpdateRunStatus(ctx, r.ID, domain.RunBlocked, domain.BlockedRecovery, "checkpoint restore failed: "+err.Error())
							_ = st.EmitEvent(ctx, "recovery.blocked", r.ID, map[string]any{"stageId": stg.ID, "attemptId": a.ID, "reason": err.Error()})
						}
					}
				}
			}
			if stg.Status == domain.AttemptRunning {
				_ = st.UpdateStageStatus(ctx, stg.ID, domain.AttemptInterrupted, domain.FailInfrastructure, "server restart")
			}
		}
		log.Info("reconciled interrupted run", "run", r.ID, "status", r.Status)
	}
	reconcileJournals(ctx, st, log)
	cleanupValidationWorkspaces(st, log)
	return nil
}

// cleanupValidationWorkspaces removes disposable validation copies left by an
// interrupted run. They never hold authoritative state.
func cleanupValidationWorkspaces(st *storage.Store, log *slog.Logger) {
	root := filepath.Join(st.Root, "runtime", "workspaces")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		vdir := filepath.Join(root, e.Name(), "validation")
		if _, err := os.Stat(vdir); err == nil {
			if err := os.RemoveAll(vdir); err != nil {
				log.Warn("cleanup validation workspace", "dir", vdir, "err", err)
			}
		}
	}
}

// reconcileJournals finds on-disk publication journals and resumes or
// classifies them so a crash during integration does not leave source in an
// ambiguous state.
func reconcileJournals(ctx context.Context, st *storage.Store, log *slog.Logger) {
	root := filepath.Join(st.Root, "runtime", "journals")
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "journal.json" {
			return nil
		}
		j, err := integration.LoadJournal(path)
		if err != nil {
			log.Warn("load journal", "path", path, "err", err)
			return nil
		}
		proj, err := st.GetProject(ctx, j.ProjectID)
		if err != nil {
			log.Warn("journal project missing", "journal", j.IntegrationID, "err", err)
			return nil
		}
		src, err := workspace.Open(proj.Path)
		if err != nil {
			log.Warn("journal source missing", "journal", j.IntegrationID, "err", err)
			return nil
		}
		rec, res, err := integration.RecoverPublication(ctx, src.SourcePath(), j)
		if err != nil {
			log.Warn("publication recovery", "journal", j.IntegrationID, "err", err)
			return nil
		}
		status := ""
		if rec != nil {
			status = rec.Status
		}
		if res != nil {
			status = res.Status
		}
		log.Info("publication recovery", "journal", j.IntegrationID, "status", status)
		return nil
	})
}
