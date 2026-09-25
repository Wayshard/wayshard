package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
)

// TestUpgradeFromPriorVersions proves a database at schema 1, 2 or 3 upgrades
// cleanly to the current schema, including the process ownership and checkpoint
// material lifecycle additions.
func TestUpgradeFromPriorVersions(t *testing.T) {
	ctx := context.Background()
	scripts := map[int]string{1: migration001, 2: migration002, 3: migration003, 4: migration004}
	for _, target := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("v%d", target), func(t *testing.T) {
			dir := t.TempDir()
			raw, err := openRawDB(ctx, filepath.Join(dir, "app.db"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := raw.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
				t.Fatal(err)
			}
			for v := 1; v <= target; v++ {
				if err := execScript(ctx, raw, scripts[v]); err != nil {
					t.Fatalf("apply migration %d: %v", v, err)
				}
				if _, err := raw.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, v, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
					t.Fatal(err)
				}
			}
			_ = raw.Close()

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
				t.Fatalf("schema version after upgrade = %d, want %d", v, schemaVersion)
			}
			if _, err := s.DB.ExecContext(ctx, `SELECT material_state FROM workspace_checkpoints LIMIT 1`); err != nil {
				t.Fatalf("material_state column missing: %v", err)
			}
			// Migration 008 dropped the containment-era ownership tables.
			if _, err := s.DB.ExecContext(ctx, `SELECT token_hash FROM process_owners LIMIT 1`); err == nil {
				t.Fatal("process_owners table should have been dropped")
			}
			if _, err := s.DB.ExecContext(ctx, `SELECT kind FROM probe_owners LIMIT 1`); err == nil {
				t.Fatal("probe_owners table should have been dropped")
			}
		})
	}
}

func setupCheckpointRun(t *testing.T, s *Store, runStatus domain.RunStatus, attemptStatus domain.StageAttemptStatus) (runID string, cp *domain.WorkspaceCheckpoint) {
	t.Helper()
	ctx := context.Background()
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	_ = s.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "x"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "x"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := s.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRunStatus(ctx, run.ID, runStatus, "", ""); err != nil {
		t.Fatal(err)
	}
	runPath := t.TempDir()
	rec := &domain.WorkspaceRecord{RunID: run.ID, ProjectID: p.ID, Kind: "filesystem", SourcePath: runPath, RunPath: runPath}
	if err := s.InsertWorkspace(ctx, rec); err != nil {
		t.Fatal(err)
	}
	stg := &domain.Stage{RunID: run.ID, Kind: domain.StageExecute, Ordinal: 1, Status: domain.AttemptSucceeded}
	if err := s.AppendStage(ctx, stg); err != nil {
		t.Fatal(err)
	}
	att := &domain.StageAttempt{StageID: stg.ID, RunID: run.ID, Ordinal: 1, Status: attemptStatus}
	if err := s.AppendAttempt(ctx, att); err != nil {
		t.Fatal(err)
	}
	treeDir := filepath.Join(s.Root, "runtime", "workspaces", run.ID, "checkpoints", stg.ID, "1")
	if err := os.MkdirAll(filepath.Join(treeDir, "tree"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(treeDir, "tree", "tracked.txt"), []byte("baseline"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp = &domain.WorkspaceCheckpoint{
		WorkspaceID: rec.ID, RunID: run.ID, StageID: stg.ID, AttemptID: att.ID,
		Name: "pre-attempt", TreeHash: "deadbeef", HashVersion: 3, TreePath: treeDir,
	}
	if err := s.InsertCheckpoint(ctx, cp); err != nil {
		t.Fatal(err)
	}
	return run.ID, cp
}

// TestReclaimPinsNonTerminalRunCheckpoints proves active and interrupted
// recoverable checkpoints survive reclamation.
func TestReclaimPinsNonTerminalRunCheckpoints(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		run     domain.RunStatus
		attempt domain.StageAttemptStatus
	}{
		{"running", domain.RunExecuting, domain.AttemptRunning},
		{"interrupted", domain.RunExecuting, domain.AttemptInterrupted},
		{"blocked", domain.RunBlocked, domain.AttemptInterrupted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Open(ctx, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			runID, cp := setupCheckpointRun(t, s, tc.run, tc.attempt)
			n, err := s.ReclaimCheckpoints(ctx, time.Nanosecond)
			if err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Fatalf("reclaimed %d checkpoints for non-terminal run %s", n, runID)
			}
			got, err := s.GetCheckpoint(ctx, cp.ID)
			if err != nil || got.MaterialState != domain.CheckpointPresent {
				t.Fatalf("checkpoint material state changed: %+v err=%v", got, err)
			}
			if _, err := os.Stat(filepath.Join(cp.TreePath, "tree", "tracked.txt")); err != nil {
				t.Fatalf("checkpoint tree removed: %v", err)
			}
		})
	}
}

// TestReclaimTerminalCheckpoint proves a terminal run's checkpoint becomes
// reclaimable and its metadata is marked reclaimed.
func TestReclaimTerminalCheckpoint(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, cp := setupCheckpointRun(t, s, domain.RunComplete, domain.AttemptSucceeded)

	if n, _ := s.ReclaimCheckpoints(ctx, time.Hour); n != 0 {
		t.Fatalf("recent terminal checkpoint reclaimed early: %d", n)
	}
	// Backdate the checkpoint so the retention decision is deterministic and
	// independent of platform clock resolution (Windows time.Now can be coarse
	// enough that a freshly created row shares a tick with "now").
	if _, err := s.DB.ExecContext(ctx, `UPDATE workspace_checkpoints SET created_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339Nano), cp.ID); err != nil {
		t.Fatal(err)
	}
	n, err := s.ReclaimCheckpoints(ctx, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 reclaimed checkpoint, got %d", n)
	}
	got, err := s.GetCheckpoint(ctx, cp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MaterialState != domain.CheckpointReclaimed {
		t.Fatalf("material state = %q, want reclaimed", got.MaterialState)
	}
	if _, err := os.Stat(cp.TreePath); !os.IsNotExist(err) {
		t.Fatalf("checkpoint tree still present: %v", err)
	}
	// Metadata remains readable for history.
	if got.TreeHash != "deadbeef" || got.StageID == "" {
		t.Fatalf("metadata lost on reclaim: %+v", got)
	}
}

// TestCleanupCheckpointDebris proves unreferenced checkpoint dirs, staging
// leftovers and terminal sandbox dirs are removed while referenced material is
// kept.
func TestCleanupCheckpointDebris(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	runID, cp := setupCheckpointRun(t, s, domain.RunComplete, domain.AttemptSucceeded)

	orphan := filepath.Join(filepath.Dir(cp.TreePath), "orphan-dir")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(s.Root, "runtime", "restore-staging", "leftover")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	sandbox := filepath.Join(s.Root, "runtime", "sandbox", runID, "attempt-1")
	if err := os.MkdirAll(sandbox, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := s.CleanupCheckpointDebris(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("unreferenced checkpoint dir survived: %v", err)
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("restore staging leftover survived: %v", err)
	}
	if _, err := os.Stat(sandbox); !os.IsNotExist(err) {
		t.Fatalf("terminal sandbox dir survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cp.TreePath, "tree", "tracked.txt")); err != nil {
		t.Fatalf("referenced checkpoint material removed: %v", err)
	}
}
