package storage

import (
	"crypto/sha256"
	"fmt"
)

// SetupSchema creates the database schema if it doesn't already exist.
// It idempotently applies all migrations using the schema_migrations table.
func (d *Database) SetupSchema() error {
	// Create the schema_migrations table if it doesn't exist
	if _, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_on TEXT NOT NULL,
			checksum TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Check if version 1 is already applied
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version: %w", err)
	}

	if count == 0 {
		// Version 1 not applied, so apply it
		if err := d.applyV1Migration(); err != nil {
			return err
		}
	}

	// Check if version 2 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 2").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 2: %w", err)
	}

	if count == 0 {
		if err := d.applyV2Migration(); err != nil {
			return err
		}
	}

	// Check if version 3 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 3").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 3: %w", err)
	}

	if count == 0 {
		if err := d.applyV3Migration(); err != nil {
			return err
		}
	}

	// Check if version 4 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 4").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 4: %w", err)
	}

	if count == 0 {
		if err := d.applyV4Migration(); err != nil {
			return err
		}
	}

	// Check if version 5 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 5").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 5: %w", err)
	}

	if count == 0 {
		if err := d.applyV5Migration(); err != nil {
			return err
		}
	}

	// Check if version 6 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 6").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 6: %w", err)
	}

	if count == 0 {
		if err := d.applyV6Migration(); err != nil {
			return err
		}
	}

	// Check if version 7 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 7").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 7: %w", err)
	}

	if count == 0 {
		if err := d.applyV7Migration(); err != nil {
			return err
		}
	}

	// Check if version 8 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 8").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 8: %w", err)
	}

	if count == 0 {
		if err := d.applyV8Migration(); err != nil {
			return err
		}
	}

	// Check if version 9 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 9").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 9: %w", err)
	}

	if count == 0 {
		if err := d.applyV9Migration(); err != nil {
			return err
		}
	}

	// Check if version 10 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 10").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 10: %w", err)
	}

	if count == 0 {
		if err := d.applyV10Migration(); err != nil {
			return err
		}
	}

	// Check if version 11 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 11").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 11: %w", err)
	}

	if count == 0 {
		if err := d.applyV11Migration(); err != nil {
			return err
		}
	}

	// Check if version 12 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 12").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 12: %w", err)
	}

	if count == 0 {
		if err := d.applyV12Migration(); err != nil {
			return err
		}
	}

	// Check if version 13 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 13").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 13: %w", err)
	}

	if count == 0 {
		if err := d.applyV13Migration(); err != nil {
			return err
		}
	}

	return nil
}

// applyV1Migration applies version 1 of the schema (all core tables).
func (d *Database) applyV1Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Define the core DDL for this migration (for checksum)
	coreDDL := sqlV1CoreDDL

	// Execute all CREATE TABLE statements (idempotent with IF NOT EXISTS)
	if _, err := tx.Exec(sqlV1Core); err != nil {
		return fmt.Errorf("create core tables: %w", err)
	}

	// Create FTS5 virtual table and vocab view (idempotent)
	if _, err := tx.Exec(sqlV1FTS5); err != nil {
		return fmt.Errorf("create FTS5 virtual table: %w", err)
	}

	// Create triggers (idempotent with IF NOT EXISTS)
	if _, err := tx.Exec(sqlV1Triggers); err != nil {
		return fmt.Errorf("create triggers: %w", err)
	}

	// Compute checksum of the core DDL
	checksum := sha256.Sum256([]byte(coreDDL))
	checksumHex := fmt.Sprintf("%x", checksum)

	// Record migration as applied
	_, err = tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (1, 'V1 core schema', CURRENT_TIMESTAMP, ?)
	`, checksumHex)
	if err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	return nil
}

// applyV2Migration adds tables for the embedding system: chunks and embedding_metadata.
func (d *Database) applyV2Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(sqlV2); err != nil {
		return fmt.Errorf("create embedding tables: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV2))
	checksumHex := fmt.Sprintf("%x", checksum)

	_, err = tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (2, 'V2 embedding tables', CURRENT_TIMESTAMP, ?)
	`, checksumHex)
	if err != nil {
		return fmt.Errorf("record migration v2: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration v2: %w", err)
	}

	return nil
}

// applyV3Migration adds an embedding BLOB column to chunks for pure-Go vector search.
// Decision (T039): sqlite-vec requires CGO; we store embedding bytes directly in chunks
// and perform cosine similarity in Go (linear scan).
func (d *Database) applyV3Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(sqlV3); err != nil {
		return fmt.Errorf("add embedding column: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV3))
	checksumHex := fmt.Sprintf("%x", checksum)

	_, err = tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (3, 'V3 embedding blob column', CURRENT_TIMESTAMP, ?)
	`, checksumHex)
	if err != nil {
		return fmt.Errorf("record migration v3: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration v3: %w", err)
	}

	return nil
}

// applyV4Migration adds the tool_fts index (trigram tokenizer), branches the
// sync triggers by content_type, and repopulates both indexes from search_items.
func (d *Database) applyV4Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(sqlV4ToolFTS); err != nil {
		return fmt.Errorf("create tool_fts: %w", err)
	}
	if _, err := tx.Exec(sqlV4Triggers); err != nil {
		return fmt.Errorf("rebuild triggers: %w", err)
	}
	if _, err := tx.Exec(sqlV4Repopulate); err != nil {
		return fmt.Errorf("repopulate indexes: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV4ToolFTS + sqlV4Triggers))
	checksumHex := fmt.Sprintf("%x", checksum)

	_, err = tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (4, 'V4 tool_fts trigram index', CURRENT_TIMESTAMP, ?)
	`, checksumHex)
	if err != nil {
		return fmt.Errorf("record migration v4: %w", err)
	}

	return tx.Commit()
}

// applyV5Migration drops the phantom session_events table. Nothing reads or
// writes it after the structured-stats surface was removed (stats command,
// structured list filters, and the session_events query/insert paths are gone).
// Per the schema rule this is a new migration; V1 still creates the table on
// the way up, and V5 drops it.
func (d *Database) applyV5Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const sqlV5Drop = `
DROP INDEX IF EXISTS idx_session_events_order;
DROP INDEX IF EXISTS idx_session_events_project;
DROP TABLE IF EXISTS session_events;
`
	if _, err := tx.Exec(sqlV5Drop); err != nil {
		return fmt.Errorf("drop session_events: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV5Drop))
	checksumHex := fmt.Sprintf("%x", checksum)

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (5, 'V5 drop phantom session_events', CURRENT_TIMESTAMP, ?)
	`, checksumHex); err != nil {
		return fmt.Errorf("record migration v5: %w", err)
	}

	return tx.Commit()
}

// applyV6Migration drops the phantom source_metadata column. Nothing reads or
// writes it (no production callers, no SELECT access anywhere).
// Per the schema rule this is a new migration; V1 still creates the column on
// the way up, and V6 drops it.
func (d *Database) applyV6Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const sqlV6Drop = `ALTER TABLE search_items DROP COLUMN source_metadata;`

	if _, err := tx.Exec(sqlV6Drop); err != nil {
		return fmt.Errorf("drop source_metadata column: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV6Drop))
	checksumHex := fmt.Sprintf("%x", checksum)

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (6, 'V6 drop phantom source_metadata column', CURRENT_TIMESTAMP, ?)
	`, checksumHex); err != nil {
		return fmt.Errorf("record migration v6: %w", err)
	}

	return tx.Commit()
}

// applyV7Migration updates the content_type-branched triggers to support reasoning
// indexing. Reasoning blocks (content_type='reasoning') route to messages_fts
// alongside 'text' and 'code', NOT to tool_fts. This preserves the v4 semantic:
// tool_fts is for structured tool metadata (names, paths, commands); messages_fts
// is for prose (text, code, reasoning).
func (d *Database) applyV7Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(sqlV7Triggers); err != nil {
		return fmt.Errorf("rebuild triggers for reasoning: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV7Triggers))
	checksumHex := fmt.Sprintf("%x", checksum)

	_, err = tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (7, 'V7 reasoning content_type routes to messages_fts', CURRENT_TIMESTAMP, ?)
	`, checksumHex)
	if err != nil {
		return fmt.Errorf("record migration v7: %w", err)
	}

	return tx.Commit()
}

// SQL schema strings

const sqlV1Core = `
CREATE TABLE IF NOT EXISTS indexed_files (
    path TEXT PRIMARY KEY,
    hash TEXT NOT NULL,
    last_indexed DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS search_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL DEFAULT 'session',
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    role TEXT NOT NULL,
    text TEXT NOT NULL,
    timestamp TEXT,
    uuid TEXT UNIQUE,
    project TEXT,
    content_type TEXT NOT NULL DEFAULT 'text',
    source_metadata TEXT DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_search_items_source_path ON search_items(source_path);
CREATE INDEX IF NOT EXISTS idx_search_items_project ON search_items(project);

CREATE TABLE IF NOT EXISTS session_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    schema_version INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'session',
    source_path TEXT NOT NULL,
    project TEXT,
    ordinal INTEGER NOT NULL,
    timestamp TEXT,
    event_type TEXT NOT NULL,
    actor TEXT,
    role TEXT,
    tool_name TEXT,
    tool_id TEXT,
    command TEXT,
    cwd TEXT,
    exit_code INTEGER,
    is_error INTEGER,
    snippet TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_events_order ON session_events(source_path, ordinal, timestamp, id);
CREATE INDEX IF NOT EXISTS idx_session_events_project ON session_events(project);

CREATE TABLE IF NOT EXISTS session_tags (
    source_path TEXT NOT NULL,
    tag TEXT NOT NULL,
    PRIMARY KEY (source_path, tag)
);

CREATE INDEX IF NOT EXISTS idx_session_tags_tag ON session_tags(tag);

CREATE TABLE IF NOT EXISTS dynamic_stopwords (term TEXT PRIMARY KEY);
`

const sqlV1FTS5 = `
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    text,
    content=search_items,
    content_rowid=id,
    tokenize='porter unicode61'
);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_vocab USING fts5vocab(messages_fts, 'row');
`

const sqlV1Triggers = `
CREATE TRIGGER IF NOT EXISTS search_items_ai AFTER INSERT ON search_items BEGIN
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ad AFTER DELETE ON search_items BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_au AFTER UPDATE ON search_items BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;
`

const sqlV2 = `
CREATE TABLE IF NOT EXISTS chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id TEXT NOT NULL,
    chunk_idx INTEGER NOT NULL,
    content TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(source_id, chunk_idx)
);

CREATE INDEX IF NOT EXISTS idx_chunks_source_id ON chunks (source_id);

CREATE TABLE IF NOT EXISTS embedding_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_id INTEGER NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    model_name TEXT NOT NULL,
    model_version TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
`

// sqlV3 adds an embedding BLOB column to chunks for pure-Go cosine similarity search.
// This replaces the sqlite-vec virtual table approach (which requires CGO).
const sqlV3 = `ALTER TABLE chunks ADD COLUMN embedding BLOB;`

const sqlV4ToolFTS = `
CREATE VIRTUAL TABLE IF NOT EXISTS tool_fts USING fts5(
    text,
    content=search_items,
    content_rowid=id,
    tokenize='trigram'
);

CREATE VIRTUAL TABLE IF NOT EXISTS tool_vocab USING fts5vocab(tool_fts, 'row');
`

// Drop the unconditional v1 triggers and replace them with content_type-branched
// triggers: tool rows index into tool_fts, everything else into messages_fts.
const sqlV4Triggers = `
DROP TRIGGER IF EXISTS search_items_ai;
DROP TRIGGER IF EXISTS search_items_ad;
DROP TRIGGER IF EXISTS search_items_au;

-- NOTE: content_type is immutable per row (set at sync time; re-sync deletes and re-inserts).
-- The UPDATE triggers (search_items_au_tool, search_items_au_msg) intentionally branch on old.content_type
-- and do not handle cross-type transitions, since content_type never changes for existing rows.

CREATE TRIGGER IF NOT EXISTS search_items_ai_tool AFTER INSERT ON search_items
WHEN new.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ai_msg AFTER INSERT ON search_items
WHEN new.content_type <> 'tool' BEGIN
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ad_tool AFTER DELETE ON search_items
WHEN old.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(tool_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ad_msg AFTER DELETE ON search_items
WHEN old.content_type <> 'tool' BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_au_tool AFTER UPDATE ON search_items
WHEN old.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(tool_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO tool_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_au_msg AFTER UPDATE ON search_items
WHEN old.content_type <> 'tool' BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;
`

// Repopulate both indexes from search_items by content_type. 'delete-all' is valid
// for external-content FTS5 tables and resets the index without touching content rows.
const sqlV4Repopulate = `
INSERT INTO messages_fts(messages_fts) VALUES('delete-all');
INSERT INTO messages_fts(rowid, text) SELECT id, text FROM search_items WHERE content_type <> 'tool';
INSERT INTO tool_fts(rowid, text) SELECT id, text FROM search_items WHERE content_type = 'tool';
`

// sqlV7Triggers updates the v4 branched triggers to support reasoning content_type.
// The semantic is: tool-specific content (content_type='tool') indexes into tool_fts
// (trigram, substring matching); prose content (text, code, reasoning) indexes into
// messages_fts (porter, morphological matching). This preserves the v4 split while
// extending it for reasoning blocks.
const sqlV7Triggers = `
DROP TRIGGER IF EXISTS search_items_ai_tool;
DROP TRIGGER IF EXISTS search_items_ai_msg;
DROP TRIGGER IF EXISTS search_items_ad_tool;
DROP TRIGGER IF EXISTS search_items_ad_msg;
DROP TRIGGER IF EXISTS search_items_au_tool;
DROP TRIGGER IF EXISTS search_items_au_msg;

-- NOTE: content_type is immutable per row (set at sync time; re-sync deletes and re-inserts).
-- The UPDATE triggers branch on old.content_type and do not handle cross-type transitions.

CREATE TRIGGER IF NOT EXISTS search_items_ai_tool AFTER INSERT ON search_items
WHEN new.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ai_msg AFTER INSERT ON search_items
WHEN new.content_type IN ('text', 'code', 'reasoning') BEGIN
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ad_tool AFTER DELETE ON search_items
WHEN old.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(tool_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ad_msg AFTER DELETE ON search_items
WHEN old.content_type IN ('text', 'code', 'reasoning') BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_au_tool AFTER UPDATE ON search_items
WHEN old.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(tool_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO tool_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_au_msg AFTER UPDATE ON search_items
WHEN old.content_type IN ('text', 'code', 'reasoning') BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;
`

// sqlV1CoreDDL is the core DDL string used for computing the migration checksum.
// This must match the Rust version's SQL_V1 for compatibility.
const sqlV1CoreDDL = `
CREATE TABLE IF NOT EXISTS indexed_files (
    path TEXT PRIMARY KEY,
    hash TEXT NOT NULL,
    last_indexed DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS search_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL DEFAULT 'session',
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    role TEXT NOT NULL,
    text TEXT NOT NULL,
    timestamp TEXT,
    uuid TEXT UNIQUE,
    project TEXT,
    content_type TEXT NOT NULL DEFAULT 'text',
    source_metadata TEXT DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_search_items_source_path ON search_items(source_path);
CREATE INDEX IF NOT EXISTS idx_search_items_project ON search_items(project);

CREATE TABLE IF NOT EXISTS session_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    schema_version INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'session',
    source_path TEXT NOT NULL,
    project TEXT,
    ordinal INTEGER NOT NULL,
    timestamp TEXT,
    event_type TEXT NOT NULL,
    actor TEXT,
    role TEXT,
    tool_name TEXT,
    tool_id TEXT,
    command TEXT,
    cwd TEXT,
    exit_code INTEGER,
    is_error INTEGER,
    snippet TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_events_order ON session_events(source_path, ordinal, timestamp, id);
CREATE INDEX IF NOT EXISTS idx_session_events_project ON session_events(project);

CREATE TABLE IF NOT EXISTS session_tags (
    source_path TEXT NOT NULL,
    tag TEXT NOT NULL,
    PRIMARY KEY (source_path, tag)
);

CREATE INDEX IF NOT EXISTS idx_session_tags_tag ON session_tags(tag);

CREATE TABLE IF NOT EXISTS dynamic_stopwords (term TEXT PRIMARY KEY);
`

// applyV8Migration adds the F0 perennity surface: extraction_version and
// was_interrupted on search_items, and the perennial tool_events satellite
// table (one row per tool_use, anchored by message identity). tool_events is
// NOT re-derivable once source files expire — no CASCADE lifecycle; only
// purge deletes from it, explicitly.
func (d *Database) applyV8Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const sqlV8 = `
ALTER TABLE search_items ADD COLUMN extraction_version INTEGER;
ALTER TABLE search_items ADD COLUMN was_interrupted INTEGER;
CREATE TABLE IF NOT EXISTS tool_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_uuid TEXT,
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    tool_name TEXT NOT NULL,
    command_head TEXT,
    is_error INTEGER,
    exit_code INTEGER,
    extraction_version INTEGER NOT NULL,
    UNIQUE(source_path, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_tool_events_tool ON tool_events(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_events_uuid ON tool_events(message_uuid);
`
	if _, err := tx.Exec(sqlV8); err != nil {
		return fmt.Errorf("apply v8 perennity schema: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV8))
	checksumHex := fmt.Sprintf("%x", checksum)

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (8, 'V8 perennity: extraction_version, was_interrupted, tool_events', CURRENT_TIMESTAMP, ?)
	`, checksumHex); err != nil {
		return fmt.Errorf("record migration v8: %w", err)
	}

	return tx.Commit()
}

func (d *Database) applyV9Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Dedupe before indexing: v8 code could accumulate the same message_uuid
	// at different ordinals (ordinal drift) or across files; creating the
	// unique index on such data would fail on every startup. Keep the oldest
	// row per uuid.
	const sqlV9 = `
DELETE FROM tool_events WHERE message_uuid IS NOT NULL AND id NOT IN (
    SELECT MIN(id) FROM tool_events WHERE message_uuid IS NOT NULL GROUP BY message_uuid
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_events_uuid_unique ON tool_events(message_uuid) WHERE message_uuid IS NOT NULL;
`
	if _, err := tx.Exec(sqlV9); err != nil {
		return fmt.Errorf("apply v9 tool_events uuid uniqueness: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV9))
	checksumHex := fmt.Sprintf("%x", checksum)

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (9, 'V9 tool_events uuid uniqueness index', CURRENT_TIMESTAMP, ?)
	`, checksumHex); err != nil {
		return fmt.Errorf("record migration v9: %w", err)
	}

	return tx.Commit()
}

// applyV10Migration adds F2 template mining surface: message_templates
// (derived, rebuildable) and template_matches (perennial join, anchors
// templates to search_items via ordinal + source_path, with UNIQUE to stay
// idempotent under re-sync). Only templates with occurrence_count >= 3
// (configurable) are reported; mining runs inside SyncFiles tx.
func (d *Database) applyV10Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const sqlV10 = `
CREATE TABLE IF NOT EXISTS message_templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    signature TEXT UNIQUE NOT NULL,
    normalization_version INTEGER NOT NULL,
    template_text TEXT NOT NULL,
    occurrence_count INTEGER NOT NULL DEFAULT 1,
    first_seen TEXT,
    last_seen TEXT
);
CREATE INDEX IF NOT EXISTS idx_templates_sig ON message_templates(signature);
CREATE INDEX IF NOT EXISTS idx_templates_version ON message_templates(normalization_version);

CREATE TABLE IF NOT EXISTS template_matches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id INTEGER NOT NULL,
    item_uuid TEXT,
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    UNIQUE(source_path, ordinal, template_id),
    FOREIGN KEY(template_id) REFERENCES message_templates(id)
);
CREATE INDEX IF NOT EXISTS idx_matches_template ON template_matches(template_id);
CREATE INDEX IF NOT EXISTS idx_matches_uuid ON template_matches(item_uuid);
`
	if _, err := tx.Exec(sqlV10); err != nil {
		return fmt.Errorf("apply v10 template-mining schema: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV10))
	checksumHex := fmt.Sprintf("%x", checksum)

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (10, 'V10 template mining: message_templates, template_matches', CURRENT_TIMESTAMP, ?)
	`, checksumHex); err != nil {
		return fmt.Errorf("record migration v10: %w", err)
	}

	return tx.Commit()
}

// applyV11Migration adds the F3 correction-detection surface: the perennial
// correction_signals table (candidates from deterministic detectors, anchored
// by message identity). One row per (source_path, ordinal, detector) tuple.
// extraction_version tracks detector evolution (like message extraction_version).
func (d *Database) applyV11Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const sqlV11 = `
CREATE TABLE IF NOT EXISTS correction_signals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_uuid TEXT,
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    detector TEXT NOT NULL,
    confidence REAL NOT NULL,
    extraction_version INTEGER NOT NULL,
    UNIQUE(source_path, ordinal, detector)
);
CREATE INDEX IF NOT EXISTS idx_correction_signals_detector ON correction_signals(detector);
CREATE INDEX IF NOT EXISTS idx_correction_signals_confidence ON correction_signals(confidence DESC);
`
	if _, err := tx.Exec(sqlV11); err != nil {
		return fmt.Errorf("apply v11 correction_signals schema: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV11))
	checksumHex := fmt.Sprintf("%x", checksum)

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (11, 'V11 correction detection: correction_signals', CURRENT_TIMESTAMP, ?)
	`, checksumHex); err != nil {
		return fmt.Errorf("record migration v11: %w", err)
	}

	return tx.Commit()
}

// applyV12Migration adds the F3b agent-classification surface: the perennial
// annotations table (one row per message per kind; re-annotating replaces).
// Labels are free-form in v1; label_enum freezing is a future slice (post-calibration).
func (d *Database) applyV12Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const sqlV12 = `
CREATE TABLE IF NOT EXISTS annotations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_uuid TEXT,
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    kind TEXT NOT NULL,
    label TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'agent',
    created_at TEXT NOT NULL,
    UNIQUE(source_path, ordinal, kind)
);
CREATE INDEX IF NOT EXISTS idx_annotations_uuid ON annotations(item_uuid);
CREATE INDEX IF NOT EXISTS idx_annotations_kind ON annotations(kind);
`
	if _, err := tx.Exec(sqlV12); err != nil {
		return fmt.Errorf("apply v12 annotations schema: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV12))
	checksumHex := fmt.Sprintf("%x", checksum)

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (12, 'V12 agent classification: annotations (free-form labels; enum freeze deferred)', CURRENT_TIMESTAMP, ?)
	`, checksumHex); err != nil {
		return fmt.Errorf("record migration v12: %w", err)
	}

	return tx.Commit()
}

// applyV13Migration adds indexes on template_matches and correction_signals
// for efficient backfill discovery queries. The queries use NOT EXISTS subqueries
// on source_path; indexes reduce from O(N·M) table scans to O(N·log M) index lookups.
func (d *Database) applyV13Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const sqlV13 = `
CREATE INDEX IF NOT EXISTS idx_template_matches_source ON template_matches(source_path);
CREATE INDEX IF NOT EXISTS idx_correction_signals_source ON correction_signals(source_path);
`
	if _, err := tx.Exec(sqlV13); err != nil {
		return fmt.Errorf("apply v13 indexes: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV13))
	checksumHex := fmt.Sprintf("%x", checksum)

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (13, 'V13 backfill discovery indexes', CURRENT_TIMESTAMP, ?)
	`, checksumHex); err != nil {
		return fmt.Errorf("record migration v13: %w", err)
	}

	return tx.Commit()
}
