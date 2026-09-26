// Package storage is the sole SQLite writer plus the SHA-256 object store.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	_ "modernc.org/sqlite"
)

const (
	busyTimeoutMS = 5000
	// schemaVersion is the v0.2 baseline. Post-v0.2 schema changes add numbered
	// migrations alongside 001_init.sql and raise this value.
	schemaVersion = 1
	// schemaBaseline marks a database created by the v0.2 baseline. It lets a
	// pre-v0.2 database be rejected clearly instead of being mistaken for current.
	schemaBaseline = "wayshard-v0.2"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrLocked = errors.New("storage locked")
var ErrCorrupt = errors.New("storage corrupt")
var ErrIncompatibleSchema = errors.New("incompatible database schema")

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
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
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

// sqliteDSN builds a file: DSN whose path is URI-escaped, so a data directory
// containing URI-significant characters (for example '#' or '?') still opens
// its own database.
func sqliteDSN(dbPath string) string {
	q := url.Values{}
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeoutMS))
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", "wal_autocheckpoint(1000)")
	u := url.URL{Scheme: "file", Path: dbPath, RawQuery: q.Encode()}
	return u.String()
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

	// A database created before the v0.2 baseline has migration history but no
	// v0.2 baseline marker. v0.2 does not upgrade pre-v0.2 databases.
	if current > 0 {
		if baseline, ok := s.schemaBaseline(ctx); !ok || baseline != schemaBaseline {
			return fmt.Errorf("%w: this database predates the v0.2 baseline and cannot be upgraded; remove the Wayshard data directory and start fresh", ErrIncompatibleSchema)
		}
	}
	if current > schemaVersion {
		return fmt.Errorf("%w: database schema %d is newer than server %d; refusing unsafe downgrade", ErrCorrupt, current, schemaVersion)
	}

	// Baseline: fresh installations apply 001_init.sql and record version 1.
	// Post-v0.2 changes add numbered migrations here and raise schemaVersion.
	if current < 1 {
		if err := execScript(ctx, s.DB, migration001); err != nil {
			return fmt.Errorf("apply schema baseline 001: %w", err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (1, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return nil
}

// schemaBaseline reads the v0.2 baseline marker from the operational metadata
// table. It reports false when the table or marker is absent.
func (s *Store) schemaBaseline(ctx context.Context) (string, bool) {
	var v string
	if err := s.DB.QueryRowContext(ctx, `SELECT value FROM operational WHERE key = 'schema_baseline'`).Scan(&v); err != nil {
		return "", false
	}
	return v, true
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
