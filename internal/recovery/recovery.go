package recovery

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/integration"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
)

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
