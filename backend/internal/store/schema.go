package store

import _ "embed"

// Schema is applied by the API before it starts serving requests.
//
//go:embed migrations/001_initial.sql
var Schema string
