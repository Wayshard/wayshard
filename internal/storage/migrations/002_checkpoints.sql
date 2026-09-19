ALTER TABLE workspace_checkpoints ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workspace_checkpoints ADD COLUMN stage_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workspace_checkpoints ADD COLUMN attempt_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workspace_checkpoints ADD COLUMN tree_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE workspace_checkpoints ADD COLUMN tree_path TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_checkpoints_run ON workspace_checkpoints(run_id);
CREATE INDEX IF NOT EXISTS idx_checkpoints_stage ON workspace_checkpoints(stage_id);
