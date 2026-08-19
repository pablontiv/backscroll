-- Backscroll release schema fixture: v6.sql
-- Hermetic schema-only fixture captured for compatibility tests.
-- No manifest release tag maps to this fixture.
-- It preserves a partial legacy per-step migration state after V6 and before
-- V7 for compatibility triangulation only: source_metadata has already been
-- dropped, while the reasoning trigger migration has not yet run. This does
-- not claim any published release shipped V6 alone.

BEGIN TRANSACTION;
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_on TEXT NOT NULL,
    checksum TEXT NOT NULL
);

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
    content_type TEXT NOT NULL DEFAULT 'text'
);

CREATE INDEX IF NOT EXISTS idx_search_items_source_path ON search_items(source_path);
CREATE INDEX IF NOT EXISTS idx_search_items_project ON search_items(project);

CREATE TABLE IF NOT EXISTS session_tags (
    source_path TEXT NOT NULL,
    tag TEXT NOT NULL,
    PRIMARY KEY (source_path, tag)
);

CREATE INDEX IF NOT EXISTS idx_session_tags_tag ON session_tags(tag);

CREATE TABLE IF NOT EXISTS dynamic_stopwords (term TEXT PRIMARY KEY);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
    text,
    content=search_items,
    content_rowid=id,
    tokenize='porter unicode61'
);

CREATE VIRTUAL TABLE IF NOT EXISTS messages_vocab USING fts5vocab(messages_fts, 'row');

CREATE VIRTUAL TABLE IF NOT EXISTS tool_fts USING fts5(
    text,
    content=search_items,
    content_rowid=id,
    tokenize='trigram'
);

CREATE VIRTUAL TABLE IF NOT EXISTS tool_vocab USING fts5vocab(tool_fts, 'row');

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

CREATE TABLE IF NOT EXISTS chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id TEXT NOT NULL,
    chunk_idx INTEGER NOT NULL,
    content TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    embedding BLOB,
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

INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (1, 'V1 core schema', '1970-01-01 00:00:00', '4e07949ccd3912fb3c0e149be9a2e05fdd51f8cedb8df1f28b3bb5ac5afe532a');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (2, 'V2 embedding tables', '1970-01-01 00:00:00', '37dc9627f01f0e2d0fbea6bba5cd9f609d5da05089eeb9541a057dd2290cf8af');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (3, 'V3 embedding blob column', '1970-01-01 00:00:00', '36cd183f10ff84ab4753be027078cdd710b46efdc830053d4a641af725006ea5');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (4, 'V4 tool_fts trigram index', '1970-01-01 00:00:00', '77e59cf515f33c466282e0f3e921377f584794d0b7cade1704f566374a569a55');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (5, 'V5 drop phantom session_events', '1970-01-01 00:00:00', 'aedaab81efb6bc34d3f664468b71c1a716f5b55d5132156cb43bd7462d549c7b');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (6, 'V6 drop phantom source_metadata column', '1970-01-01 00:00:00', 'a327b9b6e7b8f5fe369c9fc08093ac80a87640daa515890af94b269d430e9378');
COMMIT;
