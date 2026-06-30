package storage

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Stats represents indexing statistics.
type Stats struct {
	TotalFiles      int
	TotalMessages   int
	IndexedAt       time.Time
	TotalChunks     int
	TotalEmbeddings int
	TotalVectors    int // chunks with embedding vector stored (V3)
}

// GetStats returns indexing statistics.
func (d *Database) GetStats() (Stats, error) {
	var stats Stats

	// Get total files
	err := d.db.QueryRow("SELECT COUNT(*) FROM indexed_files").Scan(&stats.TotalFiles)
	if err != nil {
		return stats, fmt.Errorf("count files: %w", err)
	}

	// Get total messages
	err = d.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source = 'session'").Scan(&stats.TotalMessages)
	if err != nil {
		return stats, fmt.Errorf("count messages: %w", err)
	}

	// Get last indexed time (most recent timestamp)
	var lastIndexed sql.NullString
	err = d.db.QueryRow("SELECT MAX(last_indexed) FROM indexed_files").Scan(&lastIndexed)
	if err != nil && err != sql.ErrNoRows {
		return stats, fmt.Errorf("get last indexed: %w", err)
	}

	if lastIndexed.Valid {
		// SQLite CURRENT_TIMESTAMP returns "YYYY-MM-DD HH:MM:SS"; also handle RFC3339.
		formats := []string{time.RFC3339, "2006-01-02 15:04:05"}
		for _, f := range formats {
			if t, err := time.Parse(f, lastIndexed.String); err == nil {
				stats.IndexedAt = t
				break
			}
		}
	}

	// Get chunk and embedding counts (V2 tables — present after migration)
	_ = d.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&stats.TotalChunks)
	_ = d.db.QueryRow("SELECT COUNT(*) FROM embedding_metadata").Scan(&stats.TotalEmbeddings)
	// V3: chunks with embedding vector blob
	_ = d.db.QueryRow("SELECT COUNT(*) FROM chunks WHERE embedding IS NOT NULL").Scan(&stats.TotalVectors)

	return stats, nil
}

// TopicEntry represents a single topic with its document frequency.
type TopicEntry struct {
	Term  string
	Count int
}

// GetTopics returns the most frequently occurring terms across all indexed content.
// This uses a heuristic approach by analyzing the search_items text content
// and extracting common terms (3+ chars, not stopwords).
// For a full vocab-based approach, use FTS5's fts5vocab() function directly.
func (d *Database) GetTopics(project string, limit int) ([]TopicEntry, error) {
	if limit == 0 {
		limit = 50
	}

	query := "SELECT text FROM search_items WHERE source = 'session'"
	var args []interface{}
	if project != "" {
		query += " AND project = ?"
		args = append(args, project)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query text: %w", err)
	}
	defer func() { _ = rows.Close() }()

	freq := make(map[string]int)
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			continue
		}
		for _, word := range strings.Fields(strings.ToLower(text)) {
			word = strings.Trim(word, ".,!?;:\"'()[]{}*/\\-_")
			if len(word) >= 4 && !isCommonWord(word) {
				freq[word]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate text: %w", err)
	}

	// Sort by frequency
	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range freq {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].v > sorted[j].v
	})

	var entries []TopicEntry
	for _, item := range sorted {
		if len(entries) >= limit {
			break
		}
		entries = append(entries, TopicEntry{Term: item.k, Count: item.v})
	}
	return entries, nil
}

// isCommonWord returns true for very common English words that aren't useful as topics.
func isCommonWord(w string) bool {
	common := map[string]bool{
		"this": true, "that": true, "with": true, "from": true,
		"have": true, "will": true, "been": true, "were": true,
		"they": true, "them": true, "their": true, "what": true,
		"when": true, "where": true, "which": true, "more": true,
		"also": true, "some": true, "than": true, "into": true,
		"your": true, "there": true, "here": true, "would": true,
		"could": true, "should": true, "about": true, "then": true,
	}
	return common[w]
}

// SessionEntry represents a session record.
type SessionEntry struct {
	Path      string
	Project   string
	Timestamp time.Time
	Tags      []string
}

// ListOptions encapsulates flexible list query parameters (v2 grammar).
// Supports input filtering, ordering, limits/offsets.
type ListOptions struct {
	Project     string // filter to project
	AllProjects bool   // if true, ignore Project filter
	Input       string // filter to input ID (maps to source field)
	Order       string // e.g., "timestamp:desc", "timestamp:asc" (default: no ordering)
	Limit       int    // result limit (0 = no limit)
	Offset      int    // result offset
	After       *time.Time
	Before      *time.Time
	// Structured query filters (when present, routes to session_events table instead of search_items)
	EventType string // filter to event_type (e.g., "tool_call")
	ToolName  string // filter to tool_name (e.g., "bash", "subagent")
	Command   string // filter to command field
}

// ListItemsV2 lists indexed search items using v2 filter grammar.
// Supports --input (maps to source), --order (timestamp:asc|desc), --limit, --offset, --after, --before.
func (d *Database) ListItemsV2(opts ListOptions) ([]SessionEntry, error) {
	query := `
		SELECT si.source_path, si.project, MAX(si.timestamp) as ts
		FROM search_items si
		WHERE 1=1
	`

	var args []interface{}

	// Filter by input (source field)
	if opts.Input != "" {
		// Map input IDs to source field values
		source := opts.Input
		// For now, map directly; in future may need mapping table
		query += " AND si.source = ?"
		args = append(args, source)
	}

	// Filter by project
	if opts.Project != "" && !opts.AllProjects {
		query += " AND si.project = ?"
		args = append(args, opts.Project)
	}

	// Date filters
	if opts.After != nil {
		query += " AND si.timestamp > ?"
		args = append(args, opts.After.Format(time.RFC3339))
	}
	if opts.Before != nil {
		query += " AND si.timestamp < ?"
		args = append(args, opts.Before.Format(time.RFC3339))
	}

	query += `
		GROUP BY si.source_path, si.project
	`

	// Apply ordering
	if opts.Order != "" {
		if strings.HasPrefix(opts.Order, "timestamp:desc") {
			query += " ORDER BY ts DESC"
		} else if strings.HasPrefix(opts.Order, "timestamp:asc") {
			query += " ORDER BY ts ASC"
		}
	}

	// Apply limit/offset
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query items v2: %w", err)
	}

	var sessions []SessionEntry
	for rows.Next() {
		var entry SessionEntry
		var ts sql.NullString

		if err := rows.Scan(&entry.Path, &entry.Project, &ts); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan item: %w", err)
		}

		if ts.Valid {
			t, _ := time.Parse(time.RFC3339, ts.String)
			entry.Timestamp = t
		}

		sessions = append(sessions, entry)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("iterate items: %w", err)
	}

	// Load tags for all sessions in one pass using GROUP_CONCAT.
	if len(sessions) == 0 {
		return sessions, nil
	}

	paths := make([]interface{}, len(sessions))
	for i, s := range sessions {
		paths[i] = s.Path
	}
	placeholder := strings.Repeat("?,", len(paths))
	placeholder = placeholder[:len(placeholder)-1]

	tagRows, err := d.db.Query(
		"SELECT source_path, tag FROM session_tags WHERE source_path IN ("+placeholder+") ORDER BY source_path, tag",
		paths...,
	)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer func() { _ = tagRows.Close() }()

	tagsByPath := make(map[string][]string)
	for tagRows.Next() {
		var path, tag string
		if err := tagRows.Scan(&path, &tag); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tagsByPath[path] = append(tagsByPath[path], tag)
	}
	if err := tagRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}

	for i := range sessions {
		sessions[i].Tags = tagsByPath[sessions[i].Path]
	}

	return sessions, nil
}

// ListSessions lists indexed sessions, optionally filtered by project.
// If recent > 0, returns the N most recent sessions ordered by timestamp descending.
func (d *Database) ListSessions(project string, recent int) ([]SessionEntry, error) {
	query := `
		SELECT si.source_path, si.project, MAX(si.timestamp) as ts
		FROM search_items si
		WHERE si.source = 'session'
	`

	var args []interface{}
	if project != "" {
		query += " AND si.project = ?"
		args = append(args, project)
	}

	query += `
		GROUP BY si.source_path, si.project
	`

	if recent > 0 {
		query += fmt.Sprintf(" ORDER BY ts DESC LIMIT %d", recent)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}

	// Collect sessions first, then close rows before issuing the tags query.
	// With SetMaxOpenConns(1), keeping rows open while issuing a second query
	// would deadlock on the single connection.
	var sessions []SessionEntry
	for rows.Next() {
		var entry SessionEntry
		var ts sql.NullString

		if err := rows.Scan(&entry.Path, &entry.Project, &ts); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan session: %w", err)
		}

		if ts.Valid {
			t, _ := time.Parse(time.RFC3339, ts.String)
			entry.Timestamp = t
		}

		sessions = append(sessions, entry)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}

	// Load tags for all sessions in one pass using GROUP_CONCAT.
	if len(sessions) == 0 {
		return sessions, nil
	}

	paths := make([]interface{}, len(sessions))
	for i, s := range sessions {
		paths[i] = s.Path
	}
	placeholder := strings.Repeat("?,", len(paths))
	placeholder = placeholder[:len(placeholder)-1]

	tagRows, err := d.db.Query(
		"SELECT source_path, tag FROM session_tags WHERE source_path IN ("+placeholder+") ORDER BY source_path, tag",
		paths...,
	)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer func() { _ = tagRows.Close() }()

	tagsByPath := make(map[string][]string)
	for tagRows.Next() {
		var path, tag string
		if err := tagRows.Scan(&path, &tag); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tagsByPath[path] = append(tagsByPath[path], tag)
	}
	if err := tagRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tags: %w", err)
	}

	for i := range sessions {
		sessions[i].Tags = tagsByPath[sessions[i].Path]
	}

	return sessions, nil
}

// Validate performs integrity checks on the database.
func (d *Database) Validate() error {
	// Check that all required tables exist
	tables := []string{
		"indexed_files",
		"search_items",
		"session_events",
		"session_tags",
		"dynamic_stopwords",
		"schema_migrations",
	}

	for _, table := range tables {
		var exists int
		err := d.db.QueryRow(`
			SELECT COUNT(*) FROM sqlite_master
			WHERE type='table' AND name=?
		`, table).Scan(&exists)
		if err != nil || exists == 0 {
			return fmt.Errorf("table %s does not exist", table)
		}
	}

	// Check FTS5 virtual table
	var ftsExists int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='messages_fts'
	`).Scan(&ftsExists)
	if err != nil || ftsExists == 0 {
		return fmt.Errorf("FTS5 virtual table messages_fts does not exist")
	}

	// Check FTS5 virtual table for tools
	var toolFTSExists int
	err = d.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='tool_fts'
	`).Scan(&toolFTSExists)
	if err != nil || toolFTSExists == 0 {
		return fmt.Errorf("FTS5 virtual table tool_fts does not exist")
	}

	// Check for orphaned search_items (not in indexed_files)
	var orphans int
	err = d.db.QueryRow(`
		SELECT COUNT(*) FROM search_items
		WHERE source_path NOT IN (SELECT path FROM indexed_files)
	`).Scan(&orphans)
	if err != nil {
		return fmt.Errorf("check for orphans: %w", err)
	}

	if orphans > 0 {
		return fmt.Errorf("found %d orphaned search_items", orphans)
	}

	return nil
}

// Purge deletes records older than the specified date.
// Returns the count of deleted items.
func (d *Database) Purge(before string) (int64, error) {
	// Parse the date
	beforeTime, err := time.Parse(time.RFC3339, before)
	if err != nil {
		// Try parsing as ISO date
		beforeTime, err = time.Parse("2006-01-02", before)
		if err != nil {
			return 0, fmt.Errorf("invalid date format: %w", err)
		}
	}

	beforeStr := beforeTime.Format(time.RFC3339)

	tx, err := d.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Find source_paths to delete
	rows, err := tx.Query(`
		SELECT DISTINCT source_path FROM search_items
		WHERE timestamp < ?
	`, beforeStr)
	if err != nil {
		return 0, fmt.Errorf("find items to purge: %w", err)
	}

	var sourcePaths []string
	for rows.Next() {
		var sp string
		if err := rows.Scan(&sp); err != nil {
			_ = rows.Close()
			return 0, err
		}
		sourcePaths = append(sourcePaths, sp)
	}
	_ = rows.Close()

	// Delete from search_items
	result, err := tx.Exec(`
		DELETE FROM search_items
		WHERE timestamp < ?
	`, beforeStr)
	if err != nil {
		return 0, fmt.Errorf("delete search_items: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}

	// Delete from session_events
	_, err = tx.Exec(`
		DELETE FROM session_events
		WHERE timestamp < ?
	`, beforeStr)
	if err != nil {
		return 0, fmt.Errorf("delete session_events: %w", err)
	}

	// Delete indexed_files entries for paths that have no more items
	for _, sp := range sourcePaths {
		var count int
		err := tx.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path = ?", sp).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("count remaining items: %w", err)
		}

		if count == 0 {
			_, err := tx.Exec("DELETE FROM indexed_files WHERE path = ?", sp)
			if err != nil {
				return 0, fmt.Errorf("delete indexed_files: %w", err)
			}

			// Also delete session tags if this is a session
			_, err = tx.Exec("DELETE FROM session_tags WHERE source_path = ?", sp)
			if err != nil {
				return 0, fmt.Errorf("delete session_tags: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return deleted, nil
}

// OptimizeFTS optimizes the FTS5 index for better query performance.
// Note: VACUUM was intentionally removed; it requires an exclusive WAL lock
// and hangs indefinitely when other connections (or pool connections) exist.
func (d *Database) OptimizeFTS() error {
	_, err := d.db.Exec("INSERT INTO messages_fts(messages_fts, rank) VALUES('optimize', 0)")
	if err != nil {
		return fmt.Errorf("optimize messages_fts: %w", err)
	}
	_, err = d.db.Exec("INSERT INTO tool_fts(tool_fts, rank) VALUES('optimize', 0)")
	if err != nil {
		return fmt.Errorf("optimize tool_fts: %w", err)
	}
	return nil
}
