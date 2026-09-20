package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
)

// InsertProbeOwner persists ownership of a discovery probe process tree before
// it is launched, so a crash cannot orphan a probe without a durable record.
func (s *Store) InsertProbeOwner(ctx context.Context, o *domain.ProbeOwner) error {
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
		_, err := tx.ExecContext(ctx, `INSERT INTO probe_owners(id, kind, token_hash, pgid, state, created_at, reconciled_at)
			VALUES (?,?,?,?,?,?,NULL)`,
			o.ID, o.Kind, o.TokenHash, o.PGID, o.State, o.CreatedAt.Format(time.RFC3339Nano))
		return err
	})
}

// SetProbeOwnerPGID records the process group of a launched probe tree. It is a
// secondary hint only; reconciliation is keyed on the ownership token.
func (s *Store) SetProbeOwnerPGID(ctx context.Context, id string, pgid int) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE probe_owners SET pgid = ? WHERE id = ?`, pgid, id)
	return err
}

// MarkProbeOwnerReconciled records that no owned probe process remains. It never
// touches a row that has already been reconciled.
func (s *Store) MarkProbeOwnerReconciled(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE probe_owners SET state = ?, reconciled_at = ? WHERE id = ? AND state = ?`,
		domain.ProcessOwnerReconciled, nowRFC3339(), id, domain.ProcessOwnerActive)
	return err
}

// ListActiveProbeOwners returns probe ownership records whose process tree has
// not been proven terminated.
func (s *Store) ListActiveProbeOwners(ctx context.Context) ([]domain.ProbeOwner, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, kind, token_hash, pgid, state, created_at, reconciled_at
		FROM probe_owners WHERE state = ? ORDER BY created_at ASC`, domain.ProcessOwnerActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ProbeOwner
	for rows.Next() {
		var o domain.ProbeOwner
		var created string
		var reconciled sql.NullString
		if err := rows.Scan(&o.ID, &o.Kind, &o.TokenHash, &o.PGID, &o.State, &created, &reconciled); err != nil {
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
