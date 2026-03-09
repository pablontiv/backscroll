use crate::core::{ParsedFile, SearchEngine, SearchResult, Stats};
use miette::IntoDiagnostic;
use rusqlite::{Connection, Result, Row, params};
use std::collections::HashMap;
use std::path::Path;
use std::time::Duration;

pub struct Database {
    conn: Connection,
}

impl Database {
    pub fn open(path: impl AsRef<Path>) -> miette::Result<Self> {
        let conn = Connection::open(path).into_diagnostic()?;

        let _: String = conn
            .query_row("PRAGMA journal_mode=WAL;", [], |row| row.get(0))
            .into_diagnostic()?;
        conn.execute("PRAGMA synchronous=NORMAL;", [])
            .into_diagnostic()?;
        conn.busy_timeout(Duration::from_millis(5000))
            .into_diagnostic()?;

        Ok(Self { conn })
    }

    pub fn setup_schema(&self) -> miette::Result<()> {
        self.conn
            .execute(
                "CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)",
                [],
            )
            .into_diagnostic()?;

        let count: i64 = self
            .conn
            .query_row("SELECT COUNT(*) FROM schema_version", [], |row| row.get(0))
            .into_diagnostic()?;

        if count == 0 {
            // Initial v1 schema
            self.conn
                .execute(
                    "CREATE TABLE IF NOT EXISTS indexed_files (
                    path TEXT PRIMARY KEY,
                    hash TEXT NOT NULL,
                    last_indexed DATETIME DEFAULT CURRENT_TIMESTAMP
                )",
                    [],
                )
                .into_diagnostic()?;

            self.conn
                .execute(
                    "CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
                    path,
                    role,
                    content,
                    project,
                    tokenize='unicode61'
                )",
                    [],
                )
                .into_diagnostic()?;

            self.conn
                .execute("INSERT INTO schema_version (version) VALUES (1)", [])
                .into_diagnostic()?;
        }

        let current_version: i64 = self
            .conn
            .query_row("SELECT version FROM schema_version", [], |row| row.get(0))
            .into_diagnostic()?;

        if current_version == 1 {
            self.conn
                .execute("BEGIN TRANSACTION", [])
                .into_diagnostic()?;

            self.conn
                .execute(
                    "CREATE TABLE IF NOT EXISTS search_items (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    source TEXT NOT NULL DEFAULT 'session',
                    source_path TEXT NOT NULL,
                    ordinal INTEGER NOT NULL,
                    role TEXT NOT NULL,
                    text TEXT NOT NULL,
                    timestamp TEXT,
                    uuid TEXT UNIQUE,
                    project TEXT
                )",
                    [],
                )
                .into_diagnostic()?;

            self.conn.execute("CREATE INDEX IF NOT EXISTS idx_search_items_source_path ON search_items(source_path)", []).into_diagnostic()?;
            self.conn
                .execute(
                    "CREATE INDEX IF NOT EXISTS idx_search_items_project ON search_items(project)",
                    [],
                )
                .into_diagnostic()?;

            self.conn
                .execute("DROP TABLE IF EXISTS messages_fts", [])
                .into_diagnostic()?;

            self.conn
                .execute(
                    "CREATE VIRTUAL TABLE messages_fts USING fts5(
                    text,
                    content=search_items,
                    content_rowid=id,
                    tokenize='unicode61'
                )",
                    [],
                )
                .into_diagnostic()?;

            self.conn
                .execute(
                    "
                CREATE TRIGGER IF NOT EXISTS search_items_ai AFTER INSERT ON search_items BEGIN
                    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
                END;
            ",
                    [],
                )
                .into_diagnostic()?;

            self.conn.execute("
                CREATE TRIGGER IF NOT EXISTS search_items_ad AFTER DELETE ON search_items BEGIN
                    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
                END;
            ", []).into_diagnostic()?;

            self.conn.execute("
                CREATE TRIGGER IF NOT EXISTS search_items_au AFTER UPDATE ON search_items BEGIN
                    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
                    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
                END;
            ", []).into_diagnostic()?;

            self.conn
                .execute("UPDATE schema_version SET version = 2", [])
                .into_diagnostic()?;
            self.conn.execute("COMMIT", []).into_diagnostic()?;
        }

        Ok(())
    }

    fn map_search_row(row: &Row<'_>) -> Result<SearchResult> {
        Ok(SearchResult {
            source_path: row.get(0)?,
            text: row.get(1)?,
            score: row.get(2)?,
            match_snippet: row.get(3).ok(),
        })
    }
}

impl SearchEngine for Database {
    fn sync_files(&self, files: Vec<ParsedFile>) -> miette::Result<()> {
        self.conn
            .execute("BEGIN TRANSACTION", [])
            .into_diagnostic()?;
        for file in files {
            self.conn
                .execute(
                    "DELETE FROM search_items WHERE source_path = ?",
                    [&file.source_path],
                )
                .into_diagnostic()?;

            for msg in file.messages {
                self.conn
                    .execute(
                        "INSERT OR IGNORE INTO search_items (source_path, ordinal, role, text, project, uuid, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?)",
                        (
                            &file.source_path,
                            msg.ordinal as i64,
                            &msg.role,
                            &msg.text,
                            file.project.as_deref(),
                            msg.uuid.as_deref(),
                            msg.timestamp.as_deref(),
                        ),
                    )
                    .into_diagnostic()?;
            }

            self.conn.execute(
                "INSERT OR REPLACE INTO indexed_files (path, hash, last_indexed) VALUES (?, ?, CURRENT_TIMESTAMP)",
                [&file.source_path, &file.hash],
            ).into_diagnostic()?;
        }
        self.conn.execute("COMMIT", []).into_diagnostic()?;
        Ok(())
    }

    #[tracing::instrument(skip(self))]
    fn search(
        &self,
        query_str: &str,
        project: &Option<String>,
    ) -> miette::Result<Vec<SearchResult>> {
        let mut results = Vec::new();
        let snippet_expr = "snippet(messages_fts, 0, '>>>', '<<<', '...', 32)";

        if let Some(p) = project {
            let sql = format!(
                "SELECT si.source_path, si.text, m.rank as score, {} as snippet FROM messages_fts m JOIN search_items si ON m.rowid = si.id WHERE messages_fts MATCH ? AND si.project = ? ORDER BY rank LIMIT 20",
                snippet_expr
            );
            let mut stmt = self.conn.prepare(&sql).into_diagnostic()?;
            let rows = stmt
                .query_map(params![query_str, p], Database::map_search_row)
                .into_diagnostic()?;
            for row in rows {
                results.push(row.into_diagnostic()?);
            }
        } else {
            let sql = format!(
                "SELECT si.source_path, si.text, m.rank as score, {} as snippet FROM messages_fts m JOIN search_items si ON m.rowid = si.id WHERE messages_fts MATCH ? ORDER BY rank LIMIT 20",
                snippet_expr
            );
            let mut stmt = self.conn.prepare(&sql).into_diagnostic()?;
            let rows = stmt
                .query_map(params![query_str], Database::map_search_row)
                .into_diagnostic()?;
            for row in rows {
                results.push(row.into_diagnostic()?);
            }
        }

        Ok(results)
    }

    fn get_file_hashes(&self) -> miette::Result<HashMap<String, String>> {
        let mut hashes = HashMap::new();
        let mut stmt = self
            .conn
            .prepare("SELECT path, hash FROM indexed_files")
            .into_diagnostic()?;
        let rows = stmt
            .query_map([], |row| {
                let path: String = row.get(0)?;
                let hash: String = row.get(1)?;
                Ok((path, hash))
            })
            .into_diagnostic()?;

        for row in rows {
            let (path, hash) = row.into_diagnostic()?;
            hashes.insert(path, hash);
        }
        Ok(hashes)
    }

    fn get_session_id(&self, source_path: &str) -> miette::Result<Option<String>> {
        let result: Option<String> = self
            .conn
            .query_row(
                "SELECT uuid FROM search_items WHERE source_path = ? AND uuid IS NOT NULL ORDER BY ordinal LIMIT 1",
                params![source_path],
                |row| row.get(0),
            )
            .ok();

        // Fallback: extract file stem from path
        Ok(Some(result.unwrap_or_else(|| {
            std::path::Path::new(source_path)
                .file_stem()
                .and_then(|s| s.to_str())
                .unwrap_or(source_path)
                .to_string()
        })))
    }

    fn get_stats(&self) -> miette::Result<Stats> {
        let file_count: i64 = self
            .conn
            .query_row(
                "SELECT count(DISTINCT source_path) FROM search_items",
                [],
                |row| row.get(0),
            )
            .unwrap_or(0);

        let message_count: i64 = self
            .conn
            .query_row("SELECT count(*) FROM search_items", [], |row| row.get(0))
            .unwrap_or(0);

        let page_count: i64 = self
            .conn
            .query_row("PRAGMA page_count", [], |row| row.get(0))
            .unwrap_or(0);
        let page_size: i64 = self
            .conn
            .query_row("PRAGMA page_size", [], |row| row.get(0))
            .unwrap_or(0);
        let db_size_bytes = page_count * page_size;

        let last_sync: Option<String> = self
            .conn
            .query_row("SELECT max(timestamp) FROM search_items", [], |row| {
                row.get(0)
            })
            .unwrap_or(None);

        let project_count: i64 = self
            .conn
            .query_row(
                "SELECT count(DISTINCT project) FROM search_items",
                [],
                |row| row.get(0),
            )
            .unwrap_or(0);

        Ok(Stats {
            file_count,
            message_count,
            db_size_bytes,
            last_sync,
            project_count,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::core::ParsedMessage;

    #[test]
    fn test_db_workflow() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let path = "test.json";
        let hash = "abc";
        let hashes = db.get_file_hashes()?;
        assert!(!hashes.contains_key(path));

        let file = ParsedFile {
            source_path: path.to_string(),
            hash: hash.to_string(),
            project: Some("project-x".to_string()),
            messages: vec![ParsedMessage {
                role: "user".to_string(),
                text: "hola mundo rust".to_string(),
                ordinal: 0,
                uuid: None,
                timestamp: None,
            }],
        };

        db.sync_files(vec![file])?;
        let hashes = db.get_file_hashes()?;
        assert_eq!(hashes.get(path).unwrap(), hash);

        let results = db.search("hola", &None)?;
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].text, "hola mundo rust");
        assert!(results[0].match_snippet.is_some());

        Ok(())
    }

    #[test]
    fn test_get_session_id_with_uuid() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let file = ParsedFile {
            source_path: "/sessions/session.jsonl".to_string(),
            hash: "h1".to_string(),
            project: None,
            messages: vec![ParsedMessage {
                role: "user".to_string(),
                text: "hello".to_string(),
                ordinal: 0,
                uuid: Some("04df2262-a48e-4549-97a9-11bcf4bb0257".to_string()),
                timestamp: None,
            }],
        };
        db.sync_files(vec![file])?;

        let id = db.get_session_id("/sessions/session.jsonl")?;
        assert_eq!(id, Some("04df2262-a48e-4549-97a9-11bcf4bb0257".to_string()));
        Ok(())
    }

    #[test]
    fn test_get_session_id_fallback_to_stem() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let file = ParsedFile {
            source_path: "/sessions/my-session.jsonl".to_string(),
            hash: "h2".to_string(),
            project: None,
            messages: vec![ParsedMessage {
                role: "user".to_string(),
                text: "no uuid here".to_string(),
                ordinal: 0,
                uuid: None,
                timestamp: None,
            }],
        };
        db.sync_files(vec![file])?;

        let id = db.get_session_id("/sessions/my-session.jsonl")?;
        assert_eq!(id, Some("my-session".to_string()));
        Ok(())
    }

    #[test]
    fn test_get_session_id_nonexistent_path() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let id = db.get_session_id("/does/not/exist.jsonl")?;
        assert_eq!(id, Some("exist".to_string()));
        Ok(())
    }
}
