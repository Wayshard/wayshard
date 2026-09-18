package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
)

func (s *Store) InsertProject(ctx context.Context, p *domain.Project) error {
	if p.ID == "" {
		p.ID = id.New()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	if p.LastOpenedAt.IsZero() {
		p.LastOpenedAt = now
	}
	if p.Status == "" {
		p.Status = domain.ProjectAvailable
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO projects(id, name, path, source_kind, repo_identity, git_remote, default_branch, status, knowledge_rev, created_at, updated_at, last_opened_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ID, p.Name, p.Path, p.SourceKind, p.RepoIdentity, p.GitRemote, p.DefaultBranch, string(p.Status), p.KnowledgeRev,
			p.CreatedAt.Format(time.RFC3339Nano), p.UpdatedAt.Format(time.RFC3339Nano), p.LastOpenedAt.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		_, err = InsertEventJSON(ctx, tx, "project.opened", p.ID, "", "", map[string]any{
			"id": p.ID, "name": p.Name, "path": p.Path, "sourceKind": p.SourceKind,
		})
		return err
	})
}

func (s *Store) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT id, name, path, source_kind, repo_identity, git_remote, default_branch, status, knowledge_rev, created_at, updated_at, last_opened_at FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

func (s *Store) GetProjectByPath(ctx context.Context, path string) (*domain.Project, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT id, name, path, source_kind, repo_identity, git_remote, default_branch, status, knowledge_rev, created_at, updated_at, last_opened_at FROM projects WHERE path = ?`, path)
	return scanProject(row)
}

func (s *Store) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, path, source_kind, repo_identity, git_remote, default_branch, status, knowledge_rev, created_at, updated_at, last_opened_at FROM projects ORDER BY last_opened_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		p, err := scanProjectRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProjectPath(ctx context.Context, id, path, repoIdentity string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE projects SET path = ?, repo_identity = ?, status = ?, updated_at = ? WHERE id = ?`,
			path, repoIdentity, string(domain.ProjectAvailable), nowRFC3339(), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrNotFound
		}
		_, err = InsertEventJSON(ctx, tx, "project.relocated", id, "", "", map[string]any{"id": id, "path": path})
		return err
	})
}

func (s *Store) MarkProjectUnavailable(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE projects SET status = ?, updated_at = ? WHERE id = ?`, string(domain.ProjectUnavailable), nowRFC3339(), id)
	return err
}

func (s *Store) RemoveProject(ctx context.Context, id string) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "project.removed", id, "", "", map[string]any{"id": id})
		return err
	})
}

func (s *Store) TouchProject(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE projects SET last_opened_at = ?, updated_at = ? WHERE id = ?`, nowRFC3339(), nowRFC3339(), id)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProject(row scanner) (*domain.Project, error) {
	var p domain.Project
	var status, created, updated, opened string
	err := row.Scan(&p.ID, &p.Name, &p.Path, &p.SourceKind, &p.RepoIdentity, &p.GitRemote, &p.DefaultBranch, &status, &p.KnowledgeRev, &created, &updated, &opened)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Status = domain.ProjectStatus(status)
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	p.LastOpenedAt = parseTime(opened)
	return &p, nil
}

func scanProjectRows(rows *sql.Rows) (*domain.Project, error) {
	return scanProject(rows)
}
