package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pablontiv/backscroll/internal/compat"
)

// Both external-content indexes store only text, keyed by search_items.id.
// Delete from the old route and insert into the new route independently so
// content-type moves (including unsupported types) preserve index membership.
const sqlV21 = `
DROP TRIGGER search_items_au_tool;
DROP TRIGGER search_items_au_msg;
CREATE TRIGGER search_items_au_tool AFTER UPDATE ON search_items
WHEN (old.id IS NOT new.id OR old.text IS NOT new.text OR old.content_type IS NOT new.content_type)
BEGIN
    INSERT INTO tool_fts(tool_fts, rowid, text)
    SELECT 'delete', old.id, old.text WHERE old.content_type = 'tool';
    INSERT INTO tool_fts(rowid, text)
    SELECT new.id, new.text WHERE new.content_type = 'tool';
END;
CREATE TRIGGER search_items_au_msg AFTER UPDATE ON search_items
WHEN (old.id IS NOT new.id OR old.text IS NOT new.text OR old.content_type IS NOT new.content_type)
BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text)
    SELECT 'delete', old.id, old.text WHERE old.content_type IN ('text','code','reasoning');
    INSERT INTO messages_fts(rowid, text)
    SELECT new.id, new.text WHERE new.content_type IN ('text','code','reasoning');
END;
CREATE INDEX idx_search_items_echo_pending ON search_items(source_path)
WHERE source = 'session' AND search_echo IS NULL;
CREATE INDEX idx_search_items_echo_zero ON search_items(source_path)
WHERE content_type = 'tool' AND COALESCE(search_echo,0) = 0 AND text LIKE '%backscroll%';
`

func applyV21(ctx context.Context, tx *sql.Tx, _ compat.SchemaShape) error {
	if _, err := tx.ExecContext(ctx, sqlV21); err != nil {
		return fmt.Errorf("apply v21 incremental FTS maintenance: %w", err)
	}
	return recordMigration(ctx, tx, 21, "V21 incremental FTS maintenance", sqlV21, "record migration v21")
}
