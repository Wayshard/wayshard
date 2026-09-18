package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/knowledge"
)

// ReplaceProjectKnowledge atomically replaces the SQLite knowledge index for a
// project. Repository files are not modified.
func (s *Store) ReplaceProjectKnowledge(ctx context.Context, projectID string, idx *knowledge.Index) error {
	if idx == nil {
		idx = &knowledge.Index{}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_edges WHERE project_id = ?`, projectID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_documents WHERE project_id = ?`, projectID); err != nil {
			return err
		}
		for i := range idx.Documents {
			d := &idx.Documents[i]
			if d.ID == "" {
				d.ID = id.New()
			}
			d.ProjectID = projectID
			if d.Source == "" {
				d.Source = knowledge.SourceDiscovered
			}
			if d.Status == "" {
				d.Status = knowledge.StatusOK
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_documents(
				id, project_id, path, kind, family, scope, authority_domain, source, hash, status, content, created_at, updated_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				d.ID, projectID, d.Path, string(d.Kind), string(d.Family), d.Scope, string(d.AuthorityDomain),
				d.Source, d.Hash, d.Status, d.Content, now, now); err != nil {
				return err
			}
		}
		for i := range idx.Sections {
			sec := &idx.Sections[i]
			if sec.ID == "" {
				sec.ID = id.New()
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_sections(id, document_id, heading, ordinal, hash, body)
				VALUES (?,?,?,?,?,?)`, sec.ID, sec.DocumentID, sec.Heading, sec.Ordinal, sec.Hash, sec.Body); err != nil {
				return err
			}
		}
		for i := range idx.Edges {
			e := &idx.Edges[i]
			if e.ID == "" {
				e.ID = id.New()
			}
			broken := 0
			if e.Broken {
				broken = 1
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_edges(id, project_id, from_document_id, to_path, kind, broken)
				VALUES (?,?,?,?,?,?)`, e.ID, projectID, e.FromDocumentID, e.ToPath, e.Kind, broken); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET knowledge_rev = ?, updated_at = ? WHERE id = ?`, idx.Revision, now, projectID); err != nil {
			return err
		}
		_, err := InsertEventJSON(ctx, tx, "knowledge.indexed", projectID, "", "", map[string]any{
			"projectId": projectID,
			"revision":  idx.Revision,
			"documents": len(idx.Documents),
			"edges":     len(idx.Edges),
			"broken":    idx.Stats.BrokenRefs,
			"cycles":    idx.Stats.Cycles,
			"conflicts": len(idx.Conflicts),
		})
		return err
	})
}

func (s *Store) ListKnowledgeDocuments(ctx context.Context, projectID string) ([]knowledge.Document, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, project_id, path, kind, family, scope, authority_domain, source, hash, status, content, created_at, updated_at
		FROM knowledge_documents WHERE project_id = ? ORDER BY path`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []knowledge.Document
	for rows.Next() {
		d, err := scanKnowledgeDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListKnowledgeSections(ctx context.Context, documentID string) ([]knowledge.Section, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, document_id, heading, ordinal, hash, body FROM knowledge_sections WHERE document_id = ? ORDER BY ordinal`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []knowledge.Section
	for rows.Next() {
		var sec knowledge.Section
		if err := rows.Scan(&sec.ID, &sec.DocumentID, &sec.Heading, &sec.Ordinal, &sec.Hash, &sec.Body); err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	return out, rows.Err()
}

func (s *Store) ListKnowledgeEdges(ctx context.Context, projectID string) ([]knowledge.Edge, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, from_document_id, to_path, kind, broken FROM knowledge_edges WHERE project_id = ? ORDER BY to_path`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []knowledge.Edge
	for rows.Next() {
		var e knowledge.Edge
		var broken int
		if err := rows.Scan(&e.ID, &e.FromDocumentID, &e.ToPath, &e.Kind, &broken); err != nil {
			return nil, err
		}
		e.Broken = broken != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// LoadKnowledgeIndex reconstructs an in-memory index from SQLite. Globs and
// always-apply flags are rehydrated from stored content.
func (s *Store) LoadKnowledgeIndex(ctx context.Context, projectID string) (*knowledge.Index, error) {
	docs, err := s.ListKnowledgeDocuments(ctx, projectID)
	if err != nil {
		return nil, err
	}
	idx := &knowledge.Index{Documents: docs, Stats: knowledge.Stats{DocsIndexed: len(docs)}}
	for i := range idx.Documents {
		d := &idx.Documents[i]
		knowledge.HydrateRuntime(d)
		secs, err := s.ListKnowledgeSections(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		idx.Sections = append(idx.Sections, secs...)
	}
	edges, err := s.ListKnowledgeEdges(ctx, projectID)
	if err != nil {
		return nil, err
	}
	idToPath := map[string]string{}
	for _, d := range idx.Documents {
		idToPath[d.ID] = d.Path
	}
	for i := range edges {
		edges[i].FromPath = idToPath[edges[i].FromDocumentID]
		if edges[i].Broken {
			idx.Stats.BrokenRefs++
		}
	}
	idx.Edges = edges
	var rev string
	_ = s.DB.QueryRowContext(ctx, `SELECT knowledge_rev FROM projects WHERE id = ?`, projectID).Scan(&rev)
	idx.Revision = rev
	idx.Conflicts = knowledge.DetectConflicts(idx.Documents)
	return idx, nil
}

func (s *Store) UpdateProjectKnowledgeRev(ctx context.Context, projectID, rev string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE projects SET knowledge_rev = ?, updated_at = ? WHERE id = ?`,
		rev, nowRFC3339(), projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanKnowledgeDocument(row scanner) (knowledge.Document, error) {
	var d knowledge.Document
	var kind, family, domain, created, updated string
	err := row.Scan(&d.ID, &d.ProjectID, &d.Path, &kind, &family, &d.Scope, &domain, &d.Source, &d.Hash, &d.Status, &d.Content, &created, &updated)
	if err != nil {
		return d, err
	}
	d.Kind = knowledge.Kind(kind)
	d.Family = knowledge.Family(family)
	d.AuthorityDomain = knowledge.AuthorityDomain(domain)
	d.Explicit = d.Source == knowledge.SourceExplicit
	d.Canonical = !containsSlash(d.Path) && knowledge.IsCanonicalRootName(d.Path)
	knowledge.HydrateRuntime(&d)
	return d, nil
}

func containsSlash(p string) bool {
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			return true
		}
	}
	return false
}
