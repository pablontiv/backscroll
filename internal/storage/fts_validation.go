package storage

import "fmt"

// validateFTSMembership compares posting-backed document sets, not external-content
// scans (which reflect search_items even when postings are absent). All scratch
// objects are TEMP and rolled back; the persistent database is only read. Using
// SQLite's own tokenizers avoids treating empty prose or sub-trigram tool text as
// missing documents. This is deliberately separate from schema identity, recovery
// equality, accounting, and the selective rebuild's routing predicates.
func (d *Database) validateFTSMembership() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin FTS membership validation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, index := range []struct{ name, tokenizer, eligible string }{
		{"messages_fts", "porter unicode61", "content_type IN ('text','code','reasoning')"},
		{"tool_fts", "trigram", "content_type = 'tool'"},
	} {
		// Fixed names are safe: the transaction pins one connection and rollback
		// removes these objects before it is returned to the pool.
		statements := []string{
			fmt.Sprintf("CREATE VIRTUAL TABLE temp.validate_expected USING fts5(text, tokenize='%s')", index.tokenizer),
			fmt.Sprintf("INSERT INTO temp.validate_expected(rowid,text) SELECT id,text FROM main.search_items WHERE %s", index.eligible),
			"CREATE VIRTUAL TABLE temp.validate_expected_posts USING fts5vocab(temp, validate_expected, 'instance')",
			fmt.Sprintf("CREATE VIRTUAL TABLE temp.validate_actual_posts USING fts5vocab(main, %s, 'instance')", index.name),
		}
		for _, stmt := range statements {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("check %s FTS membership: %w", index.name, err)
			}
		}
		for _, difference := range []struct{ label, left, right string }{
			{"missing", "validate_expected_posts", "validate_actual_posts"},
			{"unexpected (wrong routing or orphan postings)", "validate_actual_posts", "validate_expected_posts"},
		} {
			var count int
			var first int64
			query := fmt.Sprintf("SELECT COUNT(*), COALESCE(MIN(doc),0) FROM (SELECT DISTINCT doc FROM temp.%s EXCEPT SELECT DISTINCT doc FROM temp.%s)", difference.left, difference.right)
			if err := tx.QueryRow(query).Scan(&count, &first); err != nil {
				return fmt.Errorf("check %s FTS membership: %w", index.name, err)
			}
			if count != 0 {
				return fmt.Errorf("%s FTS membership: %d %s documents (first search_items rowid=%d); run backscroll rebuild to restore selective routing", index.name, count, difference.label, first)
			}
		}
		for _, name := range []string{"validate_actual_posts", "validate_expected_posts", "validate_expected"} {
			if _, err := tx.Exec("DROP TABLE temp." + name); err != nil {
				return fmt.Errorf("finish %s FTS membership validation: %w", index.name, err)
			}
		}
	}
	return nil
}
