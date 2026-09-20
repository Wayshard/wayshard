package storage

import (
	"context"
	"database/sql"

	"github.com/Wayshard/wayshard/internal/domain"
)

// CancelRun transitions a non-terminal run to CANCELLED together with its
// running/pending attempts, running stages and pending approvals in a single
// transaction. This removes the crash window in which a run is durably
// cancelled but an attempt is left running.
//
// It reports whether the run was transitioned (false when already terminal).
func (s *Store) CancelRun(ctx context.Context, runID, detail string) (bool, error) {
	changed := false
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM runs WHERE id = ?`, runID).Scan(&status); err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return err
		}
		if domain.RunStatus(status).Terminal() {
			return nil
		}
		now := nowRFC3339()

		if _, err := tx.ExecContext(ctx, `UPDATE runs SET status = ?, blocked_reason = ?, blocked_detail = ?, updated_at = ?, finished_at = ? WHERE id = ?`,
			string(domain.RunCancelled), string(domain.BlockedUser), detail, now, now, runID); err != nil {
			return err
		}

		attIDs, err := txIDs(ctx, tx, `SELECT id FROM stage_attempts WHERE run_id = ? AND status IN ('running','pending')`, runID)
		if err != nil {
			return err
		}
		for _, aid := range attIDs {
			if _, err := tx.ExecContext(ctx, `UPDATE stage_attempts SET status = ?, failure_class = ?, error = ?, finished_at = ? WHERE id = ?`,
				string(domain.AttemptCancelled), string(domain.FailUser), "cancelled", now, aid); err != nil {
				return err
			}
			if _, err := InsertEventJSON(ctx, tx, "stage.attempt.status", "", "", runID, map[string]any{
				"attemptId": aid, "status": domain.AttemptCancelled, "class": domain.FailUser,
			}); err != nil {
				return err
			}
		}

		stageIDs, err := txIDs(ctx, tx, `SELECT id FROM stages WHERE run_id = ? AND status = ?`, runID, string(domain.AttemptRunning))
		if err != nil {
			return err
		}
		for _, sid := range stageIDs {
			if _, err := tx.ExecContext(ctx, `UPDATE stages SET status = ?, updated_at = ? WHERE id = ?`,
				string(domain.AttemptInterrupted), now, sid); err != nil {
				return err
			}
			if _, err := InsertEventJSON(ctx, tx, "stage.status", "", "", runID, map[string]any{
				"stageId": sid, "status": domain.AttemptInterrupted, "class": domain.FailUser, "error": "cancelled",
			}); err != nil {
				return err
			}
		}

		apprIDs, err := txIDs(ctx, tx, `SELECT id FROM approvals WHERE run_id = ? AND status = 'pending'`, runID)
		if err != nil {
			return err
		}
		for _, pid := range apprIDs {
			if _, err := tx.ExecContext(ctx, `UPDATE approvals SET status = ?, resolved_by = ?, resolved_at = ? WHERE id = ? AND status = 'pending'`,
				"cancelled", "cancellation", now, pid); err != nil {
				return err
			}
			if _, err := InsertEventJSON(ctx, tx, "approval.resolved", "", "", runID, map[string]any{
				"id": pid, "status": "cancelled", "by": "cancellation",
			}); err != nil {
				return err
			}
		}

		if _, err := InsertEventJSON(ctx, tx, "run.status", "", "", runID, map[string]any{
			"runId": runID, "status": domain.RunCancelled, "reason": domain.BlockedUser, "detail": detail,
		}); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}

func txIDs(ctx context.Context, tx *sql.Tx, q string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
