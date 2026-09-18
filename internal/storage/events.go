package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
)

type EventRecord struct {
	Seq            int64
	Type           string
	ProjectID      string
	ConversationID string
	RunID          string
	Payload        string
	CreatedAt      time.Time
}

func InsertEvent(ctx context.Context, tx *sql.Tx, ev EventRecord) (int64, error) {
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO events(type, project_id, conversation_id, run_id, payload, created_at) VALUES (?,?,?,?,?,?)`,
		ev.Type, ev.ProjectID, ev.ConversationID, ev.RunID, ev.Payload, ev.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	seq, err := res.LastInsertId()
	return seq, err
}

func InsertEventJSON(ctx context.Context, tx *sql.Tx, typ, projectID, conversationID, runID string, payload any) (int64, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	return InsertEvent(ctx, tx, EventRecord{
		Type:           typ,
		ProjectID:      projectID,
		ConversationID: conversationID,
		RunID:          runID,
		Payload:        string(b),
	})
}

func (s *Store) EventsSince(ctx context.Context, afterSeq int64, projectID, runID string, limit int) ([]domain.Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT seq, type, project_id, run_id, payload, created_at FROM events WHERE seq > ?`
	args := []any{afterSeq}
	if projectID != "" {
		q += ` AND project_id = ?`
		args = append(args, projectID)
	}
	if runID != "" {
		q += ` AND run_id = ?`
		args = append(args, runID)
	}
	q += ` ORDER BY seq ASC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		var created string
		if err := rows.Scan(&e.Seq, &e.Type, &e.ProjectID, &e.RunID, &e.Payload, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(created)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) LatestEventSeq(ctx context.Context) (int64, error) {
	var seq sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT MAX(seq) FROM events`).Scan(&seq)
	if err != nil {
		return 0, err
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}
