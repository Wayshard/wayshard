package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/process"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// validateCheckpointPath proves a persisted checkpoint tree belongs to the
// expected run and stage. Ownership is established from path components after
// resolving symlinks, never from a substring match, so a symlink under run A's
// checkpoint location cannot redirect restoration to run B.
func validateCheckpointPath(st *storage.Store, r domain.Run, stg domain.Stage, cp *domain.WorkspaceCheckpoint) error {
	expectedBase := filepath.Join(st.Root, "runtime", "workspaces", r.ID, "checkpoints", stg.ID)
	resolved := filepath.Clean(cp.TreePath)
	if rp, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = rp
	}
	baseResolved := filepath.Clean(expectedBase)
	if rp, err := filepath.EvalSymlinks(baseResolved); err == nil {
		baseResolved = rp
	}
	rel, err := filepath.Rel(baseResolved, resolved)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("checkpoint path does not belong to run %s stage %s", r.ID, stg.ID)
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("checkpoint path does not belong to run %s stage %s", r.ID, stg.ID)
	}
	return nil
}

// restoreOptionsForTest is nil in production. Tests may set it to inject
// deterministic seams (source mutation during staging, abort before swap) into
// the verified restore without adding a production crash switch.
var restoreOptionsForTest func(*workspace.RestoreOptions)

// checkpointForAttempt resolves the checkpoint explicitly associated with one
// interrupted attempt. It never falls back to "latest checkpoint for the
// stage/run", so a legacy attempt with no checkpoint fails closed instead of
// silently restoring a different attempt's tree.
func checkpointForAttempt(ctx context.Context, st *storage.Store, a domain.StageAttempt) (*domain.WorkspaceCheckpoint, error) {
	if a.CheckpointID != "" {
		cp, err := st.GetCheckpoint(ctx, a.CheckpointID)
		if err != nil || cp == nil {
			return nil, fmt.Errorf("checkpoint %s referenced by attempt %s is unavailable", a.CheckpointID, a.ID)
		}
		return cp, nil
	}
	cp, err := st.LatestCheckpointForAttempt(ctx, a.ID)
	if err != nil || cp == nil {
		return nil, fmt.Errorf("no checkpoint for interrupted write attempt %s", a.ID)
	}
	return cp, nil
}

// restoreWriteCheckpoint restores the authoritative run workspace from the
// pre-attempt checkpoint of an interrupted write attempt, discarding partial
// writes. It never trusts the current partially-written workspace, and it
// restores only from the exact bytes it cryptographically verified.
func restoreWriteCheckpoint(ctx context.Context, st *storage.Store, r domain.Run, stg domain.Stage, a domain.StageAttempt, log *slog.Logger) error {
	cp, err := checkpointForAttempt(ctx, st, a)
	if err != nil {
		return err
	}
	if cp.MaterialState == domain.CheckpointReclaimed {
		return fmt.Errorf("checkpoint %s material has been reclaimed", cp.ID)
	}
	if cp.HashVersion != 3 {
		return fmt.Errorf("checkpoint hash version %d is not verifiable", cp.HashVersion)
	}
	if err := validateCheckpointPath(st, r, stg, cp); err != nil {
		return err
	}
	snap, err := workspace.LoadSnapshot(cp.TreePath)
	if err != nil {
		return fmt.Errorf("load checkpoint: %w", err)
	}
	ws, err := st.GetWorkspaceByRun(ctx, r.ID)
	if err != nil {
		return err
	}
	b, err := workspace.Open(ws.RunPath)
	if err != nil {
		return err
	}
	opts := workspace.RestoreOptions{
		ExpectedHash: cp.TreeHash,
		StagingRoot:  filepath.Join(st.Root, "runtime", "restore-staging"),
	}
	if restoreOptionsForTest != nil {
		restoreOptionsForTest(&opts)
	}
	if err := workspace.RestoreVerified(ctx, b, snap, ws.RunPath, opts); err != nil {
		return err
	}
	if log != nil {
		log.Info("restored workspace from checkpoint", "run", r.ID, "stage", stg.ID, "checkpoint", cp.ID)
	}
	_ = st.EmitEvent(ctx, "checkpoint.restored", r.ID, map[string]any{"checkpointId": cp.ID, "stageId": stg.ID, "attemptId": a.ID})
	return nil
}

// reconcileProcesses terminates stale process trees owned by previous server
// instances before any workspace is restored or scheduled. It returns the set
// of runs whose stale processes could not be reconciled; those runs are blocked
// and must not be restored against.
func reconcileProcesses(ctx context.Context, st *storage.Store, log *slog.Logger) (map[string]bool, error) {
	unsafe := map[string]bool{}
	owners, err := st.ListActiveProcessOwners(ctx)
	if err != nil {
		// Ownership is unknown, so a stale writer cannot be ruled out. Fail
		// closed rather than restore or schedule against a possible writer.
		log.Error("list process owners", "err", err)
		return unsafe, err
	}
	for _, o := range owners {
		observed, remaining, supported, _ := process.ReconcileTokenHash(o.TokenHash, o.PGID, 3*time.Second)
		run, runErr := st.GetRun(ctx, o.RunID)
		activeRun := runErr == nil && !run.Status.Terminal()

		if !supported {
			// Ownership cannot be verified on this platform: fail closed for a
			// live run rather than restore against a possible stale writer.
			if activeRun {
				blockForStaleProcesses(ctx, st, log, o, "process ownership cannot be verified on this platform")
				unsafe[o.RunID] = true
				continue
			}
			_ = st.MarkProcessOwnerReconciled(ctx, o.ID)
			continue
		}
		if remaining == 0 {
			_ = st.MarkProcessOwnerReconciled(ctx, o.ID)
			if observed > 0 {
				_ = st.EmitEvent(ctx, "execution.orphan_reconciled", o.RunID, map[string]any{
					"stageId": o.StageID, "attemptId": o.AttemptID, "terminated": observed,
				})
				log.Info("reconciled stale process tree", "run", o.RunID, "attempt", o.AttemptID, "terminated", observed)
			}
			continue
		}
		if activeRun {
			blockForStaleProcesses(ctx, st, log, o, "stale process tree could not be reconciled")
			unsafe[o.RunID] = true
		}
	}
	return unsafe, nil
}

// blockForStaleProcesses marks a run BLOCKED/RECOVERY and closes out its
// running attempts so nothing restores or schedules against a workspace that a
// stale process may still be writing.
func blockForStaleProcesses(ctx context.Context, st *storage.Store, log *slog.Logger, o domain.ProcessOwner, reason string) {
	log.Error("orphan reconciliation failed", "run", o.RunID, "attempt", o.AttemptID, "reason", reason)
	_ = st.UpdateRunStatus(ctx, o.RunID, domain.RunBlocked, domain.BlockedRecovery, reason)
	stages, _ := st.ListStages(ctx, o.RunID)
	for _, stg := range stages {
		atts, _ := st.ListAttempts(ctx, stg.ID)
		for _, a := range atts {
			if a.Status == domain.AttemptRunning || a.Status == domain.AttemptPending {
				_ = st.UpdateAttemptStatus(ctx, a.ID, domain.AttemptInterrupted, domain.FailInfrastructure, reason)
			}
		}
		if stg.Status == domain.AttemptRunning {
			_ = st.UpdateStageStatus(ctx, stg.ID, domain.AttemptInterrupted, domain.FailInfrastructure, reason)
		}
	}
	_ = st.EmitEvent(ctx, "recovery.blocked", o.RunID, map[string]any{"stageId": o.StageID, "attemptId": o.AttemptID, "reason": reason})
}

// reconcileTerminalRuns repairs durable state for terminal runs that still have
// a running or pending attempt, which can be left by a crash during
// cancellation or completion. A CANCELLED run's attempts become CANCELLED; other
// terminal runs' attempts become INTERRUPTED. Nothing is resumed.
func reconcileTerminalRuns(ctx context.Context, st *storage.Store, log *slog.Logger) {
	runs, err := st.TerminalRunsWithRunningAttempts(ctx)
	if err != nil {
		log.Error("list terminal runs with running attempts", "err", err)
		return
	}
	for _, r := range runs {
		status := domain.AttemptInterrupted
		class := domain.FailInfrastructure
		detail := "terminal run reconciliation"
		if r.Status == domain.RunCancelled {
			status = domain.AttemptCancelled
			class = domain.FailUser
			detail = "cancelled"
		}
		stages, _ := st.ListStages(ctx, r.ID)
		for _, stg := range stages {
			atts, _ := st.ListAttempts(ctx, stg.ID)
			for _, a := range atts {
				if a.Status != domain.AttemptRunning && a.Status != domain.AttemptPending {
					continue
				}
				_ = st.UpdateAttemptStatus(ctx, a.ID, status, class, detail)
			}
			if stg.Status == domain.AttemptRunning {
				_ = st.UpdateStageStatus(ctx, stg.ID, domain.AttemptInterrupted, domain.FailInfrastructure, detail)
			}
		}
		log.Info("reconciled terminal run attempts", "run", r.ID, "status", r.Status)
	}
}

// Reconcile runs at startup before the scheduler accepts new work. Ordering is
// deliberate: stale process trees are terminated before any workspace restore,
// then interrupted attempts are recovered, then reclaimable checkpoint material
// and debris are cleaned.
func Reconcile(ctx context.Context, st *storage.Store, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	if err := st.IntegrityCheck(ctx); err != nil {
		log.Error("storage integrity", "err", err)
		return err
	}

	// 1. Terminate stale process trees from a previous server before touching
	//    any workspace, so an old writer cannot race a restore.
	unsafe, err := reconcileProcesses(ctx, st, log)
	if err != nil {
		return fmt.Errorf("process reconciliation: %w", err)
	}

	// 2. Repair durable attempt state on terminal runs (cancellation crash
	//    window).
	reconcileTerminalRuns(ctx, st, log)

	// 3. Remove unreferenced recovery debris (staging, orphan checkpoint dirs).
	if n, err := st.CleanupCheckpointDebris(ctx); err == nil && n > 0 {
		log.Info("cleaned recovery debris", "removed", n)
	}

	// 4. Restore interrupted write attempts. Runs whose stale processes could
	//    not be reconciled are skipped and remain blocked.
	runs, err := st.ListActiveRuns(ctx)
	if err != nil {
		return err
	}
	for _, r := range runs {
		if unsafe[r.ID] {
			continue
		}
		stages, _ := st.ListStages(ctx, r.ID)
		for _, stg := range stages {
			atts, _ := st.ListAttempts(ctx, stg.ID)
			for _, a := range atts {
				if a.Status != domain.AttemptRunning && a.Status != domain.AttemptPending {
					continue
				}
				// Restore the checkpoint before recording the attempt as
				// interrupted. If recovery crashes mid-restore the attempt is
				// still durable as running, so the next startup repeats the
				// idempotent verify-and-restore instead of trusting a partial
				// workspace.
				if stg.Kind.WritesWorkspace() {
					if err := restoreWriteCheckpoint(ctx, st, r, stg, a, log); err != nil {
						log.Error("checkpoint restore failed", "run", r.ID, "stage", stg.ID, "attempt", a.ID, "err", err)
						_ = st.UpdateAttemptStatus(ctx, a.ID, domain.AttemptInterrupted, domain.FailInfrastructure, "server restart")
						_ = st.UpdateRunStatus(ctx, r.ID, domain.RunBlocked, domain.BlockedRecovery, "checkpoint restore failed: "+err.Error())
						_ = st.EmitEvent(ctx, "recovery.blocked", r.ID, map[string]any{"stageId": stg.ID, "attemptId": a.ID, "reason": err.Error()})
						if stg.Status == domain.AttemptRunning {
							_ = st.UpdateStageStatus(ctx, stg.ID, domain.AttemptInterrupted, domain.FailInfrastructure, "server restart")
						}
						continue
					}
				}
				_ = st.UpdateAttemptStatus(ctx, a.ID, domain.AttemptInterrupted, domain.FailInfrastructure, "server restart")
			}
			if stg.Status == domain.AttemptRunning {
				_ = st.UpdateStageStatus(ctx, stg.ID, domain.AttemptInterrupted, domain.FailInfrastructure, "server restart")
			}
		}
		// Any approval the interrupted attempt was waiting on can never be
		// answered; invalidate it so it cannot be resolved later or leave a
		// dangling attention item.
		_ = st.CancelPendingApprovalsForRun(ctx, r.ID, "recovery")
		log.Info("reconciled interrupted run", "run", r.ID, "status", r.Status)
	}

	reconcileJournals(ctx, st, log)
	cleanupValidationWorkspaces(st, log)

	// 5. Reclaim checkpoint material for terminal runs past retention. Active
	//    and recoverable checkpoints are pinned by the query.
	if n, err := st.ReclaimCheckpoints(ctx, storage.DefaultCheckpointRetain); err == nil && n > 0 {
		log.Info("reclaimed checkpoints", "removed", n)
	}
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
