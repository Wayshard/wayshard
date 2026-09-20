package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/gitutil"
	"github.com/Wayshard/wayshard/internal/integration"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// reconcileJournals finds durable publication journals left by an interrupted
// integration and reconciles them: classify against the real source using
// journal before/after hashes, resume only the safe remainder, finalize or
// block the run/integration durably, and emit high-level events. It never
// overwrites unexpected source state.
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
		if err := reconcilePublication(ctx, st, log, j); err != nil {
			log.Warn("publication reconciliation", "journal", j.IntegrationID, "err", err)
		}
		return nil
	})
}

func reconcilePublication(ctx context.Context, st *storage.Store, log *slog.Logger, j *integration.Journal) error {
	run, err := st.GetRun(ctx, j.RunID)
	if err != nil {
		return fmt.Errorf("journal run %s: %w", j.RunID, err)
	}
	proj, err := st.GetProject(ctx, run.ProjectID)
	if err != nil {
		return fmt.Errorf("journal project: %w", err)
	}

	// A relocated or replaced source must not receive an old journal.
	src := gitutil.CanonicalPath(proj.Path)
	if j.SourcePath != "" && gitutil.CanonicalPath(j.SourcePath) != src {
		blockPublication(ctx, st, log, j, run, "source path changed since publication prepared")
		return nil
	}
	b, err := workspace.Open(proj.Path)
	if err != nil {
		return err
	}
	if j.IdentityKey != "" && b.Identity().Key() != j.IdentityKey {
		blockPublication(ctx, st, log, j, run, "source identity changed since publication prepared")
		return nil
	}
	// Every target must remain a safe source-relative path.
	for _, e := range j.Entries {
		if _, err := workspace.SafeRel(e.Path); err != nil {
			blockPublication(ctx, st, log, j, run, "unsafe publication target: "+e.Path)
			return nil
		}
	}

	// A terminal run no longer needs its journal; clean it without republishing.
	if run.Status.Terminal() {
		cleanupJournal(j)
		return nil
	}

	rec, res, rerr := integration.RecoverPublication(ctx, b.SourcePath(), j)
	if rerr != nil {
		blockPublication(ctx, st, log, j, run, "publication could not be resumed: "+rerr.Error())
		return nil
	}
	published := (res != nil && res.Status == integration.StatusPublished) ||
		(res == nil && rec.Status == integration.RecoveryCompleted)
	if published {
		finalizePublication(ctx, st, log, j, run)
		return nil
	}
	reason := "publication diverged"
	if rec != nil {
		reason = "publication " + rec.Status
	}
	if res != nil {
		reason = "publication " + res.Status
	}
	blockPublication(ctx, st, log, j, run, reason)
	return nil
}

func finalizePublication(ctx context.Context, st *storage.Store, log *slog.Logger, j *integration.Journal, run *domain.Run) {
	in := &domain.Integration{
		ID: j.IntegrationID, RunID: j.RunID, ProjectID: j.ProjectID,
		Status: integration.StatusPublished, TargetBranch: j.TargetBranch, CurrentBranch: j.CurrentBranch,
	}
	upsertIntegration(ctx, st, j.IntegrationID, in)
	_ = st.UpdateRunStatus(ctx, run.ID, domain.RunComplete, "", "")
	_ = st.EmitEvent(ctx, "publication.reconciled", run.ID, map[string]any{
		"integrationId": j.IntegrationID, "status": integration.StatusPublished,
	})
	log.Info("publication reconciled", "run", run.ID, "integration", j.IntegrationID, "status", integration.StatusPublished)
	cleanupJournal(j)
}

func blockPublication(ctx context.Context, st *storage.Store, log *slog.Logger, j *integration.Journal, run *domain.Run, reason string) {
	in := &domain.Integration{
		ID: j.IntegrationID, RunID: j.RunID, ProjectID: j.ProjectID,
		Status: integration.StatusBlocked, TargetBranch: j.TargetBranch, CurrentBranch: j.CurrentBranch, Error: reason,
	}
	upsertIntegration(ctx, st, j.IntegrationID, in)
	_ = st.UpdateRunStatus(ctx, run.ID, domain.RunIntegrationBlocked, domain.BlockedIntegration, reason)
	_ = st.EmitEvent(ctx, "publication.reconciled", run.ID, map[string]any{
		"integrationId": j.IntegrationID, "status": integration.StatusBlocked, "reason": reason,
	})
	log.Warn("publication blocked on recovery", "run", run.ID, "integration", j.IntegrationID, "reason", reason)
	// Keep the journal: a later startup may still need its hashes, and the
	// unexpected source state must never be overwritten.
}

func upsertIntegration(ctx context.Context, st *storage.Store, id string, in *domain.Integration) {
	if existing, err := st.GetIntegrationByID(ctx, id); err == nil && existing != nil {
		in.BaseSnapshot = existing.BaseSnapshot
		_ = st.UpdateIntegration(ctx, in)
		return
	}
	_ = st.InsertIntegration(ctx, in)
}

func cleanupJournal(j *integration.Journal) {
	if j == nil || j.Dir == "" {
		return
	}
	_ = os.RemoveAll(j.Dir)
}
