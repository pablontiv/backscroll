package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/categories"
	"github.com/pablontiv/backscroll/internal/projects"
	"github.com/pablontiv/backscroll/internal/sequences"
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

	// Delete template_matches for purged rows (explicit — no CASCADE by design)
	if _, err := tx.Exec(`
		DELETE FROM template_matches
		WHERE (source_path, ordinal) IN (
			SELECT source_path, ordinal FROM search_items WHERE timestamp < ?
		)
	`, beforeStr); err != nil {
		return 0, fmt.Errorf("delete template_matches: %w", err)
	}

	// Delete satellite tool_events for the purged rows (explicit — perennial tables have no CASCADE)
	if _, err := tx.Exec(`
		DELETE FROM tool_events
		WHERE (source_path, ordinal) IN (
			SELECT source_path, ordinal FROM search_items WHERE timestamp < ?
		)
	`, beforeStr); err != nil {
		return 0, fmt.Errorf("delete tool_events: %w", err)
	}

	// Delete satellite correction_signals for the purged rows (explicit —
	// perennial tables have no CASCADE lifecycle by design).
	if _, err := tx.Exec(`
		DELETE FROM correction_signals
		WHERE (source_path, ordinal) IN (
			SELECT source_path, ordinal FROM search_items WHERE timestamp < ?
		)
	`, beforeStr); err != nil {
		return 0, fmt.Errorf("delete correction_signals: %w", err)
	}

	// Delete satellite annotations for the purged rows (explicit —
	// perennial tables have no CASCADE lifecycle by design).
	if _, err := tx.Exec(`
		DELETE FROM annotations
		WHERE (source_path, ordinal) IN (
			SELECT source_path, ordinal FROM search_items WHERE timestamp < ?
		)
	`, beforeStr); err != nil {
		return 0, fmt.Errorf("delete annotations: %w", err)
	}

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

	// Delete orphaned message_templates (derived table — if no matches, drop the template)
	if _, err := tx.Exec(`
		DELETE FROM message_templates
		WHERE id NOT IN (SELECT DISTINCT template_id FROM template_matches)
	`); err != nil {
		return 0, fmt.Errorf("delete orphaned templates: %w", err)
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

// RebuildFTS re-derives both FTS indexes from search_items using FTS5's
// external-content 'rebuild' command. It never touches search_items rows —
// the DB, not the filesystem, is the source of truth (perennity contract).
func (d *Database) RebuildFTS() error {
	// Single transaction: either both indexes re-derive or neither does —
	// a partial rebuild would leave one index stale and queries inconsistent.
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("rebuild messages_fts: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO tool_fts(tool_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("rebuild tool_fts: %w", err)
	}
	return tx.Commit()
}

// TemplateQueryOpts controls template aggregation queries.
type TemplateQueryOpts struct {
	MinSupport int
	Project    string
	Tag        string // filter by session_tags.tag
	After      string // RFC3339 timestamp
	Before     string // RFC3339 timestamp
}

// TemplateRow is a result from AggregateTemplates.
type TemplateRow struct {
	TemplateID       int64    `json:"template_id"`
	Signature        string   `json:"signature"`
	TemplateText     string   `json:"template_text"`
	OccurrenceCount  int      `json:"occurrence_count"`
	ProjectsAffected []string `json:"projects_affected"`
	SampleUUIDs      []string `json:"sample_uuids"`
	FirstSeen        string   `json:"first_seen"`
	LastSeen         string   `json:"last_seen"`
}

// AggregateTemplates returns templates meeting min_support and optional filters.
func (d *Database) AggregateTemplates(opts TemplateQueryOpts) ([]TemplateRow, error) {
	minSupport := opts.MinSupport
	if minSupport < 1 {
		minSupport = 3
	}

	// Derive count at query time from template_matches (occurrence_count column is legacy/unused)
	query := `
		SELECT
			mt.id,
			mt.signature,
			mt.template_text,
			mt.first_seen,
			mt.last_seen,
			COUNT(tm.id) as cnt,
			GROUP_CONCAT(DISTINCT si.project) as projects,
			GROUP_CONCAT(DISTINCT tm.item_uuid) as uuids
		FROM message_templates mt
		LEFT JOIN template_matches tm ON tm.template_id = mt.id
		LEFT JOIN search_items si ON si.source_path = tm.source_path AND si.ordinal = tm.ordinal
	`
	args := []interface{}{}

	whereAdded := false
	if opts.Project != "" {
		query += ` WHERE si.project = ?`
		args = append(args, opts.Project)
		whereAdded = true
	}

	if opts.Tag != "" {
		if whereAdded {
			query += ` AND tm.source_path IN (
				SELECT DISTINCT search_items.source_path FROM search_items
				JOIN session_tags ON search_items.source_path = session_tags.source_path
				WHERE session_tags.tag = ?
			)`
		} else {
			query += ` WHERE tm.source_path IN (
				SELECT DISTINCT search_items.source_path FROM search_items
				JOIN session_tags ON search_items.source_path = session_tags.source_path
				WHERE session_tags.tag = ?
			)`
			whereAdded = true
		}
		args = append(args, opts.Tag)
	}

	if opts.After != "" {
		if whereAdded {
			query += ` AND si.timestamp > ?`
		} else {
			query += ` WHERE si.timestamp > ?`
			whereAdded = true
		}
		args = append(args, opts.After)
	}

	if opts.Before != "" {
		if whereAdded {
			query += ` AND si.timestamp < ?`
		} else {
			query += ` WHERE si.timestamp < ?`
		}
		args = append(args, opts.Before)
	}

	query += ` GROUP BY mt.id HAVING cnt >= ? ORDER BY cnt DESC`
	args = append(args, minSupport)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query templates: %w", err)
	}
	defer rows.Close()

	var results []TemplateRow
	for rows.Next() {
		var r TemplateRow
		var projectsStr, uuidsStr sql.NullString
		if err := rows.Scan(&r.TemplateID, &r.Signature, &r.TemplateText,
			&r.FirstSeen, &r.LastSeen, &r.OccurrenceCount, &projectsStr, &uuidsStr); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		if projectsStr.Valid && projectsStr.String != "" {
			r.ProjectsAffected = strings.Split(projectsStr.String, ",")
		}
		if uuidsStr.Valid && uuidsStr.String != "" {
			uuids := strings.Split(uuidsStr.String, ",")
			// Limit to 3 samples for display.
			if len(uuids) > 3 {
				uuids = uuids[:3]
			}
			r.SampleUUIDs = uuids
		}

		results = append(results, r)
	}
	return results, rows.Err()
}

// CorrectionAggOpts filters and paginates correction aggregation.
type CorrectionAggOpts struct {
	Project       string
	MinConfidence float64
	Limit         int
	Offset        int
	PendingOnly   bool // NEW: only candidates WITHOUT a 'correction' annotation
}

// CorrectionCandidate represents an aggregated correction signal window.
type CorrectionCandidate struct {
	UUID          string
	SourcePath    string
	Ordinal       int
	Detectors     []string
	MaxConfidence float64
	TextSnippet   string
}

// AggregateCorrections returns correction candidates grouped by ordinal,
// ordered by max confidence descending. Detectors are sorted by name within
// each ordinal for determinism.
func (d *Database) AggregateCorrections(opts CorrectionAggOpts) ([]CorrectionCandidate, error) {
	query := `
		SELECT
			cs.source_path,
			cs.ordinal,
			si.text,
			MAX(cs.confidence) as max_confidence,
			GROUP_CONCAT(DISTINCT cs.detector ORDER BY cs.detector) as detectors,
			si.uuid
		FROM correction_signals cs
		JOIN search_items si ON (cs.source_path = si.source_path AND cs.ordinal = si.ordinal)
	`
	if opts.PendingOnly {
		query += `
		LEFT JOIN annotations a ON (
			si.source_path = a.source_path
			AND si.ordinal = a.ordinal
			AND a.kind = 'correction'
		)
		`
	}

	query += `WHERE si.source = 'session'`
	var args []interface{}

	if opts.PendingOnly {
		query += ` AND a.id IS NULL`
	}

	if opts.Project != "" {
		query += " AND si.project = ?"
		args = append(args, opts.Project)
	}

	query += ` GROUP BY cs.source_path, cs.ordinal, si.uuid
		HAVING MAX(cs.confidence) >= ?
		ORDER BY max_confidence DESC
	`
	args = append(args, opts.MinConfidence)

	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
		if opts.Offset > 0 {
			query += ` OFFSET ?`
			args = append(args, opts.Offset)
		}
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query corrections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var candidates []CorrectionCandidate
	for rows.Next() {
		var c CorrectionCandidate
		var detectorsStr, uuid string
		if err := rows.Scan(&c.SourcePath, &c.Ordinal, &c.TextSnippet, &c.MaxConfidence, &detectorsStr, &uuid); err != nil {
			return nil, fmt.Errorf("scan correction: %w", err)
		}
		c.UUID = uuid
		if detectorsStr != "" {
			c.Detectors = strings.Split(detectorsStr, ",")
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// UpsertAnnotation inserts or replaces an annotation for a message.
// Resolves the target message to canonical (source_path, ordinal) coordinates.
// If uuid is provided, resolves from uuid; otherwise uses source_path+ordinal.
// If both are provided and differ, returns error (no write).
// Returns error early if message not found (no write on validation failure).
func (d *Database) UpsertAnnotation(itemUUID, sourcePath string, ordinal int, kind, label string) error {
	var resolvedPath string
	var resolvedOrdinal int

	// Resolve canonical coordinates
	if itemUUID != "" {
		// Resolve from uuid
		if err := d.db.QueryRow("SELECT source_path, ordinal FROM search_items WHERE uuid = ?", itemUUID).Scan(&resolvedPath, &resolvedOrdinal); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("message not found: uuid=%q", itemUUID)
			}
			return fmt.Errorf("resolve uuid %q: %w", itemUUID, err)
		}

		// If caller also provided path/ordinal, validate they match
		if sourcePath != "" && ordinal >= 0 {
			if resolvedPath != sourcePath || resolvedOrdinal != ordinal {
				return fmt.Errorf("uuid and path/ordinal refer to different messages: uuid=%q resolved to (%q,%d), caller provided (%q,%d)", itemUUID, resolvedPath, resolvedOrdinal, sourcePath, ordinal)
			}
		}
	} else if sourcePath != "" && ordinal >= 0 {
		// Resolve from source_path+ordinal and validate message exists
		var uuid sql.NullString
		if err := d.db.QueryRow("SELECT uuid FROM search_items WHERE source_path = ? AND ordinal = ?", sourcePath, ordinal).Scan(&uuid); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("message not found: source_path=%q ordinal=%d", sourcePath, ordinal)
			}
			return fmt.Errorf("resolve source_path %q ordinal %d: %w", sourcePath, ordinal, err)
		}
		resolvedPath = sourcePath
		resolvedOrdinal = ordinal
		if uuid.Valid {
			itemUUID = uuid.String
		}
	} else {
		return fmt.Errorf("must provide either --uuid or both --path and --ordinal")
	}

	// Upsert with RESOLVED coordinates: INSERT OR REPLACE with current timestamp
	now := time.Now().Format(time.RFC3339)
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO annotations
		(item_uuid, source_path, ordinal, kind, label, source, created_at)
		VALUES (?, ?, ?, ?, ?, 'agent', ?)
	`, itemUUID, resolvedPath, resolvedOrdinal, kind, label, now)
	return err
}

// LoadSequencesOpts specifies filters for loading tool sequences.
type LoadSequencesOpts struct {
	Project string // if set, filter to this project
	After   string // ISO 8601 date, exclusive lower bound (tool_events.timestamp > After)
	Before  string // ISO 8601 date, exclusive upper bound (tool_events.timestamp < Before)
	Limit   int    // max sequences to return (0 = no limit)
	Offset  int    // skip first N sequences
}

// LoadToolSequences loads tool sequences for PrefixSpan mining.
// Returns per-session sequences derived from tool_events, with categories applied.
func (d *Database) LoadToolSequences(opts LoadSequencesOpts) ([]sequences.Sequence, error) {
	mapper, err := categories.Load()
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}

	// Build SQL with filters
	query := `
		SELECT source_path, tool_name, command_head
		FROM tool_events
		WHERE 1=1
	`
	var args []interface{}

	if opts.Project != "" {
		query += ` AND source_path IN (
			SELECT DISTINCT source_path FROM search_items WHERE project = ?
		)`
		args = append(args, opts.Project)
	}

	if opts.After != "" {
		query += ` AND (SELECT timestamp FROM search_items WHERE search_items.source_path = tool_events.source_path
			AND search_items.ordinal = tool_events.ordinal LIMIT 1) > ?`
		args = append(args, opts.After)
	}

	if opts.Before != "" {
		query += ` AND (SELECT timestamp FROM search_items WHERE search_items.source_path = tool_events.source_path
			AND search_items.ordinal = tool_events.ordinal LIMIT 1) < ?`
		args = append(args, opts.Before)
	}

	query += ` ORDER BY source_path, ordinal ASC`

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tool_events: %w", err)
	}
	defer rows.Close()

	// Group by source_path, categorize each tool, collect into sequences
	seqsByPath := make(map[string][]string)
	for rows.Next() {
		var sourcePath, toolName string
		var commandHead *string
		if err := rows.Scan(&sourcePath, &toolName, &commandHead); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		head := ""
		if commandHead != nil {
			head = *commandHead
		}
		cat := mapper.Categorize(toolName, head)
		seqsByPath[sourcePath] = append(seqsByPath[sourcePath], cat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Convert to []Sequence. Limit/Offset intentionally do NOT apply here:
	// they paginate the MINED PATTERNS at the CLI layer, never the input
	// corpus — truncating sessions before mining silently corrupts support
	// counts (and map order made the truncation nondeterministic).
	var result []sequences.Sequence
	for path, items := range seqsByPath {
		result = append(result, sequences.Sequence{SessionID: path, Items: items})
	}

	return result, nil
}

// StalePaths returns source paths from session rows whose extraction_version is NULL
// or older than currentVersion. Rows are ordered by last_indexed ASC (FIFO draining).
// This powers incremental re-parsing of files whose rich metadata needs backfill.
func (d *Database) StalePaths(currentVersion int) ([]string, error) {
	query := `
		SELECT DISTINCT search_items.source_path
		FROM search_items
		LEFT JOIN indexed_files ON search_items.source_path = indexed_files.path
		WHERE search_items.source = 'session'
		  AND (search_items.extraction_version IS NULL OR search_items.extraction_version < ?)
		ORDER BY indexed_files.last_indexed ASC, search_items.source_path ASC
	`

	rows, err := d.db.Query(query, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("query stale paths: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("scan stale path: %w", err)
		}
		paths = append(paths, path)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale paths: %w", err)
	}

	return paths, nil
}

// ReresolveProjects iterates all distinct source_paths where project='unknown' or project IS NULL,
// calls the resolver function for each path, and updates ALL rows for that path with the returned project ID.
// If resolver returns empty string or "unknown", the source_path is skipped and rows remain unchanged.
// Returns the count of DISTINCT source_paths that were successfully resolved (project changed).
func (d *Database) ReresolveProjects(ctx context.Context, resolver func(sourcePath string) string) (int64, error) {
	// Find all distinct source_paths with unknown or NULL project
	rows, err := d.db.QueryContext(ctx, `
		SELECT DISTINCT source_path FROM search_items
		WHERE project = 'unknown' OR project IS NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("query unknown source_paths: %w", err)
	}
	defer rows.Close()

	var sourcePaths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return 0, fmt.Errorf("scan source_path: %w", err)
		}
		sourcePaths = append(sourcePaths, path)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate source_paths: %w", err)
	}

	if len(sourcePaths) == 0 {
		return 0, nil
	}

	// Resolve each path and update in a single transaction
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var resolvedPaths int64
	for _, sourcePath := range sourcePaths {
		resolvedID := resolver(sourcePath)

		// Skip if resolver returned empty or "unknown"
		if resolvedID == "" || resolvedID == "unknown" {
			continue
		}

		// Update all rows for this source_path
		_, err := tx.ExecContext(ctx, `
			UPDATE search_items SET project = ? WHERE source_path = ?
		`, resolvedID, sourcePath)
		if err != nil {
			return 0, fmt.Errorf("update source_path %s: %w", sourcePath, err)
		}

		// Count this source_path as successfully resolved
		resolvedPaths++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit resolution transaction: %w", err)
	}

	return resolvedPaths, nil
}

// ReresolveProjectsWithRegistry re-resolves project identities using the global
// registry, correcting historical fallback labels when a registry entry matches.
// It iterates DISTINCT (source_path, project) tuples; for each path, decodes cwd,
// calls Identify with the registry, and updates rows if the registry ID differs
// from the stored fallback. Returns count of source_paths updated.
// Only registry matches count — fallback-only paths are skipped (no churn).
func (d *Database) ReresolveProjectsWithRegistry(ctx context.Context, registry projects.ProjectRegistry) (int64, error) {
	if len(registry.Projects) == 0 {
		return 0, nil // no registry entries, nothing to resolve
	}

	// Find all distinct (source_path, project) tuples where we might have a fallback label
	rows, err := d.db.QueryContext(ctx, `
		SELECT DISTINCT si.source_path, si.project
		FROM search_items si
		WHERE si.project IS NOT NULL
		ORDER BY si.source_path
	`)
	if err != nil {
		return 0, fmt.Errorf("query paths for registry re-resolution: %w", err)
	}
	defer rows.Close()

	type pathLabel struct {
		path    string
		project string
	}
	var pathLabels []pathLabel
	for rows.Next() {
		var path, proj string
		if err := rows.Scan(&path, &proj); err != nil {
			return 0, fmt.Errorf("scan path/project: %w", err)
		}
		pathLabels = append(pathLabels, pathLabel{path, proj})
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate path/project: %w", err)
	}

	if len(pathLabels) == 0 {
		return 0, nil
	}

	// Re-resolve each path against the registry
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var updatedPaths int64
	for _, pl := range pathLabels {
		cwd := projects.DecodeCwdFromSessionPath(pl.path)
		if cwd == "" {
			continue // can't decode path, skip
		}

		// Normalize cross-host equivalences
		cwd = projects.NormalizeRootEquivalence(cwd, registry)

		// Try registry match
		id := projects.Identify(cwd, registry)
		if !id.FromRegistry {
			continue // registry didn't match, skip (no churn on fallback-only)
		}

		// Update only if the registry ID differs from stored project
		if id.ProjectID != pl.project {
			_, err := tx.ExecContext(ctx, `
				UPDATE search_items SET project = ? WHERE source_path = ?
			`, id.ProjectID, pl.path)
			if err != nil {
				return 0, fmt.Errorf("update source_path %s: %w", pl.path, err)
			}
			updatedPaths++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit registry re-resolution transaction: %w", err)
	}

	return updatedPaths, nil
}
