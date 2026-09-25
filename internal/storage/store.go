// Package storage is the sole SQLite writer plus the SHA-256 object store.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	_ "modernc.org/sqlite"
)

const (
	busyTimeoutMS = 5000
	schemaVersion = 8
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrLocked = errors.New("storage locked")
var ErrCorrupt = errors.New("storage corrupt")

// Store is the control-plane persistence handle.
type Store struct {
	DB      *sql.DB
	Root    string
	Objects *ObjectStore
	// EventHook receives durable domain events after their transaction commits.
	// It is a live-delivery projection; the events table remains the source for replay.
	EventHook func(domain.Event)
}

func Open(ctx context.Context, root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(root, "app.db")
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=wal_autocheckpoint(1000)", dbPath, busyTimeoutMS)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // sole writer
	db.SetConnMaxLifetime(0)
	if err := ping(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	objs, err := NewObjectStore(filepath.Join(root, "objects"))
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{DB: db, Root: root, Objects: objs}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func ping(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

func (s *Store) Close() error {
	if s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	var current int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current)
	if current > schemaVersion {
		return fmt.Errorf("%w: database schema %d is newer than server %d; refusing unsafe downgrade", ErrCorrupt, current, schemaVersion)
	}
	if current < 1 {
		if err := execScript(ctx, s.DB, migration001); err != nil {
			return fmt.Errorf("apply migration 001: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (1, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		current = 1
	}
	if current < 2 {
		if err := execScript(ctx, s.DB, migration002); err != nil {
			return fmt.Errorf("apply migration 002: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (2, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		current = 2
	}
	if current < 3 {
		if err := execScript(ctx, s.DB, migration003); err != nil {
			return fmt.Errorf("apply migration 003: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (3, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		current = 3
	}
	if current < 4 {
		if err := execScript(ctx, s.DB, migration004); err != nil {
			return fmt.Errorf("apply migration 004: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (4, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		current = 4
	}
	if current < 5 {
		if err := execScript(ctx, s.DB, migration005); err != nil {
			return fmt.Errorf("apply migration 005: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (5, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		current = 5
	}
	if current < 6 {
		if err := execScript(ctx, s.DB, migration006); err != nil {
			return fmt.Errorf("apply migration 006: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (6, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		current = 6
	}
	if current < 7 {
		if err := execScript(ctx, s.DB, migration007); err != nil {
			return fmt.Errorf("apply migration 007: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (7, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		current = 7
	}
	if current < 8 {
		if err := execScript(ctx, s.DB, migration008); err != nil {
			return fmt.Errorf("apply migration 008: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (8, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return nil
}

func execScript(ctx context.Context, db *sql.DB, script string) error {
	for _, stmt := range splitSQL(script) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			preview := stmt
			if len(preview) > 120 {
				preview = preview[:120]
			}
			return fmt.Errorf("%w in %q", err, preview)
		}
	}
	return nil
}

func splitSQL(script string) []string {
	var out []string
	var b []rune
	inSQuote := false
	for _, r := range script {
		switch {
		case r == '\'' && !inSQuote:
			inSQuote = true
			b = append(b, r)
		case r == '\'' && inSQuote:
			inSQuote = false
			b = append(b, r)
		case r == ';' && !inSQuote:
			s := strings.TrimSpace(string(b))
			if s != "" {
				out = append(out, s)
			}
			b = b[:0]
		default:
			b = append(b, r)
		}
	}
	if s := strings.TrimSpace(string(b)); s != "" {
		out = append(out, s)
	}
	return out
}

func (s *Store) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	rec := &eventRecorder{}
	txRecorders.Store(tx, rec)
	defer txRecorders.Delete(tx)
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Publish only after a successful commit so live subscribers never observe
	// an event whose domain mutation was rolled back.
	s.dispatch(rec.events)
	return nil
}

func (s *Store) IntegrityCheck(ctx context.Context) error {
	var result string
	if err := s.DB.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("%w: %s", ErrCorrupt, result)
	}
	return nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t
}

func nullTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func scanNullTime(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	t := parseTime(v.String)
	return &t
}
