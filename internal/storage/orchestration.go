package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
)

func (s *Store) InsertConversation(ctx context.Context, c *domain.Conversation) error {
	if c.ID == "" {
		c.ID = id.New()
	}
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	pinned := 0
	if c.Pinned {
		pinned = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO conversations(id, project_id, title, pinned, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		c.ID, c.ProjectID, c.Title, pinned, c.CreatedAt.Format(time.RFC3339Nano), c.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListConversations(ctx context.Context, projectID string) ([]domain.Conversation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, project_id, title, pinned, created_at, updated_at FROM conversations WHERE project_id = ? ORDER BY updated_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Conversation
	for rows.Next() {
		var c domain.Conversation
		var pinned int
		var created, updated string
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Title, &pinned, &created, &updated); err != nil {
			return nil, err
		}
		c.Pinned = pinned == 1
		c.CreatedAt = parseTime(created)
		c.UpdatedAt = parseTime(updated)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetConversation(ctx context.Context, id string) (*domain.Conversation, error) {
	var c domain.Conversation
	var pinned int
	var created, updated string
	err := s.DB.QueryRowContext(ctx, `SELECT id, project_id, title, pinned, created_at, updated_at FROM conversations WHERE id = ?`, id).
		Scan(&c.ID, &c.ProjectID, &c.Title, &pinned, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Pinned = pinned == 1
	c.CreatedAt = parseTime(created)
	c.UpdatedAt = parseTime(updated)
	return &c, nil
}

func (s *Store) InsertMessage(ctx context.Context, m *domain.Message) error {
	if m.ID == "" {
		m.ID = id.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO messages(id, conversation_id, role, body, task_id, created_at) VALUES (?,?,?,?,?,?)`,
		m.ID, m.ConversationID, string(m.Role), m.Body, m.TaskID, m.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListMessages(ctx context.Context, conversationID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, conversation_id, role, body, COALESCE(task_id,''), created_at FROM messages WHERE conversation_id = ? ORDER BY created_at ASC LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Message
	for rows.Next() {
		var m domain.Message
		var role, created string
		if err := rows.Scan(&m.ID, &m.ConversationID, &role, &m.Body, &m.TaskID, &created); err != nil {
			return nil, err
		}
		m.Role = domain.MessageRole(role)
		m.CreatedAt = parseTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

// CreateTaskRun inserts Message + Task + Run in one transaction and emits events.
func (s *Store) CreateTaskRun(ctx context.Context, msg *domain.Message, task *domain.Task, run *domain.Run) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if msg.ID == "" {
			msg.ID = id.New()
		}
		if task.ID == "" {
			task.ID = id.New()
		}
		if run.ID == "" {
			run.ID = id.New()
		}
		now := time.Now().UTC()
		if msg.CreatedAt.IsZero() {
			msg.CreatedAt = now
		}
		msg.TaskID = task.ID
		task.MessageID = msg.ID
		if task.CreatedAt.IsZero() {
			task.CreatedAt = now
		}
		if run.CreatedAt.IsZero() {
			run.CreatedAt = now
		}
		run.UpdatedAt = now
		run.TaskID = task.ID
		if run.Status == "" {
			run.Status = domain.RunQueued
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO messages(id, conversation_id, role, body, task_id, created_at) VALUES (?,?,?,?,?,?)`,
			msg.ID, msg.ConversationID, string(msg.Role), msg.Body, task.ID, msg.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		ao := 0
		if task.ArtifactOnly {
			ao = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tasks(id, project_id, conversation_id, message_id, objective, artifact_only, created_at) VALUES (?,?,?,?,?,?,?)`,
			task.ID, task.ProjectID, task.ConversationID, msg.ID, task.Objective, ao, task.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		deg := 0
		if run.DegradedRouting {
			deg = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO runs(id, task_id, project_id, conversation_id, status, blocked_reason, blocked_detail, profile, degraded_routing, idempotency_key, created_at, updated_at, started_at, finished_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			run.ID, run.TaskID, run.ProjectID, run.ConversationID, string(run.Status), string(run.BlockedReason), run.BlockedDetail, string(run.Profile), deg, run.IdempotencyKey,
			run.CreatedAt.Format(time.RFC3339Nano), run.UpdatedAt.Format(time.RFC3339Nano), nullTime(run.StartedAt), nullTime(run.FinishedAt)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE conversations SET updated_at = ? WHERE id = ?`, now.Format(time.RFC3339Nano), msg.ConversationID); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "run.created", run.ProjectID, run.ConversationID, run.ID, map[string]any{
			"runId": run.ID, "taskId": task.ID, "status": run.Status, "objective": task.Objective,
		})
		return err
	})
}

func (s *Store) GetRun(ctx context.Context, id string) (*domain.Run, error) {
	var r domain.Run
	var status, reason, profile, created, updated string
	var started, finished sql.NullString
	var deg int
	err := s.DB.QueryRowContext(ctx, `SELECT id, task_id, project_id, conversation_id, status, blocked_reason, blocked_detail, profile, degraded_routing, idempotency_key, created_at, updated_at, started_at, finished_at FROM runs WHERE id = ?`, id).
		Scan(&r.ID, &r.TaskID, &r.ProjectID, &r.ConversationID, &status, &reason, &r.BlockedDetail, &profile, &deg, &r.IdempotencyKey, &created, &updated, &started, &finished)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.Status = domain.RunStatus(status)
	r.BlockedReason = domain.BlockedReason(reason)
	r.Profile = domain.RoutingProfile(profile)
	r.DegradedRouting = deg == 1
	r.CreatedAt = parseTime(created)
	r.UpdatedAt = parseTime(updated)
	r.StartedAt = scanNullTime(started)
	r.FinishedAt = scanNullTime(finished)
	return &r, nil
}

func (s *Store) ListRuns(ctx context.Context, projectID string, limit int) ([]domain.Run, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, task_id, project_id, conversation_id, status, blocked_reason, blocked_detail, profile, degraded_routing, idempotency_key, created_at, updated_at, started_at, finished_at FROM runs WHERE project_id = ? ORDER BY created_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Run
	for rows.Next() {
		var r domain.Run
		var status, reason, profile, created, updated string
		var started, finished sql.NullString
		var deg int
		if err := rows.Scan(&r.ID, &r.TaskID, &r.ProjectID, &r.ConversationID, &status, &reason, &r.BlockedDetail, &profile, &deg, &r.IdempotencyKey, &created, &updated, &started, &finished); err != nil {
			return nil, err
		}
		r.Status = domain.RunStatus(status)
		r.BlockedReason = domain.BlockedReason(reason)
		r.Profile = domain.RoutingProfile(profile)
		r.DegradedRouting = deg == 1
		r.CreatedAt = parseTime(created)
		r.UpdatedAt = parseTime(updated)
		r.StartedAt = scanNullTime(started)
		r.FinishedAt = scanNullTime(finished)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListActiveRuns(ctx context.Context) ([]domain.Run, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM runs WHERE status NOT IN ('complete','failed','cancelled')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	var out []domain.Run
	for _, id := range ids {
		r, err := s.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, nil
}

func (s *Store) UpdateRunStatus(ctx context.Context, runID string, status domain.RunStatus, reason domain.BlockedReason, detail string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		now := nowRFC3339()
		var finished any
		if status.Terminal() {
			finished = now
		}
		res, err := tx.ExecContext(ctx, `UPDATE runs SET status = ?, blocked_reason = ?, blocked_detail = ?, updated_at = ?, finished_at = COALESCE(?, finished_at) WHERE id = ?`,
			string(status), string(reason), detail, now, finished, runID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrNotFound
		}
		_, err = InsertEventJSON(ctx, tx, "run.status", "", "", runID, map[string]any{
			"runId": runID, "status": status, "reason": reason, "detail": detail,
		})
		return err
	})
}

func (s *Store) CompareAndSetRunStatus(ctx context.Context, runID string, from, to domain.RunStatus, reason domain.BlockedReason, detail string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		now := nowRFC3339()
		var finished any
		if to.Terminal() {
			finished = now
		}
		res, err := tx.ExecContext(ctx, `UPDATE runs SET status = ?, blocked_reason = ?, blocked_detail = ?, updated_at = ?, finished_at = COALESCE(?, finished_at) WHERE id = ? AND status = ?`,
			string(to), string(reason), detail, now, finished, runID, string(from))
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrConflict
		}
		_, err = InsertEventJSON(ctx, tx, "run.status", "", "", runID, map[string]any{
			"runId": runID, "from": from, "status": to, "reason": reason, "detail": detail,
		})
		return err
	})
}

func (s *Store) AppendStage(ctx context.Context, st *domain.Stage) error {
	if st.ID == "" {
		st.ID = id.New()
	}
	now := time.Now().UTC()
	if st.CreatedAt.IsZero() {
		st.CreatedAt = now
	}
	st.UpdatedAt = now
	if st.Status == "" {
		st.Status = domain.AttemptPending
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO stages(id, run_id, kind, ordinal, status, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
			st.ID, st.RunID, string(st.Kind), st.Ordinal, string(st.Status), st.CreatedAt.Format(time.RFC3339Nano), st.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "stage.appended", "", "", st.RunID, map[string]any{
			"stageId": st.ID, "kind": st.Kind, "ordinal": st.Ordinal,
		})
		return err
	})
}

func (s *Store) ListStages(ctx context.Context, runID string) ([]domain.Stage, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, run_id, kind, ordinal, status, created_at, updated_at FROM stages WHERE run_id = ? ORDER BY ordinal ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Stage
	for rows.Next() {
		var st domain.Stage
		var kind, status, created, updated string
		if err := rows.Scan(&st.ID, &st.RunID, &kind, &st.Ordinal, &status, &created, &updated); err != nil {
			return nil, err
		}
		st.Kind = domain.StageKind(kind)
		st.Status = domain.StageAttemptStatus(status)
		st.CreatedAt = parseTime(created)
		st.UpdatedAt = parseTime(updated)
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) AppendAttempt(ctx context.Context, a *domain.StageAttempt) error {
	if a.ID == "" {
		a.ID = id.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	if a.Status == "" {
		a.Status = domain.AttemptPending
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO stage_attempts(id, stage_id, run_id, ordinal, status, harness_id, model_id, failure_class, error, checkpoint_id, started_at, finished_at, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.StageID, a.RunID, a.Ordinal, string(a.Status), a.HarnessID, a.ModelID, string(a.FailureClass), a.Error, a.CheckpointID,
			nullTime(a.StartedAt), nullTime(a.FinishedAt), a.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "stage.attempt", "", "", a.RunID, map[string]any{
			"attemptId": a.ID, "stageId": a.StageID, "ordinal": a.Ordinal, "status": a.Status,
		})
		return err
	})
}

func (s *Store) UpdateAttemptStatus(ctx context.Context, attemptID string, status domain.StageAttemptStatus, class domain.FailureClass, errMsg string) error {
	now := nowRFC3339()
	var finished any
	if status != domain.AttemptPending && status != domain.AttemptRunning {
		finished = now
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE stage_attempts SET status = ?, failure_class = ?, error = ?, finished_at = COALESCE(?, finished_at) WHERE id = ?`,
			string(status), string(class), errMsg, finished, attemptID)
		if err != nil {
			return err
		}
		var runID string
		_ = tx.QueryRowContext(ctx, `SELECT run_id FROM stage_attempts WHERE id = ?`, attemptID).Scan(&runID)
		_, err = InsertEventJSON(ctx, tx, "stage.attempt.status", "", "", runID, map[string]any{
			"attemptId": attemptID, "status": status, "class": class,
		})
		return err
	})
}

// UpdateStageStatus records the terminal status of a stage. Stages must not
// remain "running" after their active attempt ends.
func (s *Store) UpdateStageStatus(ctx context.Context, stageID string, status domain.StageAttemptStatus, class domain.FailureClass, errMsg string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		var runID string
		if err := tx.QueryRowContext(ctx, `SELECT run_id FROM stages WHERE id = ?`, stageID).Scan(&runID); err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE stages SET status = ?, updated_at = ? WHERE id = ?`,
			string(status), nowRFC3339(), stageID); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "stage.status", "", "", runID, map[string]any{
			"stageId": stageID, "status": status, "class": class, "error": errMsg,
		})
		return err
	})
}

// CountStagesByKind returns how many stages of a kind exist for a run.
func (s *Store) CountStagesByKind(ctx context.Context, runID string, kind domain.StageKind) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM stages WHERE run_id = ? AND kind = ?`, runID, string(kind)).Scan(&n)
	return n, err
}

// CountStages returns the total number of stages for a run.
func (s *Store) CountStages(ctx context.Context, runID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM stages WHERE run_id = ?`, runID).Scan(&n)
	return n, err
}

// PendingApprovalCount returns unresolved approvals for a run.
func (s *Store) PendingApprovalCount(ctx context.Context, runID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM approvals WHERE run_id = ? AND status = 'pending'`, runID).Scan(&n)
	return n, err
}

// ResetStaleRunningStages marks any stage still running at startup as interrupted.
func (s *Store) ResetStaleRunningStages(ctx context.Context, runID string) error {
	stages, err := s.ListStages(ctx, runID)
	if err != nil {
		return err
	}
	for _, st := range stages {
		if st.Status == domain.AttemptRunning {
			_ = s.UpdateStageStatus(ctx, st.ID, domain.AttemptInterrupted, domain.FailInfrastructure, "server restart or terminal run")
		}
	}
	return nil
}

func (s *Store) ListAttempts(ctx context.Context, stageID string) ([]domain.StageAttempt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, stage_id, run_id, ordinal, status, harness_id, model_id, failure_class, error, checkpoint_id, started_at, finished_at, created_at FROM stage_attempts WHERE stage_id = ? ORDER BY ordinal ASC`, stageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StageAttempt
	for rows.Next() {
		var a domain.StageAttempt
		var status, class, created string
		var started, finished sql.NullString
		if err := rows.Scan(&a.ID, &a.StageID, &a.RunID, &a.Ordinal, &status, &a.HarnessID, &a.ModelID, &class, &a.Error, &a.CheckpointID, &started, &finished, &created); err != nil {
			return nil, err
		}
		a.Status = domain.StageAttemptStatus(status)
		a.FailureClass = domain.FailureClass(class)
		a.StartedAt = scanNullTime(started)
		a.FinishedAt = scanNullTime(finished)
		a.CreatedAt = parseTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) InsertAssessment(ctx context.Context, a *domain.TaskAssessment) error {
	if a.ID == "" {
		a.ID = id.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	deg := 0
	if a.Degraded {
		deg = 1
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO assessments(id, run_id, jev_model, question_set, policy_version, dimensions_json, input_hash, usage_json, degraded, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.RunID, a.JevModel, a.QuestionSet, a.PolicyVersion, a.DimensionsJSON, a.InputHash, a.UsageJSON, deg, a.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "assessment.recorded", "", "", a.RunID, map[string]any{"id": a.ID, "degraded": a.Degraded})
		return err
	})
}

func (s *Store) InsertRouteDecision(ctx context.Context, d *domain.RouteDecision) error {
	if d.ID == "" {
		d.ID = id.New()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	deg := 0
	if d.Degraded {
		deg = 1
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO route_decisions(id, run_id, stage_id, assessment_id, harness_id, model_id, effort, profile, isolation, fallbacks_json, policy_version, reason, degraded, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			d.ID, d.RunID, d.StageID, d.AssessmentID, d.HarnessID, d.ModelID, d.Effort, string(d.Profile), string(d.Isolation), d.FallbacksJSON, d.PolicyVersion, d.Reason, deg, d.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "route.decided", "", "", d.RunID, map[string]any{
			"id": d.ID, "harnessId": d.HarnessID, "modelId": d.ModelID, "degraded": d.Degraded,
		})
		return err
	})
}

func (s *Store) InsertArtifact(ctx context.Context, a *domain.Artifact) error {
	if a.ID == "" {
		a.ID = id.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	valid := 0
	if a.Valid {
		valid = 1
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO artifacts(id, run_id, stage_id, attempt_id, kind, schema_ver, object_hash, json, valid, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.RunID, a.StageID, a.AttemptID, string(a.Kind), a.SchemaVer, a.ObjectHash, a.JSON, valid, a.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "artifact.created", "", "", a.RunID, map[string]any{
			"id": a.ID, "kind": a.Kind, "valid": a.Valid,
		})
		return err
	})
}

func (s *Store) ListArtifacts(ctx context.Context, runID string) ([]domain.Artifact, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, run_id, stage_id, attempt_id, kind, schema_ver, object_hash, json, valid, created_at FROM artifacts WHERE run_id = ? ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Artifact
	for rows.Next() {
		var a domain.Artifact
		var kind, created string
		var valid int
		if err := rows.Scan(&a.ID, &a.RunID, &a.StageID, &a.AttemptID, &kind, &a.SchemaVer, &a.ObjectHash, &a.JSON, &valid, &created); err != nil {
			return nil, err
		}
		a.Kind = domain.ArtifactKind(kind)
		a.Valid = valid == 1
		a.CreatedAt = parseTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) LatestArtifact(ctx context.Context, runID string, kind domain.ArtifactKind) (*domain.Artifact, error) {
	arts, err := s.ListArtifacts(ctx, runID)
	if err != nil {
		return nil, err
	}
	for i := len(arts) - 1; i >= 0; i-- {
		if arts[i].Kind == kind && arts[i].Valid {
			a := arts[i]
			return &a, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) GetIdempotency(ctx context.Context, key, deviceID, reqHash string) (string, bool, error) {
	var storedHash, response string
	err := s.DB.QueryRowContext(ctx, `SELECT request_hash, response FROM idempotency_keys WHERE key = ? AND device_id = ?`, key, deviceID).Scan(&storedHash, &response)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if storedHash != reqHash {
		return "", false, ErrConflict
	}
	return response, true, nil
}

func (s *Store) PutIdempotency(ctx context.Context, key, deviceID, reqHash, response string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO idempotency_keys(key, device_id, request_hash, response, created_at) VALUES (?,?,?,?,?)`,
		key, deviceID, reqHash, response, nowRFC3339())
	return err
}
