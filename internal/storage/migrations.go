package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pablontiv/backscroll/internal/compat"
)

const createSchemaMigrationsSQL = `
CREATE TABLE schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_on TEXT NOT NULL,
	checksum TEXT NOT NULL
)`

// newDatabaseMigrations returns a new slice for each creation attempt so tests
// can inject an operation-local failure without mutating process-wide seams.
func newDatabaseMigrations() []migrationApplier {
	return []migrationApplier{
		applyV1, applyV2, applyV3, applyV4,
		applyV5, applyV6, applyV7, applyV8,
		applyV9, applyV10, applyV11, applyV12,
		applyV13, applyV14, applyV15, applyV16,
	}
}

// setupNewDatabaseSchema initializes a private, newly created database. All
// schema objects and migration records from V1 through V16 are committed as a
// single unit. Existing databases still migrate through OpenCompatible.
func (d *Database) setupNewDatabaseSchema(ctx context.Context, migrations []migrationApplier) (err error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return contextIdentityError(ctx, fmt.Errorf("begin new database schema transaction: %w", err))
	}
	defer func() {
		if err == nil {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) && !errors.Is(rollbackErr, context.Canceled) {
			err = errors.Join(err, fmt.Errorf("rollback new database schema transaction: %w", rollbackErr))
		}
	}()

	if _, err := tx.ExecContext(ctx, createSchemaMigrationsSQL); err != nil {
		return contextIdentityError(ctx, fmt.Errorf("create schema_migrations table: %w", err))
	}
	for index, apply := range migrations {
		if err := apply(ctx, tx, compat.SchemaShape{}); err != nil {
			return contextIdentityError(ctx, fmt.Errorf("apply new database migration V%d: %w", index+1, err))
		}
	}
	if err := compat.VerifyCurrentShape(ctx, tx); err != nil {
		return contextIdentityError(ctx, fmt.Errorf("verify new database schema before commit: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return contextIdentityError(ctx, fmt.Errorf("commit new database schema: %w", err))
	}
	return nil
}

// applyV1Migration applies version 1 of the schema (all core tables).
func (d *Database) applyV1Migration() error {
	return d.applySingleMigration(applyV1)
}

// applyV2Migration adds tables for the embedding system: chunks and embedding_metadata.
func (d *Database) applyV2Migration() error {
	return d.applySingleMigration(applyV2)
}

// applyV3Migration adds an embedding BLOB column to chunks for pure-Go vector search.
// Decision (T039): sqlite-vec requires CGO; we store embedding bytes directly in chunks
// and perform cosine similarity in Go (linear scan).
func (d *Database) applyV3Migration() error {
	return d.applySingleMigration(applyV3)
}

// applyV4Migration adds the tool_fts index (trigram tokenizer), branches the
// sync triggers by content_type, and repopulates both indexes from search_items.
func (d *Database) applyV4Migration() error {
	return d.applySingleMigration(applyV4)
}

// applyV5Migration drops the phantom session_events table. Nothing reads or
// writes it after the structured-stats surface was removed (stats command,
// structured list filters, and the session_events query/insert paths are gone).
// Per the schema rule this is a new migration; V1 still creates the table on
// the way up, and V5 drops it.
func (d *Database) applyV5Migration() error {
	return d.applySingleMigration(applyV5)
}

// applyV6Migration drops the phantom source_metadata column. Nothing reads or
// writes it (no production callers, no SELECT access anywhere).
// Per the schema rule this is a new migration; V1 still creates the column on
// the way up, and V6 drops it.
func (d *Database) applyV6Migration() error {
	return d.applySingleMigration(applyV6)
}

// applyV7Migration updates the content_type-branched triggers to support reasoning
// indexing. Reasoning blocks (content_type='reasoning') route to messages_fts
// alongside 'text' and 'code', NOT to tool_fts. This preserves the v4 semantic:
// tool_fts is for structured tool metadata (names, paths, commands); messages_fts
// is for prose (text, code, reasoning).
func (d *Database) applyV7Migration() error {
	return d.applySingleMigration(applyV7)
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
	return d.applySingleMigration(applyV8)
}

func (d *Database) applyV9Migration() error {
	return d.applySingleMigration(applyV9)
}

// applyV10Migration adds F2 template mining surface: message_templates
// (derived, rebuildable) and template_matches (perennial join, anchors
// templates to search_items via ordinal + source_path, with UNIQUE to stay
// idempotent under re-sync). Only templates with occurrence_count >= 3
// (configurable) are reported; mining runs inside SyncFiles tx.
func (d *Database) applyV10Migration() error {
	return d.applySingleMigration(applyV10)
}

// applyV11Migration adds the F3 correction-detection surface: the perennial
// correction_signals table (candidates from deterministic detectors, anchored
// by message identity). One row per (source_path, ordinal, detector) tuple.
// extraction_version tracks detector evolution (like message extraction_version).
func (d *Database) applyV11Migration() error {
	return d.applySingleMigration(applyV11)
}

// applyV12Migration adds the F3b agent-classification surface: the perennial
// annotations table (one row per message per kind; re-annotating replaces).
// Labels are free-form in v1; label_enum freezing is a future slice (post-calibration).
func (d *Database) applyV12Migration() error {
	return d.applySingleMigration(applyV12)
}

// applyV13Migration adds indexes on template_matches and correction_signals
// for efficient backfill discovery queries. The queries use NOT EXISTS subqueries
// on source_path; indexes reduce from O(N·M) table scans to O(N·log M) index lookups.
func (d *Database) applyV13Migration() error {
	return d.applySingleMigration(applyV13)
}

// applyV14Migration adds file_size and file_mtime columns to indexed_files
// for metadata-based prefiltering during startup sync. Existing rows get NULL values
// and are conservatively re-hashed (metadata prefilter requires BOTH columns to be non-NULL).
func (d *Database) applyV14Migration() error {
	return d.applySingleMigration(applyV14)
}
