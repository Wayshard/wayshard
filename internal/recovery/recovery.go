package recovery

import (
	"context"
	"log/slog"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/storage"
)

// Reconcile runs at startup before the scheduler accepts new work.
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
		switch r.Status {
		case domain.RunExecuting, domain.RunRepairing, domain.RunValidating, domain.RunPlanning, domain.RunReviewing, domain.RunExploring, domain.RunReplanning, domain.RunAssessing, domain.RunIntegrating:
			stages, _ := st.ListStages(ctx, r.ID)
			for _, stg := range stages {
				atts, _ := st.ListAttempts(ctx, stg.ID)
				for _, a := range atts {
					if a.Status == domain.AttemptRunning || a.Status == domain.AttemptPending {
						_ = st.UpdateAttemptStatus(ctx, a.ID, domain.AttemptInterrupted, domain.FailInfrastructure, "server restart")
					}
				}
			}
			log.Info("reconciled interrupted run", "run", r.ID, "status", r.Status)
		}
	}
	return nil
}
