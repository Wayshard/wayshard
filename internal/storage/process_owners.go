package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
)

// InsertProcessOwner persists ownership of a process tree before it is
// launched, so a crash cannot orphan a process without a durable record.
func (s *Store) InsertProcessOwner(ctx context.Context, o *domain.ProcessOwner) error {
	if o.ID == "" {
		o.ID = id.New()
	}
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}
	if o.State == "" {
		o.State = domain.ProcessOwnerActive
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO process_owners(id, run_id, stage_id, attempt_id, token_hash, pgid, state, created_at, reconciled_at)
			VALUES (?,?,?,?,?,?,?,?,NULL)`,
			o.ID, o.RunID, o.StageID, o.AttemptID, o.TokenHash, o.PGID, o.State, o.CreatedAt.Format(time.RFC3339Nano))
		return err
	})
}

// SetProcessOwnerPGID records the process group of a launched tree. It is a
// secondary hint only; reconciliation is keyed on the ownership token.
func (s *Store) SetProcessOwnerPGID(ctx context.Context, id string, pgid int) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE process_owners SET pgid = ? WHERE id = ?`, pgid, id)
	return err
}

// MarkProcessOwnerReconciled records that no owned process remains. It never
// touches a row that has already been reconciled.
func (s *Store) MarkProcessOwnerReconciled(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE process_owners SET state = ?, reconciled_at = ? WHERE id = ? AND state = ?`,
		domain.ProcessOwnerReconciled, nowRFC3339(), id, domain.ProcessOwnerActive)
	return err
}

// ListActiveProcessOwners returns ownership records whose process tree has not
// been proven terminated.
func (s *Store) ListActiveProcessOwners(ctx context.Context) ([]domain.ProcessOwner, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, run_id, stage_id, attempt_id, token_hash, pgid, state, created_at, reconciled_at
		FROM process_owners WHERE state = ? ORDER BY created_at ASC`, domain.ProcessOwnerActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ProcessOwner
	for rows.Next() {
		var o domain.ProcessOwner
		var created string
		var reconciled sql.NullString
		if err := rows.Scan(&o.ID, &o.RunID, &o.StageID, &o.AttemptID, &o.TokenHash, &o.PGID, &o.State, &created, &reconciled); err != nil {
			return nil, err
		}
		o.CreatedAt = parseTime(created)
		if reconciled.Valid {
			t := parseTime(reconciled.String)
			o.ReconciledAt = &t
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
