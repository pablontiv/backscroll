package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/models"
)

// ReadRecoveryInput reads every recoverable search_items row from a supported
// historical index lineage without applying migrations or mutating the database.
func ReadRecoveryInput(ctx context.Context, db *Database) (compat.RecoveryInput, *compat.Diagnostic, error) {
	return ReadRecoveryInputFromQueryer(ctx, db.DB())
}

// ReadRecoveryInputFromQueryer reads recoverable rows through q without opening
// another connection. Recovery apply uses this while holding a SQLite write
// reservation, so the final active input and replacement plan are derived from
// the same reserved snapshot.
func ReadRecoveryInputFromQueryer(ctx context.Context, q compat.Queryer) (compat.RecoveryInput, *compat.Diagnostic, error) {
	plan, diag, err := compat.InspectIndex(ctx, q)
	if err != nil || diag != nil {
		return compat.RecoveryInput{}, diag, err
	}

	records, diag, err := readRecordsForSignature(ctx, q, plan.From.Signature)
	if err != nil || diag != nil {
		return compat.RecoveryInput{}, diag, err
	}
	return compat.RecoveryInput{Shape: plan.From, Records: records, RowCount: len(records)}, nil, nil
}

func readRecordsForSignature(ctx context.Context, q compat.Queryer, signature string) ([]models.IndexedRecord, *compat.Diagnostic, error) {
	catalog, err := compat.LoadCatalog()
	if err != nil {
		return nil, nil, fmt.Errorf("load lineage catalog: %w", err)
	}

	if !catalog.IsKnownSignature(signature) {
		return nil, &compat.Diagnostic{
			Code:    compat.CodeUnsupportedLineage,
			Summary: fmt.Sprintf("unsupported index schema %s", signature),
		}, nil
	}

	return readCanonicalSearchItems(ctx, q)
}

func readCanonicalSearchItems(ctx context.Context, q compat.Queryer) ([]models.IndexedRecord, *compat.Diagnostic, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT source, source_path, ordinal, role, text, project, uuid, timestamp, content_type
		FROM search_items
		ORDER BY source_path, ordinal, id
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("query recovery records: %w", err)
	}
	return readCanonicalSearchItemsFromRows(rows)
}

type recoveryRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

func readCanonicalSearchItemsFromRows(rows recoveryRows) (records []models.IndexedRecord, diag *compat.Diagnostic, err error) {
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close recovery records: %w", closeErr))
		}
	}()

	for rows.Next() {
		record, rowDiag := scanRecoveryRecord(rows)
		if rowDiag != nil {
			return nil, rowDiag, nil
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read recovery records: %w", err)
	}
	return records, nil, nil
}

type recoveryScanner interface {
	Scan(dest ...any) error
}

func scanRecoveryRecord(scanner recoveryScanner) (models.IndexedRecord, *compat.Diagnostic) {
	var source, sourcePath, role, text, contentType sql.NullString
	var project, uuid, timestamp sql.NullString
	var ordinal sql.NullInt64
	if err := scanner.Scan(&source, &sourcePath, &ordinal, &role, &text, &project, &uuid, &timestamp, &contentType); err != nil {
		return models.IndexedRecord{}, uninterpretableRecoveryRowDiagnostic()
	}
	if !source.Valid || !sourcePath.Valid || !ordinal.Valid || !role.Valid || !text.Valid || !contentType.Valid {
		return models.IndexedRecord{}, uninterpretableRecoveryRowDiagnostic()
	}
	return models.IndexedRecord{
		Source:      source.String,
		SourcePath:  sourcePath.String,
		Ordinal:     ordinal.Int64,
		Role:        role.String,
		Text:        text.String,
		Project:     recoveryStringPtr(project),
		UUID:        recoveryStringPtr(uuid),
		Timestamp:   recoveryStringPtr(timestamp),
		ContentType: contentType.String,
	}, nil
}

func recoveryStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func uninterpretableRecoveryRowDiagnostic() *compat.Diagnostic {
	return &compat.Diagnostic{
		Code:    compat.CodeUninterpretableRow,
		Summary: "index contains a row that cannot be interpreted as a canonical recovery record",
	}
}
