package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
)

func (s *Store) UpsertSetting(ctx context.Context, set domain.Setting) error {
	set.UpdatedAt = time.Now().UTC()
	if set.Revision == 0 {
		set.Revision = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO settings(scope, scope_id, key, value_json, revision, updated_at) VALUES (?,?,?,?,?,?)
		ON CONFLICT(scope, scope_id, key) DO UPDATE SET value_json = excluded.value_json, revision = settings.revision + 1, updated_at = excluded.updated_at`,
		string(set.Scope), set.ScopeID, set.Key, set.ValueJSON, set.Revision, set.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetSetting(ctx context.Context, scope domain.SettingScope, scopeID, key string) (*domain.Setting, error) {
	var set domain.Setting
	var sc, updated string
	err := s.DB.QueryRowContext(ctx, `SELECT scope, scope_id, key, value_json, revision, updated_at FROM settings WHERE scope = ? AND scope_id = ? AND key = ?`,
		string(scope), scopeID, key).Scan(&sc, &set.ScopeID, &set.Key, &set.ValueJSON, &set.Revision, &updated)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	set.Scope = domain.SettingScope(sc)
	set.UpdatedAt = parseTime(updated)
	return &set, nil
}

func (s *Store) ListSettings(ctx context.Context, scope domain.SettingScope, scopeID string) ([]domain.Setting, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT scope, scope_id, key, value_json, revision, updated_at FROM settings WHERE scope = ? AND scope_id = ?`, string(scope), scopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Setting
	for rows.Next() {
		var set domain.Setting
		var sc, updated string
		if err := rows.Scan(&sc, &set.ScopeID, &set.Key, &set.ValueJSON, &set.Revision, &updated); err != nil {
			return nil, err
		}
		set.Scope = domain.SettingScope(sc)
		set.UpdatedAt = parseTime(updated)
		out = append(out, set)
	}
	return out, rows.Err()
}

func (s *Store) InsertNotification(ctx context.Context, n *domain.Notification) error {
	if n.ID == "" {
		n.ID = id.New()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	att := 0
	if n.Attention {
		att = 1
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notifications(id, kind, project_id, run_id, title, body, attention, read_at, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			n.ID, string(n.Kind), n.ProjectID, n.RunID, n.Title, n.Body, att, nullTime(n.ReadAt), n.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "notification.created", n.ProjectID, "", n.RunID, map[string]any{
			"id": n.ID, "kind": n.Kind, "title": n.Title, "attention": n.Attention,
		})
		return err
	})
}

func (s *Store) ListNotifications(ctx context.Context, unreadOnly bool, limit int) ([]domain.Notification, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT id, kind, project_id, run_id, title, body, attention, read_at, created_at FROM notifications`
	if unreadOnly {
		q += ` WHERE read_at IS NULL`
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	rows, err := s.DB.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Notification
	for rows.Next() {
		var n domain.Notification
		var kind, created string
		var att int
		var read sql.NullString
		if err := rows.Scan(&n.ID, &kind, &n.ProjectID, &n.RunID, &n.Title, &n.Body, &att, &read, &created); err != nil {
			return nil, err
		}
		n.Kind = domain.NotificationKind(kind)
		n.Attention = att == 1
		n.ReadAt = scanNullTime(read)
		n.CreatedAt = parseTime(created)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) MarkNotificationRead(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE id = ?`, nowRFC3339(), id)
	return err
}

func (s *Store) ClearAttention(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE notifications SET attention = 0 WHERE id = ?`, id)
	return err
}

func (s *Store) InsertApproval(ctx context.Context, a *domain.Approval) error {
	if a.ID == "" {
		a.ID = id.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	if a.Status == "" {
		a.Status = "pending"
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO approvals(id, run_id, stage_id, kind, resource, reason, scopes_json, status, resolved_by, created_at, resolved_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.RunID, a.StageID, a.Kind, a.Resource, a.Reason, a.ScopesJSON, a.Status, a.ResolvedBy, a.CreatedAt.Format(time.RFC3339Nano), nullTime(a.ResolvedAt)); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "approval.requested", "", "", a.RunID, map[string]any{
			"id": a.ID, "kind": a.Kind, "resource": a.Resource,
		})
		return err
	})
}

func (s *Store) ResolveApproval(ctx context.Context, id, status, by string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE approvals SET status = ?, resolved_by = ?, resolved_at = ? WHERE id = ? AND status = 'pending'`,
			status, by, nowRFC3339(), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrConflict
		}
		_, err = InsertEventJSON(ctx, tx, "approval.resolved", "", "", "", map[string]any{"id": id, "status": status, "by": by})
		return err
	})
}

func (s *Store) GetApproval(ctx context.Context, id string) (*domain.Approval, error) {
	var a domain.Approval
	var created string
	var resolved sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT id, run_id, stage_id, kind, resource, reason, scopes_json, status, resolved_by, created_at, resolved_at FROM approvals WHERE id = ?`, id).
		Scan(&a.ID, &a.RunID, &a.StageID, &a.Kind, &a.Resource, &a.Reason, &a.ScopesJSON, &a.Status, &a.ResolvedBy, &created, &resolved)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt = parseTime(created)
	a.ResolvedAt = scanNullTime(resolved)
	return &a, nil
}

func (s *Store) ListPendingApprovals(ctx context.Context) ([]domain.Approval, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM approvals WHERE status = 'pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Approval
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		a, err := s.GetApproval(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, nil
}

func (s *Store) UpsertHarnessInstallation(ctx context.Context, h *domain.HarnessInstallation) error {
	if h.ID == "" {
		h.ID = id.New()
	}
	if h.LastProbedAt.IsZero() {
		h.LastProbedAt = time.Now().UTC()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO harness_installations(id, definition_id, display_name, executable, version, adapter, health, compatibility, isolation, auth_status, capabilities_json, models_json, last_probed_at, notes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(executable) DO UPDATE SET
			display_name = excluded.display_name,
			version = excluded.version,
			adapter = excluded.adapter,
			health = excluded.health,
			compatibility = excluded.compatibility,
			isolation = excluded.isolation,
			auth_status = excluded.auth_status,
			capabilities_json = excluded.capabilities_json,
			models_json = excluded.models_json,
			last_probed_at = excluded.last_probed_at,
			notes = excluded.notes`,
		h.ID, h.DefinitionID, h.DisplayName, h.Executable, h.Version, h.Adapter, string(h.Health), string(h.Compatibility), string(h.Isolation), h.AuthStatus, h.CapabilitiesJSON, h.ModelsJSON, h.LastProbedAt.Format(time.RFC3339Nano), h.Notes)
	return err
}

func (s *Store) ListHarnessInstallations(ctx context.Context) ([]domain.HarnessInstallation, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, definition_id, display_name, executable, version, adapter, health, compatibility, isolation, auth_status, capabilities_json, models_json, last_probed_at, notes FROM harness_installations ORDER BY display_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.HarnessInstallation
	for rows.Next() {
		var h domain.HarnessInstallation
		var health, compat, iso, probed string
		if err := rows.Scan(&h.ID, &h.DefinitionID, &h.DisplayName, &h.Executable, &h.Version, &h.Adapter, &health, &compat, &iso, &h.AuthStatus, &h.CapabilitiesJSON, &h.ModelsJSON, &probed, &h.Notes); err != nil {
			return nil, err
		}
		h.Health = domain.HarnessHealth(health)
		h.Compatibility = domain.CompatibilityClass(compat)
		h.Isolation = domain.IsolationMode(iso)
		h.LastProbedAt = parseTime(probed)
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) GetHarness(ctx context.Context, id string) (*domain.HarnessInstallation, error) {
	list, err := s.ListHarnessInstallations(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) InsertWorkspace(ctx context.Context, w *domain.WorkspaceRecord) error {
	if w.ID == "" {
		w.ID = id.New()
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now().UTC()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO workspaces(id, run_id, project_id, kind, source_path, run_path, snapshot_id, branch, head, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		w.ID, w.RunID, w.ProjectID, w.Kind, w.SourcePath, w.RunPath, w.SnapshotID, w.Branch, w.HEAD, w.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetWorkspaceByRun(ctx context.Context, runID string) (*domain.WorkspaceRecord, error) {
	var w domain.WorkspaceRecord
	var created string
	err := s.DB.QueryRowContext(ctx, `SELECT id, run_id, project_id, kind, source_path, run_path, snapshot_id, branch, head, created_at FROM workspaces WHERE run_id = ?`, runID).
		Scan(&w.ID, &w.RunID, &w.ProjectID, &w.Kind, &w.SourcePath, &w.RunPath, &w.SnapshotID, &w.Branch, &w.HEAD, &created)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	w.CreatedAt = parseTime(created)
	return &w, nil
}

func (s *Store) InsertRunDelta(ctx context.Context, runID, summaryJSON, objectHash string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO run_deltas(id, run_id, summary_json, object_hash, created_at) VALUES (?,?,?,?,?)`,
		id.New(), runID, summaryJSON, objectHash, nowRFC3339())
	return err
}

func (s *Store) LatestRunDelta(ctx context.Context, runID string) (string, error) {
	var js string
	err := s.DB.QueryRowContext(ctx, `SELECT summary_json FROM run_deltas WHERE run_id = ? ORDER BY created_at DESC LIMIT 1`, runID).Scan(&js)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return js, err
}

func (s *Store) InsertSnapshot(ctx context.Context, snap *domain.Snapshot) error {
	if snap.ID == "" {
		snap.ID = id.New()
	}
	if snap.CreatedAt.IsZero() {
		snap.CreatedAt = time.Now().UTC()
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO workspace_snapshots(id, workspace_id, branch, head, dirty_json, knowledge_rev, object_hash, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		snap.ID, snap.WorkspaceID, snap.Branch, snap.HEAD, snap.DirtyJSON, snap.KnowledgeRev, snap.ObjectHash, snap.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetSnapshot(ctx context.Context, id string) (*domain.Snapshot, error) {
	var snap domain.Snapshot
	var created string
	err := s.DB.QueryRowContext(ctx, `SELECT id, workspace_id, branch, head, dirty_json, knowledge_rev, object_hash, created_at FROM workspace_snapshots WHERE id = ?`, id).
		Scan(&snap.ID, &snap.WorkspaceID, &snap.Branch, &snap.HEAD, &snap.DirtyJSON, &snap.KnowledgeRev, &snap.ObjectHash, &created)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	snap.CreatedAt = parseTime(created)
	return &snap, nil
}

func (s *Store) InsertIntegration(ctx context.Context, in *domain.Integration) error {
	if in.ID == "" {
		in.ID = id.New()
	}
	now := time.Now().UTC()
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO integrations(id, run_id, project_id, status, base_snapshot, target_branch, current_branch, journal_hash, error, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			in.ID, in.RunID, in.ProjectID, in.Status, in.BaseSnapshot, in.TargetBranch, in.CurrentBranch, in.JournalHash, in.Error, in.CreatedAt.Format(time.RFC3339Nano), in.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "integration.updated", in.ProjectID, "", in.RunID, map[string]any{
			"id": in.ID, "status": in.Status,
		})
		return err
	})
}

func (s *Store) UpdateIntegration(ctx context.Context, in *domain.Integration) error {
	in.UpdatedAt = time.Now().UTC()
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE integrations SET status = ?, current_branch = ?, journal_hash = ?, error = ?, updated_at = ? WHERE id = ?`,
			in.Status, in.CurrentBranch, in.JournalHash, in.Error, in.UpdatedAt.Format(time.RFC3339Nano), in.ID); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "integration.updated", in.ProjectID, "", in.RunID, map[string]any{
			"id": in.ID, "status": in.Status, "error": in.Error,
		})
		return err
	})
}

func (s *Store) GetIntegrationByRun(ctx context.Context, runID string) (*domain.Integration, error) {
	var in domain.Integration
	var created, updated string
	err := s.DB.QueryRowContext(ctx, `SELECT id, run_id, project_id, status, base_snapshot, target_branch, current_branch, journal_hash, error, created_at, updated_at FROM integrations WHERE run_id = ? ORDER BY created_at DESC LIMIT 1`, runID).
		Scan(&in.ID, &in.RunID, &in.ProjectID, &in.Status, &in.BaseSnapshot, &in.TargetBranch, &in.CurrentBranch, &in.JournalHash, &in.Error, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	in.CreatedAt = parseTime(created)
	in.UpdatedAt = parseTime(updated)
	return &in, nil
}

func (s *Store) InsertJournalEntry(ctx context.Context, integrationID, path, before, after, status string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO publication_journal(id, integration_id, path, before_hash, after_hash, status, created_at) VALUES (?,?,?,?,?,?,?)`,
		id.New(), integrationID, path, before, after, status, nowRFC3339())
	return err
}

func (s *Store) ListJournal(ctx context.Context, integrationID string) ([]map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT path, before_hash, after_hash, status FROM publication_journal WHERE integration_id = ?`, integrationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]string
	for rows.Next() {
		var path, before, after, status string
		if err := rows.Scan(&path, &before, &after, &status); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"path": path, "before": before, "after": after, "status": status})
	}
	return out, rows.Err()
}

func (s *Store) InsertUsage(ctx context.Context, u *domain.Usage) error {
	if u.ID == "" {
		u.ID = id.New()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	est := 0
	if u.Estimated {
		est = 1
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO model_usage(id, run_id, stage_id, attempt_id, harness_id, model_id, provider, input_tokens, output_tokens, cache_tokens, cost_micros, estimated, pricing_json, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.ID, u.RunID, u.StageID, u.AttemptID, u.HarnessID, u.ModelID, u.Provider, u.InputTokens, u.OutputTokens, u.CacheTokens, u.CostMicros, est, u.PricingJSON, u.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListUsage(ctx context.Context, runID string) ([]domain.Usage, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, run_id, stage_id, attempt_id, harness_id, model_id, provider, input_tokens, output_tokens, cache_tokens, cost_micros, estimated, pricing_json, created_at FROM model_usage WHERE run_id = ?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Usage
	for rows.Next() {
		var u domain.Usage
		var est int
		var created string
		if err := rows.Scan(&u.ID, &u.RunID, &u.StageID, &u.AttemptID, &u.HarnessID, &u.ModelID, &u.Provider, &u.InputTokens, &u.OutputTokens, &u.CacheTokens, &u.CostMicros, &est, &u.PricingJSON, &created); err != nil {
			return nil, err
		}
		u.Estimated = est == 1
		u.CreatedAt = parseTime(created)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) InsertPTY(ctx context.Context, ptyID, projectID, deviceID, cwd string) error {
	now := nowRFC3339()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO pty_sessions(id, project_id, device_id, cwd, alive, created_at, last_attached_at) VALUES (?,?,?,?,1,?,?)`,
		ptyID, projectID, deviceID, cwd, now, now)
	return err
}

func (s *Store) SetPTYAlive(ctx context.Context, ptyID string, alive bool) error {
	v := 0
	if alive {
		v = 1
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE pty_sessions SET alive = ?, last_attached_at = ? WHERE id = ?`, v, nowRFC3339(), ptyID)
	return err
}

func (s *Store) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	var t domain.Task
	var created string
	var ao int
	err := s.DB.QueryRowContext(ctx, `SELECT id, project_id, conversation_id, message_id, objective, artifact_only, created_at FROM tasks WHERE id = ?`, id).
		Scan(&t.ID, &t.ProjectID, &t.ConversationID, &t.MessageID, &t.Objective, &ao, &created)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.ArtifactOnly = ao == 1
	t.CreatedAt = parseTime(created)
	return &t, nil
}

func (s *Store) ListRouteDecisions(ctx context.Context, runID string) ([]domain.RouteDecision, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, run_id, stage_id, assessment_id, harness_id, model_id, effort, profile, isolation, fallbacks_json, policy_version, reason, degraded, created_at FROM route_decisions WHERE run_id = ? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RouteDecision
	for rows.Next() {
		var d domain.RouteDecision
		var profile, iso, created string
		var deg int
		if err := rows.Scan(&d.ID, &d.RunID, &d.StageID, &d.AssessmentID, &d.HarnessID, &d.ModelID, &d.Effort, &profile, &iso, &d.FallbacksJSON, &d.PolicyVersion, &d.Reason, &deg, &created); err != nil {
			return nil, err
		}
		d.Profile = domain.RoutingProfile(profile)
		d.Isolation = domain.IsolationMode(iso)
		d.Degraded = deg == 1
		d.CreatedAt = parseTime(created)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListAssessments(ctx context.Context, runID string) ([]domain.TaskAssessment, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, run_id, jev_model, question_set, policy_version, dimensions_json, input_hash, usage_json, degraded, created_at FROM assessments WHERE run_id = ? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TaskAssessment
	for rows.Next() {
		var a domain.TaskAssessment
		var created string
		var deg int
		if err := rows.Scan(&a.ID, &a.RunID, &a.JevModel, &a.QuestionSet, &a.PolicyVersion, &a.DimensionsJSON, &a.InputHash, &a.UsageJSON, &deg, &created); err != nil {
			return nil, err
		}
		a.Degraded = deg == 1
		a.CreatedAt = parseTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}
