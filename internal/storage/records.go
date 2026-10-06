package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pablontiv/backscroll/internal/models"
)

const maxContextRecordWindow = 50

var (
	// ErrContextRecordNotFound identifies an exact context selector with no match.
	ErrContextRecordNotFound = errors.New("context record not found")
	// ErrContextRecordAmbiguous identifies an exact context selector with multiple matches.
	ErrContextRecordAmbiguous = errors.New("context record selector is ambiguous")
	// ErrInvalidContextRecordQuery identifies a malformed selector or window.
	ErrInvalidContextRecordQuery = errors.New("invalid context record query")
)

// ContextRecordQuery selects one anchor and the number of records to return on
// either side of it. Exactly one selector must be supplied: UUID, or the pair
// SourcePath and Ordinal. Before and After are independently limited to 0..50.
type ContextRecordQuery struct {
	UUID       *string
	SourcePath *string
	Ordinal    *int64
	Before     int
	After      int
}

// ContextRecord is an indexed record in an anchor-relative window.
type ContextRecord struct {
	UUID        *string
	SourcePath  string
	Ordinal     int64
	Role        string
	Origin      models.MessageOrigin
	Timestamp   *string
	ContentType string
	Source      string
	Text        string
	Anchor      bool
}

// ContextRecordNotFoundError reports that a valid exact selector matched no rows.
type ContextRecordNotFoundError struct {
	Query ContextRecordQuery
}

func (e *ContextRecordNotFoundError) Error() string {
	return fmt.Sprintf("%v: %s", ErrContextRecordNotFound, describeContextRecordSelector(e.Query))
}

func (e *ContextRecordNotFoundError) Unwrap() error { return ErrContextRecordNotFound }

// ContextRecordAmbiguousError reports that a valid exact selector matched at
// least two rows. Resolution intentionally stops after the second match.
type ContextRecordAmbiguousError struct {
	Query ContextRecordQuery
}

func (e *ContextRecordAmbiguousError) Error() string {
	return fmt.Sprintf("%v: %s", ErrContextRecordAmbiguous, describeContextRecordSelector(e.Query))
}

func (e *ContextRecordAmbiguousError) Unwrap() error { return ErrContextRecordAmbiguous }

// QueryContextRecords resolves an exact anchor in search_items and returns its
// position-based window, ordered by ordinal ASC, id ASC. It consults SQLite
// only; source files and indexed_files are deliberately not used.
func (d *Database) QueryContextRecords(ctx context.Context, q ContextRecordQuery) (_ []ContextRecord, retErr error) {
	if err := validateContextRecordQuery(q); err != nil {
		return nil, err
	}

	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin context record query: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback()
		}
	}()

	anchor, err := resolveContextRecordAnchor(ctx, tx, q)
	if err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		WITH ordered AS (
			SELECT id, uuid, source_path, ordinal, role, origin, timestamp,
			       content_type, source, text,
			       ROW_NUMBER() OVER (ORDER BY ordinal ASC, id ASC) AS row_position
			FROM search_items
			WHERE source_path = ?
		), anchor_position AS (
			SELECT row_position FROM ordered WHERE id = ?
		)
		SELECT ordered.uuid, ordered.source_path, ordered.ordinal, ordered.role,
		       ordered.origin, ordered.timestamp, ordered.content_type,
		       ordered.source, ordered.text, ordered.id = ?
		FROM ordered
		CROSS JOIN anchor_position
		WHERE ordered.row_position BETWEEN anchor_position.row_position - ?
		                               AND anchor_position.row_position + ?
		ORDER BY ordered.ordinal ASC, ordered.id ASC
	`, anchor.sourcePath, anchor.id, anchor.id, q.Before, q.After)
	if err != nil {
		return nil, fmt.Errorf("query context record window: %w", err)
	}
	defer func() { _ = rows.Close() }()

	records := make([]ContextRecord, 0, q.Before+q.After+1)
	for rows.Next() {
		var record ContextRecord
		var uuid, timestamp sql.NullString
		var isAnchor int
		if err := rows.Scan(
			&uuid, &record.SourcePath, &record.Ordinal, &record.Role,
			&record.Origin, &timestamp, &record.ContentType,
			&record.Source, &record.Text, &isAnchor,
		); err != nil {
			return nil, fmt.Errorf("scan context record: %w", err)
		}
		if uuid.Valid && uuid.String != "" {
			record.UUID = &uuid.String
		}
		if timestamp.Valid {
			record.Timestamp = &timestamp.String
		}
		record.Anchor = isAnchor != 0
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate context record window: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close context record window: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit context record query: %w", err)
	}
	return records, nil
}

type contextRecordAnchor struct {
	id         int64
	sourcePath string
}

func resolveContextRecordAnchor(ctx context.Context, tx *sql.Tx, q ContextRecordQuery) (contextRecordAnchor, error) {
	var query string
	var args []any
	if q.UUID != nil {
		query = `SELECT id, source_path FROM search_items WHERE uuid = ? ORDER BY id ASC LIMIT 2`
		args = []any{*q.UUID}
	} else {
		query = `SELECT id, source_path FROM search_items WHERE source_path = ? AND ordinal = ? ORDER BY id ASC LIMIT 2`
		args = []any{*q.SourcePath, *q.Ordinal}
	}

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return contextRecordAnchor{}, fmt.Errorf("resolve context record anchor: %w", err)
	}
	defer func() { _ = rows.Close() }()

	matches := make([]contextRecordAnchor, 0, 2)
	for rows.Next() {
		var match contextRecordAnchor
		if err := rows.Scan(&match.id, &match.sourcePath); err != nil {
			return contextRecordAnchor{}, fmt.Errorf("scan context record anchor: %w", err)
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return contextRecordAnchor{}, fmt.Errorf("iterate context record anchors: %w", err)
	}
	if len(matches) == 0 {
		return contextRecordAnchor{}, &ContextRecordNotFoundError{Query: q}
	}
	if len(matches) > 1 {
		return contextRecordAnchor{}, &ContextRecordAmbiguousError{Query: q}
	}
	return matches[0], nil
}

func validateContextRecordQuery(q ContextRecordQuery) error {
	uuidSelector := q.UUID != nil && q.SourcePath == nil && q.Ordinal == nil
	pathOrdinalSelector := q.UUID == nil && q.SourcePath != nil && q.Ordinal != nil
	if !uuidSelector && !pathOrdinalSelector {
		return fmt.Errorf("%w: provide exactly one selector: uuid or source_path with ordinal", ErrInvalidContextRecordQuery)
	}
	if q.Before < 0 || q.Before > maxContextRecordWindow {
		return fmt.Errorf("%w: before must be between 0 and %d", ErrInvalidContextRecordQuery, maxContextRecordWindow)
	}
	if q.After < 0 || q.After > maxContextRecordWindow {
		return fmt.Errorf("%w: after must be between 0 and %d", ErrInvalidContextRecordQuery, maxContextRecordWindow)
	}
	return nil
}

func describeContextRecordSelector(q ContextRecordQuery) string {
	if q.UUID != nil {
		return fmt.Sprintf("uuid %q", *q.UUID)
	}
	if q.SourcePath != nil && q.Ordinal != nil {
		return fmt.Sprintf("source_path %q ordinal %d", *q.SourcePath, *q.Ordinal)
	}
	return "invalid selector"
}

// IndexedRecordQuery defines filter parameters for QueryIndexedRecords.
type IndexedRecordQuery struct {
	Project    *string
	Source     *string
	SourcePath *string // supports * glob (converted to SQL LIKE %)
	After      *string
	Before     *string
	Limit      int
	MaxChars   int // if >0, truncate Text to this many characters
}

// QueryIndexedRecords returns records from search_items matching the query,
// ordered by source_path and ordinal.
func (d *Database) QueryIndexedRecords(q IndexedRecordQuery) ([]models.IndexedRecord, error) {
	baseQuery := `
		SELECT source, source_path, ordinal, role, origin, text, project, uuid, timestamp, content_type
		FROM search_items`

	var whereClauses []string
	var args []interface{}

	if q.Source != nil && *q.Source != "" {
		whereClauses = append(whereClauses, "source = ?")
		args = append(args, *q.Source)
	}
	if q.Project != nil && *q.Project != "" {
		whereClauses = append(whereClauses, "project = ?")
		args = append(args, *q.Project)
	}
	if q.SourcePath != nil && *q.SourcePath != "" {
		if strings.ContainsAny(*q.SourcePath, "*%") {
			whereClauses = append(whereClauses, "source_path LIKE ?")
			args = append(args, strings.ReplaceAll(*q.SourcePath, "*", "%"))
		} else {
			whereClauses = append(whereClauses, "source_path = ?")
			args = append(args, *q.SourcePath)
		}
	}
	if q.After != nil && *q.After != "" {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, *q.After)
	}
	if q.Before != nil && *q.Before != "" {
		whereClauses = append(whereClauses, "timestamp < ?")
		args = append(args, *q.Before)
	}

	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	baseQuery += " ORDER BY source_path, ordinal"
	if q.Limit > 0 {
		baseQuery += fmt.Sprintf(" LIMIT %d", q.Limit)
	}

	rows, err := d.db.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query indexed records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []models.IndexedRecord
	for rows.Next() {
		var r models.IndexedRecord
		var project, uuid, timestamp sql.NullString
		if err := rows.Scan(
			&r.Source, &r.SourcePath, &r.Ordinal, &r.Role, &r.Origin, &r.Text,
			&project, &uuid, &timestamp, &r.ContentType,
		); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		if project.Valid {
			r.Project = &project.String
		}
		if uuid.Valid {
			r.UUID = &uuid.String
		}
		if timestamp.Valid {
			r.Timestamp = &timestamp.String
		}
		if q.MaxChars > 0 && len(r.Text) > q.MaxChars {
			r.Text = r.Text[:q.MaxChars]
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// ResolveSessionPath looks up a session source_path by file path fragment or UUID.
// Returns the first match or empty string if not found.
func (d *Database) ResolveSessionPath(query string) (string, error) {
	// Try exact match first
	var path string
	err := d.db.QueryRow(
		"SELECT path FROM indexed_files WHERE path = ? LIMIT 1", query,
	).Scan(&path)
	if err == nil {
		return path, nil
	}

	// Try path contains query
	err = d.db.QueryRow(
		"SELECT path FROM indexed_files WHERE path LIKE ? LIMIT 1", "%"+query+"%",
	).Scan(&path)
	if err == nil {
		return path, nil
	}

	// Try UUID in search_items
	err = d.db.QueryRow(
		"SELECT source_path FROM search_items WHERE uuid = ? LIMIT 1", query,
	).Scan(&path)
	if err == nil {
		return path, nil
	}

	return "", nil
}
