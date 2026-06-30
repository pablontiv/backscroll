package storage

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

// SearchResult represents a single search result.
type SearchResult struct {
	ID          int
	Source      string
	SourcePath  string
	Ordinal     int
	Role        string
	Text        string
	Snippet     string
	Score       float64
	Timestamp   time.Time
	UUID        string
	Project     string
	ContentType string
}

// Search performs a hybrid search (BM25 + optional vector) on the indexed content.
// It applies all filters and returns results ranked by BM25 score.
// When ContentType is empty, it queries both FTS tables and merges by normalized score.
func (d *Database) Search(query string, opts models.SearchOptions) ([]SearchResult, error) {
	switch opts.ContentType {
	case "tool":
		return d.searchTable("tool_fts", query, opts)
	case "text", "code":
		return d.searchTable("messages_fts", query, opts)
	case "":
		// Unfiltered search: query both tables and merge
		prose, err := d.searchTable("messages_fts", query, withoutPaging(opts))
		if err != nil {
			return nil, err
		}
		tool, err := d.searchTable("tool_fts", query, withoutPaging(opts))
		if err != nil {
			return nil, err
		}
		merged := mergeNormalized(prose, tool)
		return paginate(merged, opts.Limit, opts.Offset), nil
	default:
		return d.searchTable("messages_fts", query, opts)
	}
}

// searchTable queries a single FTS table with all filters applied.
func (d *Database) searchTable(ftsTable, query string, opts models.SearchOptions) ([]SearchResult, error) {
	// Load dynamic stopwords
	stopwords, err := d.loadStopwords()
	if err != nil {
		return nil, fmt.Errorf("load stopwords: %w", err)
	}

	// Pick sanitizer based on table name
	var ftsQuery string
	if ftsTable == "tool_fts" {
		ftsQuery = sanitizeFTS5QueryTrigram(query, stopwords)
	} else {
		ftsQuery = sanitizeFTS5Query(query, stopwords)
	}

	// Build WHERE clause for filters
	var whereClauses []string
	var args []interface{}

	// Source filter (normalize source names)
	if opts.Source != "" {
		normalizedSource := normalizeSource(opts.Source)
		if normalizedSource != "" {
			whereClauses = append(whereClauses, "si.source = ?")
			args = append(args, normalizedSource)
		}
	}

	// Project filter
	if opts.Project != "" && !opts.AllProjects {
		whereClauses = append(whereClauses, "si.project = ?")
		args = append(args, opts.Project)
	}

	// Role filter
	if opts.Role != "" {
		whereClauses = append(whereClauses, "si.role = ?")
		args = append(args, opts.Role)
	}

	// SourcePath filter (exact path, SQL LIKE pattern, or * glob)
	if opts.SourcePath != "" {
		if strings.ContainsAny(opts.SourcePath, "*%") {
			whereClauses = append(whereClauses, "si.source_path LIKE ?")
			args = append(args, strings.ReplaceAll(opts.SourcePath, "*", "%"))
		} else {
			whereClauses = append(whereClauses, "si.source_path = ?")
			args = append(args, opts.SourcePath)
		}
	}

	// ContentType filter
	if opts.ContentType != "" {
		whereClauses = append(whereClauses, "si.content_type = ?")
		args = append(args, opts.ContentType)
	}

	// Date filters
	if opts.After != nil {
		whereClauses = append(whereClauses, "si.timestamp > ?")
		args = append(args, opts.After.Format(time.RFC3339))
	}

	if opts.Before != nil {
		// Use exclusive < comparison for "before"
		whereClauses = append(whereClauses, "si.timestamp < ?")
		args = append(args, opts.Before.Format(time.RFC3339))
	}

	// Tag filter (requires JOIN with session_tags)
	var tagJoin string
	if opts.Tag != "" {
		tagJoin = `
			LEFT JOIN session_tags st ON si.source_path = st.source_path
		`
		whereClauses = append(whereClauses, "st.tag = ?")
		args = append(args, opts.Tag)
	}

	// Build all WHERE conditions (including FTS5 MATCH)
	if ftsQuery != "" {
		whereClauses = append([]string{ftsTable + " MATCH ?"}, whereClauses...)
		args = append([]interface{}{ftsQuery}, args...)
	}

	// Build the full WHERE clause
	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	sqlQuery := fmt.Sprintf(`
		SELECT
			si.id,
			si.source,
			si.source_path,
			si.ordinal,
			si.role,
			si.text,
			snippet(%[1]s, 0, '<b>', '</b>', '...', 32) as snippet,
			bm25(%[1]s) as score,
			si.timestamp,
			si.uuid,
			si.project,
			si.content_type
		FROM %[1]s
		JOIN search_items si ON %[1]s.rowid = si.id
		%[2]s
		%[3]s
		ORDER BY score DESC
		LIMIT ? OFFSET ?
	`, ftsTable, tagJoin, whereSQL)

	// Add limit and offset
	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)

	// Execute query
	rows, err := d.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("execute search query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var ts sql.NullString
		var uuid sql.NullString
		var project sql.NullString

		err := rows.Scan(
			&r.ID,
			&r.Source,
			&r.SourcePath,
			&r.Ordinal,
			&r.Role,
			&r.Text,
			&r.Snippet,
			&r.Score,
			&ts,
			&uuid,
			&project,
			&r.ContentType,
		)
		if err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}

		// Parse timestamp
		if ts.Valid {
			t, _ := time.Parse(time.RFC3339, ts.String)
			r.Timestamp = t
		}

		if uuid.Valid {
			r.UUID = uuid.String
		}

		if project.Valid {
			r.Project = project.String
		}

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}

	return results, nil
}

// loadStopwords loads the dynamic stopwords from the database.
func (d *Database) loadStopwords() (map[string]struct{}, error) {
	rows, err := d.db.Query("SELECT term FROM dynamic_stopwords")
	if err != nil {
		// If table doesn't exist, return empty map
		if strings.Contains(err.Error(), "no such table") {
			return make(map[string]struct{}), nil
		}
		return nil, fmt.Errorf("query stopwords: %w", err)
	}
	defer func() { _ = rows.Close() }()

	stopwords := make(map[string]struct{})
	for rows.Next() {
		var term string
		if err := rows.Scan(&term); err != nil {
			continue
		}
		stopwords[strings.ToLower(term)] = struct{}{}
	}

	return stopwords, nil
}

// sanitizeFTS5Query sanitizes a query for FTS5 by:
// 1. Filtering out stopwords
// 2. Wrapping remaining tokens in quotes and prefix wildcard
// 3. Falling back to unfiltered if all tokens were stopwords
func sanitizeFTS5Query(query string, stopwords map[string]struct{}) string {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return ""
	}

	var filtered []string
	for _, t := range tokens {
		if _, ok := stopwords[strings.ToLower(t)]; !ok {
			filtered = append(filtered, t)
		}
	}

	// If all tokens were stopwords, use unfiltered
	if len(filtered) == 0 {
		filtered = tokens
	}

	// Wrap each token in quotes with prefix wildcard
	var parts []string
	for _, t := range filtered {
		escaped := strings.ReplaceAll(t, `"`, `""`)
		parts = append(parts, fmt.Sprintf(`"%s"*`, escaped))
	}

	return strings.Join(parts, " ")
}

// sanitizeFTS5QueryTrigram builds a MATCH query for the trigram-tokenized
// tool_fts. Unlike the porter sanitizer it does NOT append a prefix wildcard
// (trigram matches substrings directly) and it preserves path/command tokens.
func sanitizeFTS5QueryTrigram(query string, stopwords map[string]struct{}) string {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return ""
	}
	var filtered []string
	for _, t := range tokens {
		if _, ok := stopwords[strings.ToLower(t)]; !ok {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) == 0 {
		filtered = tokens
	}
	var parts []string
	for _, t := range filtered {
		escaped := strings.ReplaceAll(t, `"`, `""`)
		parts = append(parts, fmt.Sprintf(`"%s"`, escaped))
	}
	return strings.Join(parts, " ")
}

// withoutPaging returns a copy of opts with Limit/Offset cleared so each
// table query returns its full candidate set for cross-table merging.
func withoutPaging(o models.SearchOptions) models.SearchOptions {
	o.Limit = 200
	o.Offset = 0
	return o
}

// mergeNormalized min-max normalizes each slice's BM25 scores into [0,1]
// (BM25 is negative; higher is better) and returns the union sorted by the
// normalized score descending. Cross-index ordering is approximate by design.
func mergeNormalized(a, b []SearchResult) []SearchResult {
	normalize(a)
	normalize(b)
	out := append(append([]SearchResult{}, a...), b...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// normalize min-max normalizes a slice of SearchResults' scores into [0,1].
func normalize(rs []SearchResult) {
	if len(rs) == 0 {
		return
	}
	min, max := rs[0].Score, rs[0].Score
	for _, r := range rs {
		if r.Score < min {
			min = r.Score
		}
		if r.Score > max {
			max = r.Score
		}
	}
	span := max - min
	for i := range rs {
		if span == 0 {
			rs[i].Score = 1
		} else {
			rs[i].Score = (rs[i].Score - min) / span
		}
	}
}

// paginate applies limit and offset to a result slice.
func paginate(rs []SearchResult, limit, offset int) []SearchResult {
	if limit == 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(rs) {
		return nil
	}
	end := offset + limit
	if end > len(rs) {
		end = len(rs)
	}
	return rs[offset:end]
}

// normalizeSource normalizes source names:
// - "" or "all" -> "" (no filter)
// - "sessions" -> "session"
// - "plans" -> "plan"
// - others -> pass through
func normalizeSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case "", "all":
		return ""
	case "sessions":
		return "session"
	case "plans":
		return "plan"
	default:
		return source
	}
}
