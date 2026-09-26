package storage

import _ "embed"

//go:embed migrations/001_init.sql
var migration001 string
