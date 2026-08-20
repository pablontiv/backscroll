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
	switch signature {
	case
		"sha256:e2f7cd4bd71c964717c00fd67c2b3f396306307f578382c4a7956f83f8555a57", // v1
		"sha256:a4256a6029bbc953df37a12979fc81b1879ef9f8ba28ff5cfe6b78472dee804b", // v2
		"sha256:ed864d83d4c873bd34e0e3708f32a5bf47d65cac8ad55ff04305a81d91e9e22e", // v3
		"sha256:a7b05ddcb786f8d633fd2f27b19e0274fdf78dbd5c0e5ef420cecc831bcc3a8f", // v3 without source_metadata
		"sha256:b2bba3cec75bb3f6c697e78882e3924550832e3b5706d013f3f54d32a1c25acd", // v4
		"sha256:ce23ee41f1af6007bd0bde7b020f2cac4c215d23bae456bdfce25b90313c709e", // v5 with source_metadata
		"sha256:95c1c0aa96f1093511dd4e159ec3b574aacdfc67136531b2fb9c3cd01e90aad0", // v5 without source_metadata
		"sha256:68f237fe4a85c97df36522ebdd54703afb97ea84c3f0b0cd45e6400c5e95e220", // v6
		"sha256:dcaa1205df039fa545aa7ebe672d391acfcbf651869c2603ceef12dab9de00d2", // v7
		"sha256:6d503881ebac89e009644df5bf24cf7c4b1afed334afb37e6ee87d2ca6edae87", // v8
		"sha256:e41ae857c862ffa10516681b65bcd8c755484b338a6847dad5c0d770a202670f", // v9
		"sha256:8e28ce0fe1c5f3cd36f2d64ac7a7996f032e66c0c32468a0a206eb983491388c", // v10
		"sha256:975656bb5e894e12bd30aa65bb3366ca2bdeee23f59aca8e133b18a62e7ffad5", // v11
		"sha256:ff136d58048d69f02be857f632750ef3e2daa35fd6ba637f7645986950f83d31", // v12
		"sha256:ef52be2fb56acd0e51b38506746fa3594ed1de76bb0b5dd180767d3c903fb46d", // v13 canonical
		"sha256:952e517fe590b0c75a02996b6035ac6bb22143b9a707b6448ad05a592bf015b4", // v13 observed development ALTER-built alias
		"sha256:19d65765b8b0d0bb7d1c41692712c395627e005b7aeccca5f887c75be781e314", // v13 legacy ALTER-built alias
		"sha256:f52b4132322f3addb0fc01304f92ca6a2c2f4706e5ac010c59a5ea594cbd25b2": // v13 legacy schema_migrations alias
		return readCanonicalSearchItems(ctx, q)
	default:
		return nil, &compat.Diagnostic{
			Code:    compat.CodeUnsupportedLineage,
			Summary: fmt.Sprintf("unsupported index schema %s", signature),
		}, nil
	}
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
