package storage

import (
	"database/sql"
	"fmt"
)

// AggregateOptions filters and paginates pattern aggregations.
type AggregateOptions struct {
	Project     string // filter to single project; "" = all
	Tag         string // filter by session_tags.tag
	StartDate   string // RFC3339; "" = no lower bound
	EndDate     string // RFC3339; "" = no upper bound
	Limit       int    // result limit; 0 = no limit
	Offset      int    // result offset
	TrendWeekly bool   // enable week bucketing (commands/failures only)
}

// CommandPattern represents a top (tool_name, command_head) pair.
type CommandPattern struct {
	ToolName    string `json:"tool_name"`
	CommandHead string `json:"command_head"`
	Count       int    `json:"count"`
}

// FailurePattern represents a (tool_name, is_error, exit_code) failure signature.
type FailurePattern struct {
	ToolName        string `json:"tool_name"`
	IsError         bool   `json:"is_error"`         // true = error; false = not-error (both counted as signals)
	ExitCode        *int   `json:"exit_code"`        // nullable
	Count           int    `json:"count"`            // count of this signature in result set
	SignalledEvents int    `json:"signalled_events"` // total count of events WITH is_error IS NOT NULL (coverage)
}

// CommandPatternWeekly represents a top (week, tool_name, command_head) tuple for trend output.
type CommandPatternWeekly struct {
	Week        string `json:"week"`
	ToolName    string `json:"tool_name"`
	CommandHead string `json:"command_head"`
	Count       int    `json:"count"`
}

// FailurePatternWeekly represents a (week, tool_name, is_error, exit_code) failure signature for trend output.
type FailurePatternWeekly struct {
	Week            string `json:"week"`
	ToolName        string `json:"tool_name"`
	IsError         bool   `json:"is_error"`
	ExitCode        *int   `json:"exit_code"`
	Count           int    `json:"count"`
	SignalledEvents int    `json:"signalled_events"`
}

// AggregateCommands returns top (tool_name, command_head) pairs by frequency,
// optionally stratified by tag and time window.
func (d *Database) AggregateCommands(opts AggregateOptions) ([]CommandPattern, error) {
	query := `
SELECT tool_name, command_head, COUNT(*) as cnt
FROM tool_events
WHERE 1=1
`
	var args []interface{}

	// Project filter
	if opts.Project != "" {
		query += ` AND tool_events.id IN (
			SELECT tool_events.id FROM tool_events
			JOIN search_items ON (
				tool_events.source_path = search_items.source_path
				AND tool_events.ordinal = search_items.ordinal
			)
			WHERE search_items.project = ?
		)`
		args = append(args, opts.Project)
	}

	// Tag filter
	if opts.Tag != "" {
		query += ` AND tool_events.source_path IN (
			SELECT DISTINCT search_items.source_path FROM search_items
			JOIN session_tags ON search_items.source_path = session_tags.source_path
			WHERE session_tags.tag = ?
		)`
		args = append(args, opts.Tag)
	}

	// Time window (start_date, end_date)
	if opts.StartDate != "" {
		query += ` AND tool_events.id IN (
			SELECT tool_events.id FROM tool_events
			JOIN search_items ON (
				tool_events.source_path = search_items.source_path
				AND tool_events.ordinal = search_items.ordinal
			)
			WHERE search_items.timestamp >= ?
		)`
		args = append(args, opts.StartDate)
	}
	if opts.EndDate != "" {
		query += ` AND tool_events.id IN (
			SELECT tool_events.id FROM tool_events
			JOIN search_items ON (
				tool_events.source_path = search_items.source_path
				AND tool_events.ordinal = search_items.ordinal
			)
			WHERE search_items.timestamp < ?
		)`
		args = append(args, opts.EndDate)
	}

	query += ` GROUP BY tool_name, command_head
ORDER BY cnt DESC, tool_name, command_head
`

	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, opts.Offset)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query commands: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []CommandPattern
	for rows.Next() {
		var p CommandPattern
		if err := rows.Scan(&p.ToolName, &p.CommandHead, &p.Count); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		results = append(results, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return results, nil
}

// AggregateCommandsTrend returns (week, tool_name, command_head) aggregations grouped by week.
// NULL timestamps are excluded and the count is reported via a reference parameter.
func (d *Database) AggregateCommandsTrend(opts AggregateOptions, excludedPtr *int) ([]CommandPatternWeekly, error) {
	query := `
SELECT strftime('%Y-W%W', si.timestamp) AS week, tool_name, command_head, COUNT(*) as cnt
FROM tool_events
JOIN search_items si ON (
	tool_events.source_path = si.source_path
	AND tool_events.ordinal = si.ordinal
)
WHERE si.timestamp IS NOT NULL
`
	var args []interface{}

	// Project filter
	if opts.Project != "" {
		query += ` AND si.project = ?`
		args = append(args, opts.Project)
	}

	// Tag filter
	if opts.Tag != "" {
		query += ` AND tool_events.source_path IN (
			SELECT DISTINCT search_items.source_path FROM search_items
			JOIN session_tags ON search_items.source_path = session_tags.source_path
			WHERE session_tags.tag = ?
		)`
		args = append(args, opts.Tag)
	}

	// Time window (start_date, end_date)
	if opts.StartDate != "" {
		query += ` AND si.timestamp >= ?`
		args = append(args, opts.StartDate)
	}
	if opts.EndDate != "" {
		query += ` AND si.timestamp < ?`
		args = append(args, opts.EndDate)
	}

	query += ` GROUP BY week, tool_name, command_head
ORDER BY week DESC, cnt DESC, tool_name, command_head
`

	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, opts.Offset)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query commands trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []CommandPatternWeekly
	for rows.Next() {
		var p CommandPatternWeekly
		var week sql.NullString
		if err := rows.Scan(&week, &p.ToolName, &p.CommandHead, &p.Count); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		if week.Valid {
			p.Week = week.String
		}
		results = append(results, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	// Count excluded NULL-timestamp rows
	// Note: date filters (StartDate, EndDate) do not apply here because NULL timestamps
	// cannot be compared (NULL >= date yields UNKNOWN, not true). This count reflects
	// rows filtered out due to missing timestamp only.
	if excludedPtr != nil {
		query := `SELECT COUNT(*) FROM tool_events
JOIN search_items si ON (tool_events.source_path = si.source_path AND tool_events.ordinal = si.ordinal)
WHERE si.timestamp IS NULL`
		var args []interface{}
		if opts.Project != "" {
			query += ` AND si.project = ?`
			args = append(args, opts.Project)
		}
		if opts.Tag != "" {
			query += ` AND tool_events.source_path IN (
				SELECT DISTINCT search_items.source_path FROM search_items
				JOIN session_tags ON search_items.source_path = session_tags.source_path
				WHERE session_tags.tag = ?
			)`
			args = append(args, opts.Tag)
		}
		_ = d.db.QueryRow(query, args...).Scan(excludedPtr)
	}

	return results, nil
}

// AggregateFailures returns top (tool_name, is_error, exit_code) signatures for
// actual failures (is_error = 1), stratified by tag/time window. SignalledEvents
// reports the coverage denominator: the count of events with ANY non-NULL
// is_error signal, so callers can see how much of the corpus carries a signal.
func (d *Database) AggregateFailures(opts AggregateOptions) ([]FailurePattern, error) {
	// First: count total signalled events (coverage denominator)
	coverageQuery := `
SELECT COUNT(*) FROM tool_events
WHERE is_error IS NOT NULL
`
	var coverageArgs []interface{}

	if opts.Project != "" {
		coverageQuery += ` AND tool_events.id IN (
			SELECT tool_events.id FROM tool_events
			JOIN search_items ON (
				tool_events.source_path = search_items.source_path
				AND tool_events.ordinal = search_items.ordinal
			)
			WHERE search_items.project = ?
		)`
		coverageArgs = append(coverageArgs, opts.Project)
	}

	if opts.Tag != "" {
		coverageQuery += ` AND tool_events.source_path IN (
			SELECT DISTINCT search_items.source_path FROM search_items
			JOIN session_tags ON search_items.source_path = session_tags.source_path
			WHERE session_tags.tag = ?
		)`
		coverageArgs = append(coverageArgs, opts.Tag)
	}

	if opts.StartDate != "" {
		coverageQuery += ` AND tool_events.id IN (
			SELECT tool_events.id FROM tool_events
			JOIN search_items ON (
				tool_events.source_path = search_items.source_path
				AND tool_events.ordinal = search_items.ordinal
			)
			WHERE search_items.timestamp >= ?
		)`
		coverageArgs = append(coverageArgs, opts.StartDate)
	}
	if opts.EndDate != "" {
		coverageQuery += ` AND tool_events.id IN (
			SELECT tool_events.id FROM tool_events
			JOIN search_items ON (
				tool_events.source_path = search_items.source_path
				AND tool_events.ordinal = search_items.ordinal
			)
			WHERE search_items.timestamp < ?
		)`
		coverageArgs = append(coverageArgs, opts.EndDate)
	}

	var totalSignalled int
	if err := d.db.QueryRow(coverageQuery, coverageArgs...).Scan(&totalSignalled); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("count signalled events: %w", err)
	}

	// Main aggregation: group by signature (only actual failures, not successes)
	query := `
SELECT tool_name, is_error, exit_code, COUNT(*) as cnt
FROM tool_events
WHERE is_error = 1
`
	var args []interface{}

	if opts.Project != "" {
		query += ` AND tool_events.id IN (
			SELECT tool_events.id FROM tool_events
			JOIN search_items ON (
				tool_events.source_path = search_items.source_path
				AND tool_events.ordinal = search_items.ordinal
			)
			WHERE search_items.project = ?
		)`
		args = append(args, opts.Project)
	}

	if opts.Tag != "" {
		query += ` AND tool_events.source_path IN (
			SELECT DISTINCT search_items.source_path FROM search_items
			JOIN session_tags ON search_items.source_path = session_tags.source_path
			WHERE session_tags.tag = ?
		)`
		args = append(args, opts.Tag)
	}

	if opts.StartDate != "" {
		query += ` AND tool_events.id IN (
			SELECT tool_events.id FROM tool_events
			JOIN search_items ON (
				tool_events.source_path = search_items.source_path
				AND tool_events.ordinal = search_items.ordinal
			)
			WHERE search_items.timestamp >= ?
		)`
		args = append(args, opts.StartDate)
	}
	if opts.EndDate != "" {
		query += ` AND tool_events.id IN (
			SELECT tool_events.id FROM tool_events
			JOIN search_items ON (
				tool_events.source_path = search_items.source_path
				AND tool_events.ordinal = search_items.ordinal
			)
			WHERE search_items.timestamp < ?
		)`
		args = append(args, opts.EndDate)
	}

	query += ` GROUP BY tool_name, is_error, exit_code
ORDER BY cnt DESC, tool_name, is_error DESC, exit_code
`

	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, opts.Offset)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failures: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []FailurePattern
	for rows.Next() {
		var toolName string
		var isError int
		var exitCode sql.NullInt64
		var count int
		if err := rows.Scan(&toolName, &isError, &exitCode, &count); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		var ec *int
		if exitCode.Valid {
			v := int(exitCode.Int64)
			ec = &v
		}
		results = append(results, FailurePattern{
			ToolName:        toolName,
			IsError:         isError != 0,
			ExitCode:        ec,
			Count:           count,
			SignalledEvents: totalSignalled,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return results, nil
}

// AggregateFailuresTrend returns (week, tool_name, is_error, exit_code) failure signatures grouped by week.
// NULL timestamps are excluded and the count is reported via a reference parameter.
func (d *Database) AggregateFailuresTrend(opts AggregateOptions, excludedPtr *int) ([]FailurePatternWeekly, error) {
	// First: count total signalled events per week (coverage denominator)
	coverageQuery := `
SELECT COUNT(*) FROM tool_events
JOIN search_items si ON (
	tool_events.source_path = si.source_path
	AND tool_events.ordinal = si.ordinal
)
WHERE is_error IS NOT NULL AND si.timestamp IS NOT NULL
`
	var coverageArgs []interface{}

	if opts.Project != "" {
		coverageQuery += ` AND si.project = ?`
		coverageArgs = append(coverageArgs, opts.Project)
	}

	if opts.Tag != "" {
		coverageQuery += ` AND tool_events.source_path IN (
			SELECT DISTINCT search_items.source_path FROM search_items
			JOIN session_tags ON search_items.source_path = session_tags.source_path
			WHERE session_tags.tag = ?
		)`
		coverageArgs = append(coverageArgs, opts.Tag)
	}

	if opts.StartDate != "" {
		coverageQuery += ` AND si.timestamp >= ?`
		coverageArgs = append(coverageArgs, opts.StartDate)
	}
	if opts.EndDate != "" {
		coverageQuery += ` AND si.timestamp < ?`
		coverageArgs = append(coverageArgs, opts.EndDate)
	}

	var totalSignalled int
	if err := d.db.QueryRow(coverageQuery, coverageArgs...).Scan(&totalSignalled); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("count signalled events trend: %w", err)
	}

	// Main aggregation: group by week + signature (only actual failures)
	query := `
SELECT strftime('%Y-W%W', si.timestamp) AS week, tool_name, is_error, exit_code, COUNT(*) as cnt
FROM tool_events
JOIN search_items si ON (
	tool_events.source_path = si.source_path
	AND tool_events.ordinal = si.ordinal
)
WHERE is_error = 1 AND si.timestamp IS NOT NULL
`
	var args []interface{}

	if opts.Project != "" {
		query += ` AND si.project = ?`
		args = append(args, opts.Project)
	}

	if opts.Tag != "" {
		query += ` AND tool_events.source_path IN (
			SELECT DISTINCT search_items.source_path FROM search_items
			JOIN session_tags ON search_items.source_path = session_tags.source_path
			WHERE session_tags.tag = ?
		)`
		args = append(args, opts.Tag)
	}

	if opts.StartDate != "" {
		query += ` AND si.timestamp >= ?`
		args = append(args, opts.StartDate)
	}
	if opts.EndDate != "" {
		query += ` AND si.timestamp < ?`
		args = append(args, opts.EndDate)
	}

	query += ` GROUP BY week, tool_name, is_error, exit_code
ORDER BY week DESC, cnt DESC, tool_name, is_error DESC, exit_code
`

	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, opts.Offset)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failures trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []FailurePatternWeekly
	for rows.Next() {
		var week sql.NullString
		var toolName string
		var isError int
		var exitCode sql.NullInt64
		var count int
		if err := rows.Scan(&week, &toolName, &isError, &exitCode, &count); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		var ec *int
		if exitCode.Valid {
			v := int(exitCode.Int64)
			ec = &v
		}
		p := FailurePatternWeekly{
			ToolName:        toolName,
			IsError:         isError != 0,
			ExitCode:        ec,
			Count:           count,
			SignalledEvents: totalSignalled,
		}
		if week.Valid {
			p.Week = week.String
		}
		results = append(results, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	// Count excluded NULL-timestamp rows (only is_error=1, matching main query)
	// Note: date filters (StartDate, EndDate) do not apply here because NULL timestamps
	// cannot be compared (NULL >= date yields UNKNOWN, not true). This count reflects
	// rows filtered out due to missing timestamp only.
	if excludedPtr != nil {
		query := `SELECT COUNT(*) FROM tool_events
JOIN search_items si ON (tool_events.source_path = si.source_path AND tool_events.ordinal = si.ordinal)
WHERE si.timestamp IS NULL AND is_error = 1`
		var args []interface{}
		if opts.Project != "" {
			query += ` AND si.project = ?`
			args = append(args, opts.Project)
		}
		if opts.Tag != "" {
			query += ` AND tool_events.source_path IN (
				SELECT DISTINCT search_items.source_path FROM search_items
				JOIN session_tags ON search_items.source_path = session_tags.source_path
				WHERE session_tags.tag = ?
			)`
			args = append(args, opts.Tag)
		}
		_ = d.db.QueryRow(query, args...).Scan(excludedPtr)
	}

	return results, nil
}
