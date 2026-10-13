package storage

// appendPagination applies offsets to final rows, including unlimited pages.
func appendPagination(query string, args []interface{}, limit, offset int) (string, []interface{}) {
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	} else if offset > 0 {
		query += " LIMIT -1"
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}
	return query, args
}
