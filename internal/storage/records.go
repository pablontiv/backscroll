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
	SourcePath *string // supports * glob (converted to SQL LIKE %)
	After      *string
	Before     *string
	Limit      int
	MaxChars   int // if >0, truncate Text to this many characters
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
