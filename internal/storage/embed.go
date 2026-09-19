package storage

import _ "embed"

//go:embed migrations/001_init.sql
var migration001 string

//go:embed migrations/002_checkpoints.sql
var migration002 string
