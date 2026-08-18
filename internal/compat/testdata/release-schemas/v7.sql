-- Backscroll release schema fixture: v7.sql
-- Hermetic schema-only fixture captured for compatibility tests.

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

INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (1, 'V1 core schema', '1970-01-01 00:00:00', '382f1c806871c1cbcd1e7e01c9a54ee19018af6664ed66b406fcc003927d2550');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (2, 'V2 embedding tables', '1970-01-01 00:00:00', '7989ada72a0079e1d36317f09b4ec9b220f0d6bc649e69e7a6a728871569fd19');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (3, 'V3 embedding blob column', '1970-01-01 00:00:00', '0ce6d04de0ba20c9fd15d937c5177ba0fafa50f50eb6e9ac9a10525779665bb6');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (4, 'V4 tool_fts trigram index', '1970-01-01 00:00:00', '65cced471db2b3fdd67128dcf88ebd99525cd9520e2ee5704496cb3e7caa41db');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (5, 'V5 drop phantom session_events', '1970-01-01 00:00:00', '71ae84adf2092151071c6165ff9468b1d1e58066ffbd0e20294ef410dc0513cc');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (6, 'V6 drop phantom source_metadata column', '1970-01-01 00:00:00', 'b3da57e183c69f8d4c541dfb80fa833e2fe967ee078c7cfb07e6acf42bb2f20f');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (7, 'V7 reasoning content_type routes to messages_fts', '1970-01-01 00:00:00', '06ee045e314c2d601320d4a1ce868429ccd1c0932baa6c2a7542a3c27d3cdc14');
COMMIT;
