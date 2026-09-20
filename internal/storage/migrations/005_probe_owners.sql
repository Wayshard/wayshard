CREATE TABLE IF NOT EXISTS probe_owners (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    pgid INTEGER NOT NULL DEFAULT 0,
    state TEXT NOT NULL,
    created_at TEXT NOT NULL,
    reconciled_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_probe_owners_state ON probe_owners(state);