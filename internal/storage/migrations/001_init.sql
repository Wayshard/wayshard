-- Wayshard control-plane schema v1.
-- The Go server is the sole writer. WAL + foreign keys are applied at open.

CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS server_identity (
  server_id TEXT PRIMARY KEY,
  public_key BLOB NOT NULL,
  display_name TEXT NOT NULL DEFAULT 'Wayshard',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  verifier BLOB NOT NULL,
  salt BLOB NOT NULL,
  pairing_id TEXT,
  created_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  revoked_at TEXT
);

CREATE TABLE IF NOT EXISTS pairing_invitations (
  id TEXT PRIMARY KEY,
  code_hash BLOB NOT NULL,
  advertised_url TEXT NOT NULL,
  listen_url TEXT NOT NULL,
  server_fingerprint TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at TEXT,
  created_by TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS auth_sessions (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL REFERENCES devices(id),
  token_hash BLOB NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  repo_identity TEXT NOT NULL DEFAULT '',
  git_remote TEXT NOT NULL DEFAULT '',
  default_branch TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  knowledge_rev TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_opened_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_projects_path ON projects(path);
CREATE INDEX IF NOT EXISTS idx_projects_repo ON projects(repo_identity);

CREATE TABLE IF NOT EXISTS knowledge_documents (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id),
  path TEXT NOT NULL,
  kind TEXT NOT NULL,
  family TEXT NOT NULL,
  scope TEXT NOT NULL DEFAULT '',
  authority_domain TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'discovered',
  hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ok',
  content TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(project_id, path)
);

CREATE TABLE IF NOT EXISTS knowledge_sections (
  id TEXT PRIMARY KEY,
  document_id TEXT NOT NULL REFERENCES knowledge_documents(id) ON DELETE CASCADE,
  heading TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  hash TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS knowledge_edges (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id),
  from_document_id TEXT NOT NULL,
  to_path TEXT NOT NULL,
  kind TEXT NOT NULL,
  broken INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS conversations (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id),
  title TEXT NOT NULL DEFAULT '',
  pinned INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES conversations(id),
  role TEXT NOT NULL,
  body TEXT NOT NULL,
  task_id TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id, created_at);

CREATE TABLE IF NOT EXISTS conversation_summaries (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES conversations(id),
  body TEXT NOT NULL,
  through_message_id TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id),
  conversation_id TEXT NOT NULL REFERENCES conversations(id),
  message_id TEXT NOT NULL,
  objective TEXT NOT NULL,
  artifact_only INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES tasks(id),
  project_id TEXT NOT NULL REFERENCES projects(id),
  conversation_id TEXT NOT NULL REFERENCES conversations(id),
  status TEXT NOT NULL,
  blocked_reason TEXT NOT NULL DEFAULT '',
  blocked_detail TEXT NOT NULL DEFAULT '',
  profile TEXT NOT NULL DEFAULT 'auto',
  degraded_routing INTEGER NOT NULL DEFAULT 0,
  idempotency_key TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_runs_project ON runs(project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);

CREATE TABLE IF NOT EXISTS stages (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  kind TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(run_id, ordinal)
);

CREATE TABLE IF NOT EXISTS stage_attempts (
  id TEXT PRIMARY KEY,
  stage_id TEXT NOT NULL REFERENCES stages(id),
  run_id TEXT NOT NULL REFERENCES runs(id),
  ordinal INTEGER NOT NULL,
  status TEXT NOT NULL,
  harness_id TEXT NOT NULL DEFAULT '',
  model_id TEXT NOT NULL DEFAULT '',
  failure_class TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  checkpoint_id TEXT NOT NULL DEFAULT '',
  started_at TEXT,
  finished_at TEXT,
  created_at TEXT NOT NULL,
  UNIQUE(stage_id, ordinal)
);

CREATE TABLE IF NOT EXISTS assessments (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  jev_model TEXT NOT NULL,
  question_set TEXT NOT NULL,
  policy_version TEXT NOT NULL,
  dimensions_json TEXT NOT NULL,
  input_hash TEXT NOT NULL,
  usage_json TEXT NOT NULL DEFAULT '{}',
  degraded INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS route_decisions (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  stage_id TEXT NOT NULL,
  assessment_id TEXT NOT NULL DEFAULT '',
  harness_id TEXT NOT NULL,
  model_id TEXT NOT NULL DEFAULT '',
  effort TEXT NOT NULL DEFAULT '',
  profile TEXT NOT NULL,
  isolation TEXT NOT NULL,
  fallbacks_json TEXT NOT NULL DEFAULT '[]',
  policy_version TEXT NOT NULL,
  reason TEXT NOT NULL,
  degraded INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifacts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  stage_id TEXT NOT NULL DEFAULT '',
  attempt_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  schema_ver INTEGER NOT NULL DEFAULT 1,
  object_hash TEXT NOT NULL DEFAULT '',
  json TEXT NOT NULL,
  valid INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS attachments (
  id TEXT PRIMARY KEY,
  run_id TEXT,
  project_id TEXT,
  object_hash TEXT NOT NULL,
  filename TEXT NOT NULL,
  mime TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  project_id TEXT NOT NULL REFERENCES projects(id),
  kind TEXT NOT NULL,
  source_path TEXT NOT NULL,
  run_path TEXT NOT NULL,
  snapshot_id TEXT NOT NULL DEFAULT '',
  branch TEXT NOT NULL DEFAULT '',
  head TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS workspace_snapshots (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  branch TEXT NOT NULL DEFAULT '',
  head TEXT NOT NULL DEFAULT '',
  dirty_json TEXT NOT NULL DEFAULT '{}',
  knowledge_rev TEXT NOT NULL DEFAULT '',
  object_hash TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS workspace_checkpoints (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  name TEXT NOT NULL,
  object_hash TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS run_deltas (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  summary_json TEXT NOT NULL,
  object_hash TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS integrations (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  project_id TEXT NOT NULL REFERENCES projects(id),
  status TEXT NOT NULL,
  base_snapshot TEXT NOT NULL,
  target_branch TEXT NOT NULL DEFAULT '',
  current_branch TEXT NOT NULL DEFAULT '',
  journal_hash TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS publication_journal (
  id TEXT PRIMARY KEY,
  integration_id TEXT NOT NULL REFERENCES integrations(id),
  path TEXT NOT NULL,
  before_hash TEXT NOT NULL,
  after_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS approvals (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  stage_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  resource TEXT NOT NULL,
  reason TEXT NOT NULL,
  scopes_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL,
  resolved_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  resolved_at TEXT
);

CREATE TABLE IF NOT EXISTS notifications (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  project_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  attention INTEGER NOT NULL DEFAULT 1,
  read_at TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS model_usage (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  stage_id TEXT NOT NULL DEFAULT '',
  attempt_id TEXT NOT NULL DEFAULT '',
  harness_id TEXT NOT NULL DEFAULT '',
  model_id TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  cache_tokens INTEGER NOT NULL DEFAULT 0,
  cost_micros INTEGER NOT NULL DEFAULT 0,
  estimated INTEGER NOT NULL DEFAULT 0,
  pricing_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,
  project_id TEXT NOT NULL DEFAULT '',
  conversation_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  payload TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_project ON events(project_id, seq);
CREATE INDEX IF NOT EXISTS idx_events_run ON events(run_id, seq);

CREATE TABLE IF NOT EXISTS idempotency_keys (
  key TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  response TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
  scope TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  key TEXT NOT NULL,
  value_json TEXT NOT NULL,
  revision INTEGER NOT NULL DEFAULT 1,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (scope, scope_id, key)
);

CREATE TABLE IF NOT EXISTS policy_versions (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  body_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS harness_definitions (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  executable_hints TEXT NOT NULL,
  adapter TEXT NOT NULL,
  launch_recipe TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS harness_installations (
  id TEXT PRIMARY KEY,
  definition_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  executable TEXT NOT NULL,
  version TEXT NOT NULL DEFAULT '',
  adapter TEXT NOT NULL,
  health TEXT NOT NULL,
  compatibility TEXT NOT NULL,
  isolation TEXT NOT NULL,
  auth_status TEXT NOT NULL DEFAULT 'unknown',
  capabilities_json TEXT NOT NULL DEFAULT '{}',
  models_json TEXT NOT NULL DEFAULT '[]',
  last_probed_at TEXT NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  UNIQUE(executable)
);

CREATE TABLE IF NOT EXISTS pty_sessions (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  device_id TEXT NOT NULL,
  cwd TEXT NOT NULL,
  alive INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  last_attached_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS objects (
  hash TEXT PRIMARY KEY,
  size INTEGER NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS operational (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
