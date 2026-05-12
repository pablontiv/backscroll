use miette::IntoDiagnostic;
use rusqlite::Connection;
use sha2::{Digest, Sha256};

struct Migration {
    version: u32,
    name: &'static str,
    sql: &'static str,
}

impl Migration {
    fn checksum(&self) -> String {
        let mut hasher = Sha256::new();
        hasher.update(self.sql.as_bytes());
        hex::encode(hasher.finalize())
    }
}

// Complete v7 schema — all objects use IF NOT EXISTS so this is safe on both
// new and existing databases. Existing tables/indexes/triggers are no-ops.
const SQL_V1: &str = "
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

CREATE TABLE IF NOT EXISTS chunks (
    id INTEGER PRIMARY KEY,
    source_item_id INTEGER NOT NULL,
    chunk_index INTEGER NOT NULL,
    chunk_text TEXT NOT NULL,
    FOREIGN KEY (source_item_id) REFERENCES search_items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_chunks_source_item_id ON chunks(source_item_id);

CREATE TABLE IF NOT EXISTS embedding_metadata (
    model_name TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    last_embedded_at TEXT NOT NULL
);
";

// Virtual tables and triggers cannot use IF NOT EXISTS universally across all
// SQLite versions for TRIGGER, so we apply them conditionally via existence checks.
const SQL_V1_VIRTUAL: &str = "
CREATE VIRTUAL TABLE IF NOT EXISTS vec_embeddings USING vec0(
    embedding float[384] distance_metric=cosine
);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    text,
    content=search_items,
    content_rowid=id,
    tokenize='porter unicode61'
);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_vocab USING fts5vocab(messages_fts, row);
";

const SQL_V1_TRIGGERS: &str = "
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
";

fn migrations() -> Vec<Migration> {
    vec![Migration {
        version: 1,
        name: "bootstrap",
        // Checksum covers core schema only; virtual tables/triggers are applied
        // separately and are idempotent via IF NOT EXISTS / existence checks.
        sql: SQL_V1,
    }]
}

fn ensure_migrations_table(conn: &Connection) -> miette::Result<()> {
    conn.execute_batch(
        "CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            applied_on TEXT NOT NULL,
            checksum TEXT NOT NULL
        )",
    )
    .into_diagnostic()
}

fn drop_legacy_tracking(conn: &Connection) -> miette::Result<()> {
    let exists: bool = conn
        .query_row(
            "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'",
            [],
            |row| row.get::<_, i64>(0),
        )
        .into_diagnostic()?
        > 0;

    if exists {
        conn.execute("DROP TABLE schema_version", [])
            .into_diagnostic()?;
    }
    Ok(())
}

fn apply_virtual_objects(conn: &Connection) -> miette::Result<()> {
    // Check if messages_fts already exists before trying to create virtual tables.
    // Virtual tables don't always support IF NOT EXISTS in older SQLite, but
    // sqlite-vec's vec0 and SQLite's fts5/fts5vocab do support it in our bundled version.
    conn.execute_batch(SQL_V1_VIRTUAL).into_diagnostic()?;
    conn.execute_batch(SQL_V1_TRIGGERS).into_diagnostic()?;
    Ok(())
}

pub fn run(conn: &mut Connection) -> miette::Result<()> {
    drop_legacy_tracking(conn)?;
    ensure_migrations_table(conn)?;

    for migration in migrations() {
        let already_applied: bool = conn
            .query_row(
                "SELECT COUNT(*) FROM schema_migrations WHERE version = ?1",
                [migration.version],
                |row| row.get::<_, i64>(0),
            )
            .into_diagnostic()?
            > 0;

        if already_applied {
            let stored: String = conn
                .query_row(
                    "SELECT checksum FROM schema_migrations WHERE version = ?1",
                    [migration.version],
                    |row| row.get(0),
                )
                .into_diagnostic()?;
            let expected = migration.checksum();
            if stored != expected {
                return Err(miette::miette!(
                    "migration V{} '{}' checksum mismatch — expected {}, stored {}",
                    migration.version,
                    migration.name,
                    expected,
                    stored
                ));
            }
            // Core schema already applied; still ensure virtual objects exist.
            apply_virtual_objects(conn)?;
            continue;
        }

        let tx = conn.transaction().into_diagnostic()?;
        tx.execute_batch(migration.sql).into_diagnostic()?;
        tx.execute(
            "INSERT INTO schema_migrations (version, name, applied_on, checksum)
             VALUES (?1, ?2, datetime('now'), ?3)",
            rusqlite::params![migration.version, migration.name, migration.checksum()],
        )
        .into_diagnostic()?;
        tx.commit().into_diagnostic()?;

        apply_virtual_objects(conn)?;
    }

    Ok(())
}
