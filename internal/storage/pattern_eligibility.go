package storage

// patternItemID resolves a satellite to at most one indexed item, before scope
// filters. Proven UUIDs never fall back to coordinates, even when their source
// is unavailable. UUID-less satellites use coordinates only when unambiguous.
// All arguments are internal SQL identifiers, never caller-provided values.
func patternItemID(uuid, path, ordinal string) string {
	return `CASE WHEN NULLIF(` + uuid + `, '') IS NOT NULL THEN
  (SELECT id FROM search_items WHERE uuid = ` + uuid + `)
 ELSE
  (SELECT MIN(id) FROM search_items WHERE source_path = ` + path + `
   AND ordinal = ` + ordinal + ` HAVING COUNT(*) = 1)
 END`
}

// patternEligibility applies conjunctive scope to the resolved item (alias si).
// Dates follow search's half-open interval: after inclusive, before exclusive.
// EXISTS avoids multiplying events when a session has several tags.
func patternEligibility(project, tag, after, before string) (string, []interface{}) {
	var query string
	var args []interface{}
	if project != "" {
		query += ` AND si.project = ?`
		args = append(args, project)
	}
	if tag != "" {
		query += ` AND EXISTS (SELECT 1 FROM scoped_session_tags st WHERE st.source_path = si.source_path AND st.session_id IS si.session_id AND st.tag = ?)`
		args = append(args, tag)
	}
	if after != "" {
		query += ` AND si.timestamp >= ?`
		args = append(args, after)
	}
	if before != "" {
		query += ` AND si.timestamp < ?`
		args = append(args, before)
	}
	return query, args
}

// eligibleToolEvents is shared by flat/trend counts, their coverage denominators,
// missing-timestamp diagnostics, and sequence loading. Unscoped flat consumers
// retain orphan events; these cannot prove scope or contribute dated buckets.
func eligibleToolEvents(project, tag, after, before string) (string, []interface{}) {
	filter, args := patternEligibility(project, tag, after, before)
	return `WITH eligible_tool_events AS (
 SELECT te.*, si.timestamp, COALESCE(si.session_id, '') AS session_identity,
 COALESCE(si.source_path, te.source_path) AS session_path,
 COALESCE(si.ordinal, te.ordinal) AS session_ordinal
 FROM tool_events te
 LEFT JOIN search_items si ON si.id = ` + patternItemID("te.message_uuid", "te.source_path", "te.ordinal") + `
 WHERE 1=1` + filter + `)
 `, args
}
