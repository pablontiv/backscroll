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

// SessionEvent represents a single event from session_events.
type SessionEvent struct {
	ID         int64
	Source     string
	SourcePath string
	Project    *string
	Ordinal    int64
	Timestamp  *string
	EventType  string
	Actor      *string
	Role       *string
	ToolName   *string
	ToolID     *string
	Command    *string
	ExitCode   *int64
	IsError    *bool
	Snippet    string
}

// SessionEventQuery defines filter parameters for QuerySessionEvents.
type SessionEventQuery struct {
	Project    *string // nil = all projects
	Source     *string // nil = all sources; "all" is normalized to nil
	SourcePath string  // supports * glob (converted to SQL LIKE %)
	EventType  *string // nil = all event types
	Role       string
	After      string
	Before     string
	Limit      int
}

// QuerySessionEvents returns events from session_events matching the query.
func (d *Database) QuerySessionEvents(q SessionEventQuery) ([]SessionEvent, error) {
	baseQuery := `
		SELECT id, source, source_path, project, ordinal, timestamp, event_type,
		       actor, role, tool_name, tool_id, command, exit_code, is_error, snippet
		FROM session_events`

	var where []string
	var args []interface{}

	if q.Project != nil {
		where = append(where, "project = ?")
		args = append(args, *q.Project)
	}
	if q.Source != nil {
		where = append(where, "source = ?")
		args = append(args, *q.Source)
	}
	if q.SourcePath != "" {
		if strings.ContainsAny(q.SourcePath, "*%") {
			where = append(where, "source_path LIKE ?")
			args = append(args, strings.ReplaceAll(q.SourcePath, "*", "%"))
		} else {
			where = append(where, "source_path = ?")
			args = append(args, q.SourcePath)
		}
	}
	if q.EventType != nil {
		where = append(where, "event_type = ?")
		args = append(args, *q.EventType)
	}
	if q.Role != "" {
		where = append(where, "role = ?")
		args = append(args, q.Role)
	}
	if q.After != "" {
		where = append(where, "timestamp >= ?")
		args = append(args, q.After)
	}
	if q.Before != "" {
		where = append(where, "timestamp < ?")
		args = append(args, q.Before)
	}

	if len(where) > 0 {
		baseQuery += " WHERE " + strings.Join(where, " AND ")
	}
	baseQuery += " ORDER BY source_path, ordinal, id"
	if q.Limit > 0 {
		baseQuery += fmt.Sprintf(" LIMIT %d", q.Limit)
	}

	rows, err := d.db.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query session events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []SessionEvent
	for rows.Next() {
		var e SessionEvent
		var project, timestamp, actor, role, toolName, toolID, command sql.NullString
		var exitCode sql.NullInt64
		var isError sql.NullInt64
		if err := rows.Scan(
			&e.ID, &e.Source, &e.SourcePath, &project, &e.Ordinal,
			&timestamp, &e.EventType, &actor, &role, &toolName, &toolID,
			&command, &exitCode, &isError, &e.Snippet,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if project.Valid {
			e.Project = &project.String
		}
		if timestamp.Valid {
			e.Timestamp = &timestamp.String
		}
		if actor.Valid {
			e.Actor = &actor.String
		}
		if role.Valid {
			e.Role = &role.String
		}
		if toolName.Valid {
			e.ToolName = &toolName.String
		}
		if toolID.Valid {
			e.ToolID = &toolID.String
		}
		if command.Valid {
			e.Command = &command.String
		}
		if exitCode.Valid {
			e.ExitCode = &exitCode.Int64
		}
		if isError.Valid {
			b := isError.Int64 != 0
			e.IsError = &b
		}
		events = append(events, e)
	}
	return events, rows.Err()
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
