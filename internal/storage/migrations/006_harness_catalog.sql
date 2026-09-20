ALTER TABLE harness_installations ADD COLUMN definition_source TEXT NOT NULL DEFAULT '';
ALTER TABLE harness_installations ADD COLUMN bridge_executable TEXT NOT NULL DEFAULT '';
ALTER TABLE harness_installations ADD COLUMN bridge_present INTEGER NOT NULL DEFAULT 0;
ALTER TABLE harness_installations ADD COLUMN acp_status TEXT NOT NULL DEFAULT '';
ALTER TABLE harness_installations ADD COLUMN blocking_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE harness_installations ADD COLUMN provider_transport TEXT NOT NULL DEFAULT '';
ALTER TABLE harness_installations ADD COLUMN model_selection TEXT NOT NULL DEFAULT '';
ALTER TABLE harness_installations ADD COLUMN requires_provider_network INTEGER NOT NULL DEFAULT 0;