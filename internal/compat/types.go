package compat

import (
	"context"
	"database/sql"

	"github.com/pablontiv/backscroll/internal/models"
)

type Code string

const (
	CodeUnsupportedLineage Code = "unsupported_lineage"
	CodeMigrationFailed    Code = "migration_failed"
	CodeIndexStale         Code = "index_stale"
	CodeRecoveryConflict   Code = "recovery_conflict"
	CodeUninterpretableRow Code = "uninterpretable_row"
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

type RecoveryInput struct {
	Shape    SchemaShape
	Records  []models.IndexedRecord
	RowCount int
}

type CanonicalRecord struct {
	Record      models.IndexedRecord
	PayloadHash string
}

type RecoveryPlan struct {
	InputShapes     []SchemaShape
	Records         []CanonicalRecord
	ExactDuplicates int
}

type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
