package storage

import _ "embed"

//go:embed migrations/001_init.sql
var migration001 string

//go:embed migrations/002_checkpoints.sql
var migration002 string

//go:embed migrations/003_checkpoint_hash_version.sql
var migration003 string

//go:embed migrations/004_process_owners_and_checkpoint_lifecycle.sql
var migration004 string

//go:embed migrations/005_probe_owners.sql
var migration005 string

//go:embed migrations/006_harness_catalog.sql
var migration006 string

//go:embed migrations/007_harness_fingerprint.sql
var migration007 string
