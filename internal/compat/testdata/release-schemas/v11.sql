-- Backscroll release schema fixture: v11.sql
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
    content_type TEXT NOT NULL DEFAULT 'text',
    extraction_version INTEGER,
    was_interrupted INTEGER
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
CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_events_uuid_unique ON tool_events(message_uuid) WHERE message_uuid IS NOT NULL;

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

INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (1, 'V1 core schema', '1970-01-01 00:00:00', '4e07949ccd3912fb3c0e149be9a2e05fdd51f8cedb8df1f28b3bb5ac5afe532a');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (2, 'V2 embedding tables', '1970-01-01 00:00:00', '37dc9627f01f0e2d0fbea6bba5cd9f609d5da05089eeb9541a057dd2290cf8af');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (3, 'V3 embedding blob column', '1970-01-01 00:00:00', '36cd183f10ff84ab4753be027078cdd710b46efdc830053d4a641af725006ea5');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (4, 'V4 tool_fts trigram index', '1970-01-01 00:00:00', '77e59cf515f33c466282e0f3e921377f584794d0b7cade1704f566374a569a55');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (5, 'V5 drop phantom session_events', '1970-01-01 00:00:00', 'aedaab81efb6bc34d3f664468b71c1a716f5b55d5132156cb43bd7462d549c7b');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (6, 'V6 drop phantom source_metadata column', '1970-01-01 00:00:00', 'a327b9b6e7b8f5fe369c9fc08093ac80a87640daa515890af94b269d430e9378');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (7, 'V7 reasoning content_type routes to messages_fts', '1970-01-01 00:00:00', 'a80704442c2a0084f98e4bc53978364b14d1c6bd3bd99ba6119a2a9ecbd685e7');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (8, 'V8 perennity: extraction_version, was_interrupted, tool_events', '1970-01-01 00:00:00', '6853d72ded3bdc775b52507321c31df44bf35e8277719432ca8aa126ee16cec1');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (9, 'V9 tool_events uuid uniqueness index', '1970-01-01 00:00:00', 'b16094805a4e08f6e0dd56bce5266c7c5fd71934389da9d13c4132076e546ca2');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (10, 'V10 template mining: message_templates, template_matches', '1970-01-01 00:00:00', '0e548d0cb6c47147726f943bfc860500ca9a9bc821df0601998876ea5e9652c2');
INSERT INTO schema_migrations (version, name, applied_on, checksum) VALUES (11, 'V11 correction detection: correction_signals', '1970-01-01 00:00:00', '5a7180f901c5feacb67cd104a61e7b7ba9cec69aaeab1d2b5d723459dc590456');
COMMIT;
