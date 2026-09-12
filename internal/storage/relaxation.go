package storage

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/pablontiv/backscroll/internal/models"
)

type recallTerm struct {
	text      string
	protected bool
	phrase    bool
}

// ValidateRelaxationQuery validates opt-in syntax before startup side effects.
// Ordinary search deliberately keeps its existing literal-token grammar.
func ValidateRelaxationQuery(query string) error {
	_, err := parseRecallTerms(query)
	return err
}

func parseRecallTerms(query string) ([]recallTerm, error) {
	var terms []recallTerm
	for rest := strings.TrimSpace(query); rest != ""; rest = strings.TrimSpace(rest) {
		term := recallTerm{}
		if strings.HasPrefix(rest, "+") {
			term.protected = true
			rest = rest[1:]
		}
		if strings.HasPrefix(rest, `"`) {
			term.phrase, term.protected = true, true
			rest = rest[1:]
			var text strings.Builder
			closed := false
			for len(rest) > 0 {
				if strings.HasPrefix(rest, `""`) {
					text.WriteByte('"')
					rest = rest[2:]
				} else if rest[0] == '"' {
					rest, closed = rest[1:], true
					break
				} else {
					text.WriteByte(rest[0])
					rest = rest[1:]
				}
			}
			if !closed || (rest != "" && !unicode.IsSpace([]rune(rest)[0])) {
				return nil, fmt.Errorf("invalid --relax query: quoted phrases must be closed and whitespace-delimited")
			}
			term.text = text.String()
		} else {
			end := strings.IndexFunc(rest, unicode.IsSpace)
			if end < 0 {
				end = len(rest)
			}
			term.text, rest = rest[:end], rest[end:]
		}
		if strings.TrimSpace(term.text) == "" {
			return nil, fmt.Errorf("invalid --relax query: empty phrase or keep-term")
		}
		// FTS can ignore punctuation-only units inside AND expressions. They
		// must not count toward the core while adding no search constraint.
		if !strings.ContainsFunc(term.text, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			return nil, fmt.Errorf("invalid --relax query: unit %q has no searchable characters", term.text)
		}
		// Repeated spellings are one unit, not a way around the two-term floor.
		duplicate := false
		for i := range terms {
			if strings.EqualFold(terms[i].text, term.text) && terms[i].phrase == term.phrase {
				terms[i].protected = terms[i].protected || term.protected
				duplicate = true
				break
			}
		}
		if !duplicate {
			terms = append(terms, term)
		}
		if len(terms) > 32 {
			return nil, fmt.Errorf("invalid --relax query: at most 32 distinct query units are supported")
		}
	}
	if len(terms) == 0 {
		return nil, fmt.Errorf("invalid --relax query: at least one term is required")
	}
	return terms, nil
}

func recallExpression(terms []recallTerm, table string) string {
	parts := make([]string, len(terms))
	for i, term := range terms {
		parts[i] = `"` + strings.ReplaceAll(term.text, `"`, `""`) + `"`
		if table != "tool_fts" && !term.phrase {
			parts[i] += "*"
		}
	}
	return strings.Join(parts, " ")
}

// SearchRelaxed performs bounded, opt-in lexical recall. Each stage keeps all
// filters and requires every remaining unit. Unmarked strict queries use the
// existing sanitizer. Protected queries and drop stages bypass stopwords so
// protected terms and the retained core cannot vanish.
// Stemming/prefix expansion is already supplied by FTS; scope widening is never
// permitted. Only lowest-IDF term dropping is applicable after a zero-row stage.
func (d *Database) SearchRelaxed(query string, opts models.SearchOptions) ([]SearchResult, []string, error) {
	terms, err := parseRecallTerms(query)
	if err != nil {
		return nil, nil, err
	}
	stages := []string{"strict"}
	strictQuery := query
	for _, term := range terms {
		if term.protected {
			strictQuery = "" // protected units must bypass stopword removal
			break
		}
	}
	results, exists, err := d.recallStage(terms, opts, strictQuery)
	if err != nil || exists {
		return results, stages, err
	}

	// For a fixed corpus, IDF is monotone decreasing in document frequency.
	// Counting actual MATCH documents respects stemming, prefix and trigram
	// semantics rather than comparing raw words to Porter-stemmed vocab rows.
	type candidate struct {
		index int
		docs  int
	}
	var candidates []candidate
	for i, term := range terms {
		if !term.protected {
			candidates = append(candidates, candidate{index: i})
		}
	}
	if len(candidates) <= 2 {
		return results, stages, nil
	}
	for i := range candidates {
		docs, err := d.recallFrequency(terms[candidates[i].index], opts.ContentType)
		if err != nil {
			return nil, stages, err
		}
		candidates[i].docs = docs
	}
	// Equal IDF drops in original query order, not map or database row order.
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].docs > candidates[j].docs })
	dropped := make([]string, 0, len(candidates)-2)
	removed := make(map[int]bool)
	for _, candidate := range candidates[:len(candidates)-2] {
		removed[candidate.index] = true
		dropped = append(dropped, terms[candidate.index].text)
		var remaining []recallTerm
		for i, term := range terms {
			if !removed[i] {
				remaining = append(remaining, term)
			}
		}
		stages = append(stages, fmt.Sprintf("drop-terms/%d", len(dropped)))
		results, exists, err = d.recallStage(remaining, opts, "")
		if err != nil {
			return nil, stages, err
		}
		if exists {
			for i := range results {
				results[i].MatchStage = "drop-terms"
				results[i].DroppedTerms = append([]string(nil), dropped...)
			}
			return results, stages, nil
		}
	}
	return results, stages, nil
}

func (d *Database) recallStage(terms []recallTerm, opts models.SearchOptions, strictQuery string) ([]SearchResult, bool, error) {
	search := func(table string, page models.SearchOptions) ([]SearchResult, error) {
		if strictQuery != "" {
			return d.searchTable(table, strictQuery, page)
		}
		return d.searchTableQuery(table, recallExpression(terms, table), page)
	}
	results, err := searchTables(opts, search)
	if err != nil || len(results) > 0 || opts.Offset <= 0 {
		return results, len(results) > 0, err
	}
	// An exhausted page is not a zero-result query. Check before pagination;
	// do not widen a successful earlier stage to fill a later empty page.
	opts.Offset, opts.Limit = 0, 1
	first, err := searchTables(opts, search)
	return results, len(first) > 0, err
}

func (d *Database) recallFrequency(term recallTerm, contentType string) (int, error) {
	tables := []string{"messages_fts"}
	if contentType == "tool" {
		tables = []string{"tool_fts"}
	} else if contentType == "" {
		tables = append(tables, "tool_fts")
	}
	var selects []string
	var args []any
	for _, table := range tables {
		selects = append(selects, "SELECT rowid FROM "+table+" WHERE "+table+" MATCH ?")
		args = append(args, recallExpression([]recallTerm{term}, table))
	}
	matched := strings.Join(selects, " UNION ")
	// Explicit tool search keeps echo rows in the result page, so they remain
	// IDF documents. Unfiltered recall excludes them from both pages and DF.
	if contentType != "" {
		var count int
		err := d.db.QueryRow("SELECT COUNT(*) FROM ("+matched+")", args...).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("measure relaxation term frequency: %w", err)
		}
		return count, nil
	}
	var count int
	err := d.db.QueryRow(
		"SELECT COUNT(*) FROM ("+matched+") matched JOIN search_items si ON si.id = matched.rowid WHERE NOT ("+directBackscrollSearchEchoSQL("si")+")",
		args...,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("measure relaxation term frequency: %w", err)
	}
	// directBackscrollSearchEchoSQL cannot recognize the Codex shell wrapper
	// form: its argv is JSON-encoded and the separator byte-sequences are
	// unbounded for SQL GLOB. Subtract those rows with the same
	// broad-SQL-prefilter plus strict-Go-predicate split the requeue path
	// uses (see pendingSearchEchoShellMatches), so unfiltered IDF counts the
	// exact row set isDirectBackscrollSearchEcho keeps.
	shellRows, err := d.db.Query(
		"SELECT si.text FROM ("+matched+") matched JOIN search_items si ON si.id = matched.rowid WHERE si.content_type = 'tool' AND COALESCE(si.search_echo, 0) = 0 AND si.text LIKE 'shell %'",
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("measure relaxation term frequency: %w", err)
	}
	defer func() { _ = shellRows.Close() }()
	for shellRows.Next() {
		var text string
		if err := shellRows.Scan(&text); err != nil {
			return 0, fmt.Errorf("measure relaxation term frequency: %w", err)
		}
		if pendingSearchEchoShellMatches(text) {
			count--
		}
	}
	if err := shellRows.Err(); err != nil {
		return 0, fmt.Errorf("measure relaxation term frequency: %w", err)
	}
	return count, nil
}
