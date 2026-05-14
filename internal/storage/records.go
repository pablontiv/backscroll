package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// IndexedRecord represents a single record from search_items.
type IndexedRecord struct {
	Source      string
	SourcePath  string
	Ordinal     int64
	Role        string
	Text        string
	Project     *string
	UUID        *string
	Timestamp   *string
	ContentType string
}

// IndexedRecordQuery defines filter parameters for QueryIndexedRecords.
type IndexedRecordQuery struct {
	Project    *string
	Source     *string
	SourcePath *string
	After      *string
	Before     *string
	Limit      int
}

// QueryIndexedRecords returns records from search_items matching the query,
// ordered by source_path and ordinal.
func (d *Database) QueryIndexedRecords(q IndexedRecordQuery) ([]IndexedRecord, error) {
	baseQuery := `
		SELECT source, source_path, ordinal, role, text, project, uuid, timestamp, content_type
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
		whereClauses = append(whereClauses, "source_path = ?")
		args = append(args, *q.SourcePath)
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

	var records []IndexedRecord
	for rows.Next() {
		var r IndexedRecord
		var project, uuid, timestamp sql.NullString
		if err := rows.Scan(
			&r.Source, &r.SourcePath, &r.Ordinal, &r.Role, &r.Text,
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
		records = append(records, r)
	}
	return records, rows.Err()
}
