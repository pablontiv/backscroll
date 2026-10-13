package storage

import (
	"database/sql"
	"fmt"
)

// AggregateOptions filters and paginates pattern aggregations.
type AggregateOptions struct {
	Project     string // filter to single project; "" = all
	Tag         string // filter by session_tags.tag
	StartDate   string // inclusive RFC3339 lower bound; "" = unbounded
	EndDate     string // exclusive RFC3339 upper bound; "" = unbounded
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
// optionally stratified by tag and time window. NULL and empty command heads
// share the empty-string unknown group; no command is inferred.
func (d *Database) AggregateCommands(opts AggregateOptions) ([]CommandPattern, error) {
	cte, args := eligibleToolEvents(opts.Project, opts.Tag, opts.StartDate, opts.EndDate)
	query := cte + `SELECT tool_name, COALESCE(command_head, '') AS command_head, COUNT(*) as cnt
FROM eligible_tool_events
WHERE 1=1
`

	query += ` GROUP BY tool_name, COALESCE(command_head, '')
ORDER BY cnt DESC, tool_name, command_head
`

	query, args = appendPagination(query, args, opts.Limit, opts.Offset)

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
// NULL and empty command heads share the empty-string unknown group.
// NULL timestamps are excluded and the count is reported via a reference parameter.
func (d *Database) AggregateCommandsTrend(opts AggregateOptions, excludedPtr *int) ([]CommandPatternWeekly, error) {
	cte, args := eligibleToolEvents(opts.Project, opts.Tag, opts.StartDate, opts.EndDate)
	query := cte + `SELECT strftime('%Y-W%W', timestamp) AS week, tool_name, COALESCE(command_head, '') AS command_head, COUNT(*) as cnt
FROM eligible_tool_events
WHERE timestamp IS NOT NULL
`

	query += ` GROUP BY week, tool_name, COALESCE(command_head, '')
ORDER BY week DESC, cnt DESC, tool_name, command_head
`

	query, args = appendPagination(query, args, opts.Limit, opts.Offset)

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

	if excludedPtr != nil {
		if err := d.countUndatedPatternEvents(opts, false, excludedPtr); err != nil {
			return nil, err
		}
	}

	return results, nil
}

// AggregateFailures returns top (tool_name, is_error, exit_code) signatures for
// actual failures (is_error = 1), stratified by tag/time window. SignalledEvents
// reports the coverage denominator: the count of events with ANY non-NULL
// is_error signal, so callers can see how much of the corpus carries a signal.
func (d *Database) AggregateFailures(opts AggregateOptions) ([]FailurePattern, error) {
	cte, args := eligibleToolEvents(opts.Project, opts.Tag, opts.StartDate, opts.EndDate)
	var totalSignalled int
	if err := d.db.QueryRow(cte+`SELECT COUNT(*) FROM eligible_tool_events WHERE is_error IS NOT NULL`, args...).Scan(&totalSignalled); err != nil {
		return nil, fmt.Errorf("count signalled events: %w", err)
	}
	query := cte + `SELECT tool_name, is_error, exit_code, COUNT(*) as cnt
FROM eligible_tool_events
WHERE is_error = 1
`

	query += ` GROUP BY tool_name, is_error, exit_code
ORDER BY cnt DESC, tool_name, is_error DESC, exit_code
`

	query, args = appendPagination(query, args, opts.Limit, opts.Offset)

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
	cte, args := eligibleToolEvents(opts.Project, opts.Tag, opts.StartDate, opts.EndDate)
	var totalSignalled int
	if err := d.db.QueryRow(cte+`SELECT COUNT(*) FROM eligible_tool_events WHERE is_error IS NOT NULL AND timestamp IS NOT NULL`, args...).Scan(&totalSignalled); err != nil {
		return nil, fmt.Errorf("count signalled events trend: %w", err)
	}
	query := cte + `SELECT strftime('%Y-W%W', timestamp) AS week, tool_name, is_error, exit_code, COUNT(*) as cnt
FROM eligible_tool_events
WHERE is_error = 1 AND timestamp IS NOT NULL
`

	query += ` GROUP BY week, tool_name, is_error, exit_code
ORDER BY week DESC, cnt DESC, tool_name, is_error DESC, exit_code
`

	query, args = appendPagination(query, args, opts.Limit, opts.Offset)

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

	if excludedPtr != nil {
		if err := d.countUndatedPatternEvents(opts, true, excludedPtr); err != nil {
			return nil, err
		}
	}

	return results, nil
}

// Missing-timestamp diagnostics share identity and scope, but deliberately omit
// date bounds: NULL timestamps cannot be compared to either bound.
func (d *Database) countUndatedPatternEvents(opts AggregateOptions, failuresOnly bool, excluded *int) error {
	cte, args := eligibleToolEvents(opts.Project, opts.Tag, "", "")
	query := cte + `SELECT COUNT(*) FROM eligible_tool_events WHERE timestamp IS NULL`
	if failuresOnly {
		query += ` AND is_error = 1`
	}
	if err := d.db.QueryRow(query, args...).Scan(excluded); err != nil {
		return fmt.Errorf("count undated pattern events: %w", err)
	}
	return nil
}
