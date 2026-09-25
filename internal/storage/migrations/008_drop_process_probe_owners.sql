-- The containment/provider architecture was removed: discovered harnesses run
-- as the Wayshard server OS user, so these columns have no meaning.
-- Migration 008 drops the containment-era ownership tables and the harness
-- isolation/provider metadata columns from the control-plane tables.

DROP TABLE IF EXISTS probe_owners;
DROP TABLE IF EXISTS process_owners;

ALTER TABLE harness_installations DROP COLUMN isolation;
ALTER TABLE harness_installations DROP COLUMN provider_transport;
ALTER TABLE harness_installations DROP COLUMN requires_provider_network;

ALTER TABLE route_decisions DROP COLUMN isolation;
