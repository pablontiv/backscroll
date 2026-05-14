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
	TotalFiles    int
	TotalMessages int
	IndexedAt     time.Time
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

// ListSessions lists all indexed sessions, optionally filtered by project.
func (d *Database) ListSessions(project string, recent bool) ([]SessionEntry, error) {
	query := `
		SELECT DISTINCT si.source_path, si.project, MAX(si.timestamp) as ts
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

	if recent {
		query += " ORDER BY ts DESC"
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sessions []SessionEntry
	for rows.Next() {
		var entry SessionEntry
		var ts sql.NullString

		if err := rows.Scan(&entry.Path, &entry.Project, &ts); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}

		if ts.Valid {
			t, _ := time.Parse(time.RFC3339, ts.String)
			entry.Timestamp = t
		}

		// Load tags for this session
		tagRows, err := d.db.Query("SELECT tag FROM session_tags WHERE source_path = ? ORDER BY tag", entry.Path)
		if err != nil {
			return nil, fmt.Errorf("query tags for %s: %w", entry.Path, err)
		}

		for tagRows.Next() {
			var tag string
			if err := tagRows.Scan(&tag); err != nil {
				_ = tagRows.Close()
				return nil, fmt.Errorf("scan tag: %w", err)
			}
			entry.Tags = append(entry.Tags, tag)
		}
		_ = tagRows.Close()

		sessions = append(sessions, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
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
func (d *Database) OptimizeFTS() error {
	_, err := d.db.Exec("INSERT INTO messages_fts(messages_fts, rank) VALUES('optimize', 0)")
	if err != nil {
		return fmt.Errorf("optimize FTS5: %w", err)
	}

	// VACUUM to reclaim space
	_, err = d.db.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("vacuum database: %w", err)
	}

	return nil
}
