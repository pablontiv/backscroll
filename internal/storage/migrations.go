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
