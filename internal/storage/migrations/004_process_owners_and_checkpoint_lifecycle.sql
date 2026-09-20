-- Process-tree ownership for startup orphan reconciliation after a crash.
CREATE TABLE IF NOT EXISTS process_owners (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL DEFAULT '',
  stage_id TEXT NOT NULL DEFAULT '',
  attempt_id TEXT NOT NULL DEFAULT '',
  token_hash TEXT NOT NULL DEFAULT '',
  pgid INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL DEFAULT '',
  reconciled_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_process_owners_state ON process_owners(state);
CREATE INDEX IF NOT EXISTS idx_process_owners_attempt ON process_owners(attempt_id);

-- Checkpoint material lifecycle. A row may outlive its materialized tree once
-- retention reclaims it, and recovery must never treat a reclaimed checkpoint
-- as restorable.
ALTER TABLE workspace_checkpoints ADD COLUMN material_state TEXT NOT NULL DEFAULT 'present';
