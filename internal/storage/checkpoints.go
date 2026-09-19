package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
)

// InsertCheckpoint persists a durable pre-attempt workspace checkpoint and emits
// a high-level event. Callers must persist it before the write attempt begins.
func (s *Store) InsertCheckpoint(ctx context.Context, cp *domain.WorkspaceCheckpoint) error {
	if cp.ID == "" {
		cp.ID = id.New()
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_checkpoints(id, workspace_id, name, object_hash, created_at, run_id, stage_id, attempt_id, tree_hash, tree_path)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			cp.ID, cp.WorkspaceID, cp.Name, "", cp.CreatedAt.Format(time.RFC3339Nano), cp.RunID, cp.StageID, cp.AttemptID, cp.TreeHash, cp.TreePath); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "checkpoint.created", "", "", cp.RunID, map[string]any{
			"id": cp.ID, "stageId": cp.StageID, "attemptId": cp.AttemptID, "treeHash": cp.TreeHash,
		})
		return err
	})
}

func scanCheckpoint(row scanner) (*domain.WorkspaceCheckpoint, error) {
	var cp domain.WorkspaceCheckpoint
	var created string
	if err := row.Scan(&cp.ID, &cp.WorkspaceID, &cp.RunID, &cp.StageID, &cp.AttemptID, &cp.Name, &cp.TreeHash, &cp.TreePath, &created); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	cp.CreatedAt = parseTime(created)
	return &cp, nil
}

const checkpointCols = `id, workspace_id, run_id, stage_id, attempt_id, name, tree_hash, tree_path, created_at`

// EmitEvent records a durable high-level event outside a domain mutation.
func (s *Store) EmitEvent(ctx context.Context, typ, runID string, payload any) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := InsertEventJSON(ctx, tx, typ, "", "", runID, payload)
		return err
	})
}

func (s *Store) GetCheckpoint(ctx context.Context, id string) (*domain.WorkspaceCheckpoint, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+checkpointCols+` FROM workspace_checkpoints WHERE id = ?`, id)
	return scanCheckpoint(row)
}

// LatestCheckpointForStage returns the most recent checkpoint for a stage.
func (s *Store) LatestCheckpointForStage(ctx context.Context, stageID string) (*domain.WorkspaceCheckpoint, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+checkpointCols+` FROM workspace_checkpoints WHERE stage_id = ? ORDER BY created_at DESC LIMIT 1`, stageID)
	return scanCheckpoint(row)
}

// LatestCheckpointForAttempt returns the checkpoint created for an attempt.
func (s *Store) LatestCheckpointForAttempt(ctx context.Context, attemptID string) (*domain.WorkspaceCheckpoint, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+checkpointCols+` FROM workspace_checkpoints WHERE attempt_id = ? ORDER BY created_at DESC LIMIT 1`, attemptID)
	return scanCheckpoint(row)
}

func (s *Store) ListCheckpointsByRun(ctx context.Context, runID string) ([]domain.WorkspaceCheckpoint, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+checkpointCols+` FROM workspace_checkpoints WHERE run_id = ? ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WorkspaceCheckpoint
	for rows.Next() {
		cp, err := scanCheckpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *cp)
	}
	return out, rows.Err()
}

// SetAttemptCheckpoint links an attempt to its pre-attempt checkpoint.
func (s *Store) SetAttemptCheckpoint(ctx context.Context, attemptID, checkpointID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE stage_attempts SET checkpoint_id = ? WHERE id = ?`, checkpointID, attemptID)
	return err
}
