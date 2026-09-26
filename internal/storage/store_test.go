package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

// TestFreshInstallCurrentSchema proves a fresh installation applies exactly the
// v0.2 baseline (schema version 1), records the baseline marker, exposes the
// current tables and columns, and contains no obsolete containment structures.
func TestFreshInstallCurrentSchema(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var v int
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want the v0.2 baseline of 1", schemaVersion)
	}
	if v != schemaVersion {
		t.Fatalf("schema version = %d, want 1", v)
	}
	if b, ok := s.schemaBaseline(ctx); !ok || b != schemaBaseline {
		t.Fatalf("baseline marker = %q present=%v, want %q", b, ok, schemaBaseline)
	}

	for _, table := range []string{
		"runs", "stages", "stage_attempts", "workspace_checkpoints", "workspace_snapshots",
		"harness_definitions", "harness_installations", "integrations", "publication_journal",
		"approvals", "route_decisions", "run_deltas", "events",
	} {
		if _, err := s.DB.ExecContext(ctx, `SELECT 1 FROM `+table+` LIMIT 1`); err != nil {
			t.Fatalf("current table %s missing: %v", table, err)
		}
	}
	for _, table := range []string{"process_owners", "probe_owners"} {
		if _, err := s.DB.ExecContext(ctx, `SELECT 1 FROM `+table+` LIMIT 1`); err == nil {
			t.Fatalf("obsolete containment table %s is present", table)
		}
	}

	if _, err := s.DB.ExecContext(ctx, `SELECT run_id, stage_id, attempt_id, tree_hash, tree_path, hash_version, material_state FROM workspace_checkpoints LIMIT 1`); err != nil {
		t.Fatalf("current checkpoint columns missing: %v", err)
	}
	if _, err := s.DB.ExecContext(ctx, `SELECT definition_source, bridge_executable, bridge_present, acp_status, blocking_reason, model_selection, definition_fingerprint FROM harness_installations LIMIT 1`); err != nil {
		t.Fatalf("current harness columns missing: %v", err)
	}
	for _, q := range []string{
		`SELECT isolation FROM route_decisions LIMIT 1`,
		`SELECT isolation FROM harness_installations LIMIT 1`,
		`SELECT provider_transport FROM harness_installations LIMIT 1`,
		`SELECT requires_provider_network FROM harness_installations LIMIT 1`,
	} {
		if _, err := s.DB.ExecContext(ctx, q); err == nil {
			t.Fatalf("obsolete column still present: %s", q)
		}
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

// TestOpenPathWithSpecialCharacters proves the DSN escapes the characters that
// would otherwise break it (for example '#' and '%'), so a data directory with
// such characters opens its own database. '?' is exercised off Windows only,
// because it is not a legal Windows path character.
func TestOpenPathWithSpecialCharacters(t *testing.T) {
	ctx := context.Background()
	name := "a#b%c d"
	if runtime.GOOS != "windows" {
		name = "a#b?c% d"
	}
	root := filepath.Join(t.TempDir(), name)
	s, err := Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := os.Stat(filepath.Join(root, "app.db")); err != nil {
		t.Fatalf("app.db not created at the requested path: %v", err)
	}
	p := &domain.Project{Name: "x", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
}

// TestReopenUsesBaseline proves reopening a v0.2 database keeps schema version 1,
// does not reapply the baseline, and leaves existing data readable.
func TestReopenUsesBaseline(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &domain.Project{Name: "keep", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	var v int
	if err := s2.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != schemaVersion {
		t.Fatalf("schema version after reopen = %d, want %d", v, schemaVersion)
	}
	got, err := s2.GetProject(ctx, p.ID)
	if err != nil || got.Name != "keep" {
		t.Fatalf("row unreadable after reopen: %+v err=%v", got, err)
	}
}

// TestRejectPreV02Database proves a database with pre-v0.2 migration history and
// no v0.2 baseline marker is refused with clear guidance instead of being read as
// current.
func TestRejectPreV02Database(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	raw, err := openRawDB(ctx, filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (7, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE operational (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	_, err = Open(ctx, dir)
	if !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

// TestRefuseNewerSchema proves a database whose schema is newer than the running
// server is refused rather than downgraded.
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
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}
