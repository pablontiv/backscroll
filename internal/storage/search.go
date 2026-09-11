package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/hybrid"
	"github.com/pablontiv/backscroll/internal/models"
)

// SearchResult represents a single search result.
type SearchResult struct {
	ID           int
	Source       string
	SourcePath   string
	Ordinal      int
	Role         string
	Text         string
	Snippet      string
	Score        float64
	Timestamp    time.Time
	UUID         string
	Project      string
	ContentType  string
	SearchEcho   bool `json:"-"` // persisted call/result provenance; not a public output field
	MatchStage   string
	DroppedTerms []string
}

// Search performs a hybrid search (BM25 + optional vector) on the indexed content.
// It applies all filters and returns results ranked by BM25 score.
// When ContentType is empty, it queries both FTS tables and merges via RRF.
func (d *Database) Search(query string, opts models.SearchOptions) ([]SearchResult, error) {
	return searchTables(opts, func(table string, page models.SearchOptions) ([]SearchResult, error) {
		return d.searchTable(table, query, page)
	})
}

// searchTables shares filtering, echo exclusion, RRF and pagination between
// ordinary lexical queries and opt-in relaxation. Query syntax stays private.
func searchTables(opts models.SearchOptions, search func(string, models.SearchOptions) ([]SearchResult, error)) ([]SearchResult, error) {
	if opts.ContentType == "tool" {
		return search("tool_fts", opts)
	}
	if opts.ContentType != "" {
		return search("messages_fts", opts)
	}
	candidates := func(table string) ([]SearchResult, error) {
		return refillCandidatesWithoutDirectEchoes(withoutPaging(opts), func(page models.SearchOptions) ([]SearchResult, error) {
			return search(table, page)
		})
	}
	prose, err := candidates("messages_fts")
	if err != nil {
		return nil, err
	}
	tool, err := candidates("tool_fts")
	if err != nil {
		return nil, err
	}
	return paginate(mergeRRF(prose, tool), opts.Limit, opts.Offset), nil
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

	return d.searchTableQuery(ftsTable, ftsQuery, opts)
}

// searchTableQuery accepts only internally compiled MATCH expressions.
func (d *Database) searchTableQuery(ftsTable, ftsQuery string, opts models.SearchOptions) ([]SearchResult, error) {
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
			si.content_type,
			COALESCE(si.search_echo, 0)
		FROM %[1]s
		JOIN search_items si ON %[1]s.rowid = si.id
		%[2]s
		%[3]s
		ORDER BY score ASC, si.id ASC -- bm25() is lower-is-better; si.id keeps LIMIT/OFFSET pages stable
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
			&r.SearchEcho,
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

const unfilteredSearchCandidateLimit = 200

// withoutPaging returns a copy of opts with Limit/Offset cleared so each
// table query returns its full candidate set for cross-table merging.
func withoutPaging(o models.SearchOptions) models.SearchOptions {
	o.Limit = unfilteredSearchCandidateLimit
	o.Offset = 0
	return o
}

func refillCandidatesWithoutDirectEchoes(opts models.SearchOptions, loadPage func(models.SearchOptions) ([]SearchResult, error)) ([]SearchResult, error) {
	target := opts.Limit
	if target <= 0 {
		target = unfilteredSearchCandidateLimit
	}
	pageOpts := opts
	pageOpts.Limit = target

	filtered := make([]SearchResult, 0, target)
	seen := make(map[int]struct{})
	for len(filtered) < target {
		page, err := loadPage(pageOpts)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		for _, result := range page {
			if _, duplicate := seen[result.ID]; duplicate {
				continue
			}
			seen[result.ID] = struct{}{}
			if !isDirectBackscrollSearchEcho(result) {
				filtered = append(filtered, result)
				if len(filtered) == target {
					break
				}
			}
		}
		if len(page) < pageOpts.Limit {
			break
		}
		pageOpts.Offset += len(page)
	}
	return filtered, nil
}

// excludeDirectBackscrollSearchEchoes removes direct Backscroll retrieval calls
// from the tool candidates used for unfiltered recall. Explicit tool-only search
// bypasses this function and retains the commands.
func excludeDirectBackscrollSearchEchoes(results []SearchResult) []SearchResult {
	filtered := make([]SearchResult, 0, len(results))
	for _, result := range results {
		if !isDirectBackscrollSearchEcho(result) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func isDirectBackscrollSearchEcho(result SearchResult) bool {
	if result.ContentType != "tool" {
		return false
	}
	if result.SearchEcho {
		return true
	}
	fields := strings.Fields(result.Text)
	if len(fields) < 3 || !strings.EqualFold(fields[0], "bash") {
		return false
	}
	return fields[1] == "command=backscroll" && fields[2] == "search"
}

// asciiWhitespaceSQL is the ASCII subset of unicode.IsSpace. SQL-side echo
// matching trims a leading run and treats one separator between tokens.
const asciiWhitespaceSQL = "char(9, 10, 11, 12, 13, 32)"

// directBackscrollSearchEchoSQL is the SQL equivalent of
// isDirectBackscrollSearchEcho for the given search_items alias. Keep them in
// lockstep: tool rows with search_echo != 0, or a three-token prefix of
// case-insensitive "bash", exact "command=backscroll", exact "search".
// GLOB is case-sensitive, so only the first token uses a character class;
// LIKE would fold tokens 2 and 3. The pattern is prefix-only: a lookalike
// that does not start with that prefix, including path collisions and
// "searcher", must not match.
func directBackscrollSearchEchoSQL(alias string) string {
	trimmed := "ltrim(" + alias + ".text, " + asciiWhitespaceSQL + ")"
	sep := "'[' || " + asciiWhitespaceSQL + " || ']'"
	prefix := "'[Bb][Aa][Ss][Hh]' || " + sep + " || 'command=backscroll' || " + sep + " || 'search'"
	return alias + ".content_type = 'tool' AND (COALESCE(" + alias + ".search_echo, 0) != 0 OR " +
		trimmed + " GLOB (" + prefix + ") OR " +
		trimmed + " GLOB (" + prefix + " || " + sep + " || '*'))"
}

// unmarkedDirectSearchCallSQL matches tool rows stored as search_echo=0 whose
// serialized text is a direct Backscroll search call. Pre-#80 Codex/OpenCode
// readers wrote false as 0, so those rows are not in the v15 NULL backlog.
// Requeueing the call's source file lets identity pairing mark the result;
// output shape is never matched here.
func unmarkedDirectSearchCallSQL(alias string) string {
	trimmed := "ltrim(" + alias + ".text, " + asciiWhitespaceSQL + ")"
	sep := "'[' || " + asciiWhitespaceSQL + " || ']'"
	bash := "'[Bb][Aa][Ss][Hh]' || " + sep + " || 'command=backscroll' || " + sep + " || 'search'"
	execCmd := "'exec_command' || " + sep + " || 'cmd=backscroll' || " + sep + " || 'search'"
	glob := func(prefix string) string {
		return trimmed + " GLOB (" + prefix + ") OR " + trimmed + " GLOB (" + prefix + " || " + sep + " || '*')"
	}
	// Codex shell wrapper form: argv is exactly [<shell>, -c|-lc, "backscroll search ..."].
	// SerializeToolInput renders the JSON-encoded argv as compact JSON, so the
	// separator between the 'backscroll' and 'search' tokens depends on what the
	// original argv[2] looked like: a literal space (the common case), a JSON
	// single-letter escape (\t, \n, \f, \r) for the matching ASCII whitespace,
	// or a JSON \uXXXX escape for any other unicode.IsSpace rune (vertical tab,
	// NEL, NBSP, em/en spaces, line/paragraph separators). The reader's
	// isCodexDirectSearchCall uses strings.Fields on the unescaped command, so
	// it accepts every separator form — the SQL must mirror that exactly, or
	// the row stays at search_echo=0 forever.
	shellPrefixC := "'shell' || " + sep + " || 'command=[[]*sh' || '\",\"' || '-c' || '\",\"' || 'backscroll'"
	shellPrefixLC := "'shell' || " + sep + " || 'command=[[]*sh' || '\",\"' || '-lc' || '\",\"' || 'backscroll'"
	// shellSeparators lists the SQL fragments that evaluate to the separator text
	// between 'backscroll' and 'search' in the serialized third element. The
	// single-letter escapes and the \uXXXX char class cover every JSON encoding
	// of a rune that strings.Fields would split on. SQLite (via modernc.org/sqlite)
	// does not process backslash escapes in string literals by default, so we build
	// the JSON backslash via char(92) (92 = ASCII '\') and concatenate.
	backslash := "char(92)"
	shellSeparators := []string{
		sep, // literal whitespace
		backslash + " || 't'",
		backslash + " || 'n'",
		backslash + " || 'f'",
		backslash + " || 'r'",
		backslash + " || 'u[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]'",
	}
	// shellForms returns the bare + with-args GLOB clauses for one (prefix,
	// separator) pair. The trailing '"]' anchors the closing of the JSON
	// string for argv[2] and the array.
	shellForms := func(prefix, separator string) string {
		p := prefix + " || " + separator
		return trimmed + " GLOB (" + p + ` || '"]') OR ` +
			trimmed + " GLOB (" + p + ` || '*"]')`
	}
	// shellFormNotExtra returns the NOT GLOB guard for one (prefix, separator)
	// pair. Excludes 4+ argv-element shell calls (e.g. argv[2]="backscroll search ",
	// argv[3]="bar") that the reader rejects via len(Command)==3, so requeuing
	// them would loop forever: SQL matches every sync, reader never marks.
	shellFormNotExtra := func(prefix, separator string) string {
		notPattern := prefix + " || " + separator + " || '*' || '\",\"*'"
		return alias + ".text NOT GLOB (" + notPattern + ")"
	}
	orParts := []string{glob(bash), glob(execCmd)}
	notParts := make([]string, 0, len(shellSeparators)*2)
	for _, separator := range shellSeparators {
		orParts = append(orParts, shellForms(shellPrefixC, separator))
		orParts = append(orParts, shellForms(shellPrefixLC, separator))
		notParts = append(notParts, shellFormNotExtra(shellPrefixC, separator))
		notParts = append(notParts, shellFormNotExtra(shellPrefixLC, separator))
	}
	return alias + ".content_type = 'tool' AND COALESCE(" + alias + ".search_echo, 0) = 0 AND (" +
		strings.Join(orParts, " OR ") + ")" +
		" AND " + strings.Join(notParts, " AND ")
}

// mergeRRF uses Reciprocal Rank Fusion to merge two ranked lists by position,
// immune to score-scale differences between tokenizers (trigram vs porter).
// k=60 is the standard RRF constant.
func mergeRRF(proseResults, toolResults []SearchResult) []SearchResult {
	// Convert to hybrid.RankResult for RRF fusion
	toolRanking := make([]hybrid.RankResult, len(toolResults))
	for i, r := range toolResults {
		toolRanking[i] = hybrid.RankResult{
			ID:    fmt.Sprintf("%d", r.ID),
			Score: r.Score, // score value ignored by RRF; rank position is used
		}
	}

	proseRanking := make([]hybrid.RankResult, len(proseResults))
	for i, r := range proseResults {
		proseRanking[i] = hybrid.RankResult{
			ID:    fmt.Sprintf("%d", r.ID),
			Score: r.Score,
		}
	}

	// Fuse rankings via RRF
	fused := hybrid.ReciprocatRankFusion(60, toolRanking, proseRanking)

	// Map fused results back to SearchResult with RRF score
	// Create ID→SearchResult lookup from original results
	byID := make(map[string]*SearchResult)
	for i := range toolResults {
		key := fmt.Sprintf("%d", toolResults[i].ID)
		byID[key] = &toolResults[i]
	}
	for i := range proseResults {
		key := fmt.Sprintf("%d", proseResults[i].ID)
		// If already in byID (overlap), keep the existing entry's pointer
		if _, exists := byID[key]; !exists {
			byID[key] = &proseResults[i]
		}
	}

	// Build final list in RRF order
	final := make([]SearchResult, 0, len(fused))
	for _, f := range fused {
		if r, ok := byID[f.ID]; ok {
			result := *r
			result.Score = f.Score // Replace with RRF score
			final = append(final, result)
		}
	}

	return final
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
		return []SearchResult{}
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
