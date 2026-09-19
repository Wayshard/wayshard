package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
)

func openRawDB(ctx context.Context, path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func TestOpenMigrateAndIntegrity(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.IntegrityCheck(ctx); err != nil {
		t.Fatal(err)
	}
	var v int
	if err := s.DB.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("schema version = %d, want %d", v, schemaVersion)
	}
}

// TestUpgradeFromV1 verifies an existing schema-1 database upgrades cleanly to
// the checkpoint schema while preserving existing rows.
func TestUpgradeFromV1(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// Build a schema-1 database by applying only migration 001.
	raw, err := openRawDB(ctx, filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := execScript(ctx, raw, migration001); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (1, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `INSERT INTO projects(id, name, path, source_kind, repo_identity, git_remote, default_branch, status, knowledge_rev, created_at, updated_at, last_opened_at) VALUES ('p1','legacy','/tmp/legacy','filesystem','','','','available','',?,?,?)`, nowRFC3339(), nowRFC3339(), nowRFC3339()); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	// Now open with the server, which should migrate to v2.
	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var v int
	if err := s.DB.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("schema version after upgrade = %d", v)
	}
	got, err := s.GetProject(ctx, "p1")
	if err != nil || got.Name != "legacy" {
		t.Fatalf("legacy row unreadable after upgrade: %+v err=%v", got, err)
	}
}

func TestProjectAndEventTransaction(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := &domain.Project{Name: "demo", Path: "/tmp/demo", SourceKind: "filesystem"}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "demo" {
		t.Fatalf("name = %s", got.Name)
	}
	ev, err := s.EventsSince(ctx, 0, p.ID, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) != 1 || ev[0].Type != "project.opened" {
		t.Fatalf("events = %+v", ev)
	}
}

func TestTaskRunTransactional(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "s1"}
	if err := s.InsertConversation(ctx, c); err != nil {
		t.Fatal(err)
	}
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "add tests"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "add tests"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := s.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.RunQueued {
		t.Fatalf("status = %s", got.Status)
	}
	if err := s.CompareAndSetRunStatus(ctx, run.ID, domain.RunQueued, domain.RunPlanning, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.CompareAndSetRunStatus(ctx, run.ID, domain.RunQueued, domain.RunExecuting, "", ""); err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestObjectStoreContentAddress(t *testing.T) {
	o, err := NewObjectStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	h1, err := o.Put([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	h2, err := o.Put([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("hash mismatch %s %s", h1, h2)
	}
	b, err := o.Get(h1)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Fatalf("got %q", b)
	}
}

func TestDeviceRevokeDoesNotRequireRunCancel(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d := &domain.Device{Name: "cli", Kind: "cli", Verifier: []byte("v"), Salt: []byte("s")}
	if err := s.InsertDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeDevice(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDevice(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RevokedAt == nil {
		t.Fatal("expected revoked_at")
	}
}

func TestRefuseNewerSchema(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (99, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	_, err = Open(ctx, dir)
	if err == nil {
		t.Fatal("expected refuse newer schema")
	}
}
