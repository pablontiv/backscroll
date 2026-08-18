package compat

import (
	"context"
	"database/sql"
)

type Code string

const (
	CodeUnsupportedLineage Code = "unsupported_lineage"
	CodeMigrationFailed    Code = "migration_failed"
	CodeIndexStale         Code = "index_stale"
)

type Diagnostic struct {
	Code         Code
	Summary      string
	Continuation []string
}

type SchemaShape struct {
	AppliedVersion int
	Signature      string
}

type MigrationStep struct {
	Version int
	Name    string
}

type MigrationPlan struct {
	From  SchemaShape
	Steps []MigrationStep
}

type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
