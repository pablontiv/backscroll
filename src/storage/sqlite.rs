use crate::core::{
    ParsedFile, ProjectBreakdown, SearchEngine, SearchResult, SessionEntry, Stats, TopicEntry,
};
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

        let current_version: i64 = self
            .conn
            .query_row("SELECT version FROM schema_version", [], |row| row.get(0))
            .into_diagnostic()?;

        if current_version == 2 {
            self.conn
                .execute(
                    "CREATE VIRTUAL TABLE IF NOT EXISTS messages_vocab USING fts5vocab(messages_fts, row)",
                    [],
                )
                .into_diagnostic()?;

            self.conn
                .execute("UPDATE schema_version SET version = 3", [])
                .into_diagnostic()?;
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
                        "INSERT OR IGNORE INTO search_items (source, source_path, ordinal, role, text, project, uuid, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                        (
                            &file.source,
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
        source: &Option<String>,
        after: &Option<String>,
        before: &Option<String>,
    ) -> miette::Result<Vec<SearchResult>> {
        let mut results = Vec::new();
        let snippet_expr = "snippet(messages_fts, 0, '>>>', '<<<', '...', 32)";

        let source_filter = match source.as_deref() {
            Some("sessions") => Some("session"),
            Some("plans") => Some("plan"),
            _ => None, // "all" or None
        };

        let base = format!(
            "SELECT si.source_path, si.text, m.rank as score, {} as snippet FROM messages_fts m JOIN search_items si ON m.rowid = si.id WHERE messages_fts MATCH ?",
            snippet_expr
        );

        let mut conditions = Vec::new();
        let mut param_values: Vec<Box<dyn rusqlite::types::ToSql>> = Vec::new();
        param_values.push(Box::new(query_str.to_string()));

        if let Some(p) = project {
            conditions.push("si.project = ?");
            param_values.push(Box::new(p.clone()));
        }
        if let Some(s) = source_filter {
            conditions.push("si.source = ?");
            param_values.push(Box::new(s.to_string()));
        }
        if let Some(a) = after {
            conditions.push("si.timestamp IS NOT NULL AND si.timestamp >= ?");
            param_values.push(Box::new(a.clone()));
        }
        if let Some(b) = before {
            conditions.push("si.timestamp IS NOT NULL AND si.timestamp < ?");
            param_values.push(Box::new(b.clone()));
        }

        let sql = if conditions.is_empty() {
            format!("{} ORDER BY rank LIMIT 20", base)
        } else {
            format!(
                "{} AND {} ORDER BY rank LIMIT 20",
                base,
                conditions.join(" AND ")
            )
        };

        let params: Vec<&dyn rusqlite::types::ToSql> =
            param_values.iter().map(|p| p.as_ref()).collect();
        let mut stmt = self.conn.prepare(&sql).into_diagnostic()?;
        let rows = stmt
            .query_map(params.as_slice(), Database::map_search_row)
            .into_diagnostic()?;
        for row in rows {
            results.push(row.into_diagnostic()?);
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

    fn get_topics(&self, project: Option<&str>, limit: usize) -> miette::Result<Vec<TopicEntry>> {
        const STOPWORDS: &[&str] = &[
            // Spanish
            "como", "cual", "debe", "desde", "donde", "ella", "ellos", "esta", "este", "esto",
            "esta", "estan", "hacer", "hacia", "hasta", "ninguno", "nosotros", "nuestro", "otra",
            "otras", "otro", "otros", "para", "pero", "puede", "quien", "sera", "sido", "sino",
            "sobre", "solo", "somos", "tambien", "tiene", "tienen", "toda", "todas", "todo",
            "todos", "usted", "vamos", // English
            "about", "after", "also", "been", "before", "being", "between", "both", "could",
            "does", "doing", "done", "down", "each", "even", "every", "file", "from", "have",
            "here", "http", "https", "into", "just", "know", "like", "line", "make", "many",
            "more", "most", "much", "must", "need", "only", "other", "over", "path", "said",
            "same", "self", "should", "some", "such", "text", "than", "that", "them", "then",
            "there", "these", "they", "this", "those", "through", "used", "using", "very", "want",
            "were", "what", "when", "where", "which", "while", "will", "with", "would", "your",
            // Code noise
            "args", "assert", "break", "case", "catch", "class", "const", "continue", "default",
            "else", "enum", "error", "false", "func", "function", "impl", "import", "index",
            "item", "items", "length", "match", "module", "name", "none", "null", "option",
            "param", "print", "println", "private", "public", "query", "result", "return", "some",
            "string", "struct", "super", "switch", "test", "tests", "throw", "true", "type",
            "value", "void",
        ];

        let stopword_placeholders: String =
            STOPWORDS.iter().map(|_| "?").collect::<Vec<_>>().join(",");

        match project {
            None => {
                let sql = format!(
                    "SELECT term, doc, cnt FROM messages_vocab \
                     WHERE length(term) > 3 AND term NOT IN ({}) \
                     ORDER BY doc DESC LIMIT ?",
                    stopword_placeholders
                );
                let mut stmt = self.conn.prepare(&sql).into_diagnostic()?;
                let mut params: Vec<Box<dyn rusqlite::types::ToSql>> = STOPWORDS
                    .iter()
                    .map(|s| Box::new(s.to_string()) as Box<dyn rusqlite::types::ToSql>)
                    .collect();
                params.push(Box::new(limit as i64));

                let param_refs: Vec<&dyn rusqlite::types::ToSql> =
                    params.iter().map(|p| p.as_ref()).collect();

                let rows = stmt
                    .query_map(param_refs.as_slice(), |row| {
                        Ok(TopicEntry {
                            term: row.get(0)?,
                            sessions: row.get(1)?,
                            mentions: row.get(2)?,
                        })
                    })
                    .into_diagnostic()?;

                let mut results = Vec::new();
                for row in rows {
                    results.push(row.into_diagnostic()?);
                }
                Ok(results)
            }
            Some(proj) => {
                // Get candidate terms from global vocab
                let candidate_limit = limit * 5;
                let candidates_sql = format!(
                    "SELECT term FROM messages_vocab \
                     WHERE length(term) > 3 AND term NOT IN ({}) \
                     ORDER BY doc DESC LIMIT ?",
                    stopword_placeholders
                );
                let mut stmt = self.conn.prepare(&candidates_sql).into_diagnostic()?;
                let mut params: Vec<Box<dyn rusqlite::types::ToSql>> = STOPWORDS
                    .iter()
                    .map(|s| Box::new(s.to_string()) as Box<dyn rusqlite::types::ToSql>)
                    .collect();
                params.push(Box::new(candidate_limit as i64));

                let param_refs: Vec<&dyn rusqlite::types::ToSql> =
                    params.iter().map(|p| p.as_ref()).collect();

                let candidate_rows = stmt
                    .query_map(param_refs.as_slice(), |row| {
                        let term: String = row.get(0)?;
                        Ok(term)
                    })
                    .into_diagnostic()?;

                let mut candidates: Vec<String> = Vec::new();
                for row in candidate_rows {
                    candidates.push(row.into_diagnostic()?);
                }

                // For each candidate, get project-specific counts via FTS MATCH
                let count_sql = "SELECT COUNT(DISTINCT si.source_path), COUNT(*) \
                                 FROM messages_fts mf \
                                 JOIN search_items si ON mf.rowid = si.id \
                                 WHERE messages_fts MATCH ? AND si.project = ?";

                let mut results: Vec<TopicEntry> = Vec::new();
                for term in candidates {
                    let row: std::result::Result<(i64, i64), _> =
                        self.conn.query_row(count_sql, params![term, proj], |row| {
                            Ok((row.get(0)?, row.get(1)?))
                        });
                    if let Ok((sessions, mentions)) = row {
                        if sessions > 0 {
                            results.push(TopicEntry {
                                term,
                                sessions,
                                mentions,
                            });
                        }
                    }
                }

                results.sort_by(|a, b| b.sessions.cmp(&a.sessions));
                results.truncate(limit);
                Ok(results)
            }
        }
    }

    fn list_sessions(
        &self,
        project: Option<&str>,
        limit: usize,
    ) -> miette::Result<Vec<SessionEntry>> {
        let (sql, params): (String, Vec<Box<dyn rusqlite::types::ToSql>>) = match project {
            Some(proj) => (
                "SELECT source_path, project, COUNT(*) as messages, \
                 MIN(timestamp) as started, MAX(timestamp) as ended \
                 FROM search_items WHERE source = 'session' AND project = ? \
                 GROUP BY source_path ORDER BY MAX(timestamp) DESC LIMIT ?"
                    .to_string(),
                vec![
                    Box::new(proj.to_string()) as Box<dyn rusqlite::types::ToSql>,
                    Box::new(limit as i64),
                ],
            ),
            None => (
                "SELECT source_path, project, COUNT(*) as messages, \
                 MIN(timestamp) as started, MAX(timestamp) as ended \
                 FROM search_items WHERE source = 'session' \
                 GROUP BY source_path ORDER BY MAX(timestamp) DESC LIMIT ?"
                    .to_string(),
                vec![Box::new(limit as i64) as Box<dyn rusqlite::types::ToSql>],
            ),
        };

        let param_refs: Vec<&dyn rusqlite::types::ToSql> =
            params.iter().map(|p| p.as_ref()).collect();
        let mut stmt = self.conn.prepare(&sql).into_diagnostic()?;
        let rows = stmt
            .query_map(param_refs.as_slice(), |row| {
                Ok(SessionEntry {
                    source_path: row.get(0)?,
                    project: row.get(1)?,
                    messages: row.get(2)?,
                    started: row.get(3)?,
                    ended: row.get(4)?,
                })
            })
            .into_diagnostic()?;

        let mut results = Vec::new();
        for row in rows {
            results.push(row.into_diagnostic()?);
        }
        Ok(results)
    }

    fn get_project_breakdown(&self) -> miette::Result<Vec<ProjectBreakdown>> {
        let sql = "SELECT project, COUNT(DISTINCT source_path) as sessions, COUNT(*) as messages \
                   FROM search_items GROUP BY project ORDER BY sessions DESC";
        let mut stmt = self.conn.prepare(sql).into_diagnostic()?;
        let rows = stmt
            .query_map([], |row| {
                Ok(ProjectBreakdown {
                    project: row.get(0)?,
                    sessions: row.get(1)?,
                    messages: row.get(2)?,
                })
            })
            .into_diagnostic()?;

        let mut results = Vec::new();
        for row in rows {
            results.push(row.into_diagnostic()?);
        }
        Ok(results)
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
            source: "session".into(),
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

        let results = db.search("hola", &None, &None, &None, &None)?;
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
            source: "session".into(),
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
            source: "session".into(),
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

    #[test]
    fn test_plan_source_stored() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let file = ParsedFile {
            source: "plan".into(),
            source_path: "/plans/arch.md".to_string(),
            hash: "p1".to_string(),
            project: None,
            messages: vec![ParsedMessage {
                role: "plan".to_string(),
                text: "## Database\n\nUse SQLite".to_string(),
                ordinal: 0,
                uuid: None,
                timestamp: None,
            }],
        };
        db.sync_files(vec![file])?;

        let source: String = db
            .conn
            .query_row(
                "SELECT source FROM search_items WHERE source_path = ?",
                params!["/plans/arch.md"],
                |row| row.get(0),
            )
            .into_diagnostic()?;
        assert_eq!(source, "plan");
        Ok(())
    }

    #[test]
    fn test_plan_sections_produce_multiple_rows() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let file = ParsedFile {
            source: "plan".into(),
            source_path: "/plans/multi.md".to_string(),
            hash: "p2".to_string(),
            project: None,
            messages: vec![
                ParsedMessage {
                    role: "plan".to_string(),
                    text: "# Title\n\nIntro".to_string(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                },
                ParsedMessage {
                    role: "plan".to_string(),
                    text: "## Section A\n\nContent A".to_string(),
                    ordinal: 1,
                    uuid: None,
                    timestamp: None,
                },
            ],
        };
        db.sync_files(vec![file])?;

        let count: i64 = db
            .conn
            .query_row(
                "SELECT COUNT(*) FROM search_items WHERE source_path = ?",
                params!["/plans/multi.md"],
                |row| row.get(0),
            )
            .into_diagnostic()?;
        assert_eq!(count, 2);
        Ok(())
    }

    #[test]
    fn test_plan_incremental_sync_skips_unchanged() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let file = ParsedFile {
            source: "plan".into(),
            source_path: "/plans/inc.md".to_string(),
            hash: "phash1".to_string(),
            project: None,
            messages: vec![ParsedMessage {
                role: "plan".to_string(),
                text: "content".to_string(),
                ordinal: 0,
                uuid: None,
                timestamp: None,
            }],
        };
        db.sync_files(vec![file])?;

        let hashes = db.get_file_hashes()?;
        assert_eq!(hashes.get("/plans/inc.md").unwrap(), "phash1");
        // Second sync with same hash would be skipped by caller
        Ok(())
    }

    #[test]
    fn test_search_source_filter_plans_only() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        db.sync_files(vec![
            ParsedFile {
                source: "session".into(),
                source_path: "/s/s1.jsonl".into(),
                hash: "s1".into(),
                project: None,
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "deploy application".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
            ParsedFile {
                source: "plan".into(),
                source_path: "/p/p1.md".into(),
                hash: "p1".into(),
                project: None,
                messages: vec![ParsedMessage {
                    role: "plan".into(),
                    text: "deploy strategy plan".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
        ])?;

        let plans = db.search("deploy", &None, &Some("plans".into()), &None, &None)?;
        assert_eq!(plans.len(), 1);
        assert_eq!(plans[0].source_path, "/p/p1.md");
        Ok(())
    }

    #[test]
    fn test_search_source_filter_sessions_only() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        db.sync_files(vec![
            ParsedFile {
                source: "session".into(),
                source_path: "/s/s2.jsonl".into(),
                hash: "s2".into(),
                project: None,
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "configure server".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
            ParsedFile {
                source: "plan".into(),
                source_path: "/p/p2.md".into(),
                hash: "p2".into(),
                project: None,
                messages: vec![ParsedMessage {
                    role: "plan".into(),
                    text: "configure infrastructure plan".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
        ])?;

        let sessions = db.search("configure", &None, &Some("sessions".into()), &None, &None)?;
        assert_eq!(sessions.len(), 1);
        assert_eq!(sessions[0].source_path, "/s/s2.jsonl");
        Ok(())
    }

    #[test]
    fn test_search_source_filter_all() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        db.sync_files(vec![
            ParsedFile {
                source: "session".into(),
                source_path: "/s/s3.jsonl".into(),
                hash: "s3".into(),
                project: None,
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "testing filter both".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
            ParsedFile {
                source: "plan".into(),
                source_path: "/p/p3.md".into(),
                hash: "p3".into(),
                project: None,
                messages: vec![ParsedMessage {
                    role: "plan".into(),
                    text: "testing filter both plan".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
        ])?;

        let all = db.search("testing", &None, &None, &None, &None)?;
        assert_eq!(all.len(), 2);
        Ok(())
    }

    #[test]
    fn test_session_entries_not_affected_by_plan_sync() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let session = ParsedFile {
            source: "session".into(),
            source_path: "/sessions/s1.jsonl".to_string(),
            hash: "sh1".to_string(),
            project: Some("proj".into()),
            messages: vec![ParsedMessage {
                role: "user".to_string(),
                text: "session content".to_string(),
                ordinal: 0,
                uuid: None,
                timestamp: None,
            }],
        };
        let plan = ParsedFile {
            source: "plan".into(),
            source_path: "/plans/p1.md".to_string(),
            hash: "ph1".to_string(),
            project: None,
            messages: vec![ParsedMessage {
                role: "plan".to_string(),
                text: "plan content".to_string(),
                ordinal: 0,
                uuid: None,
                timestamp: None,
            }],
        };
        db.sync_files(vec![session])?;
        db.sync_files(vec![plan])?;

        let session_count: i64 = db
            .conn
            .query_row(
                "SELECT COUNT(*) FROM search_items WHERE source = 'session'",
                [],
                |row| row.get(0),
            )
            .into_diagnostic()?;
        let plan_count: i64 = db
            .conn
            .query_row(
                "SELECT COUNT(*) FROM search_items WHERE source = 'plan'",
                [],
                |row| row.get(0),
            )
            .into_diagnostic()?;
        assert_eq!(session_count, 1);
        assert_eq!(plan_count, 1);
        Ok(())
    }

    #[test]
    fn test_get_topics_global() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        db.sync_files(vec![
            ParsedFile {
                source: "session".into(),
                source_path: "/s/s1.jsonl".into(),
                hash: "t1".into(),
                project: Some("alpha".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "kubernetes deployment configuration yaml".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
            ParsedFile {
                source: "session".into(),
                source_path: "/s/s2.jsonl".into(),
                hash: "t2".into(),
                project: Some("alpha".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "kubernetes cluster monitoring setup".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
            ParsedFile {
                source: "session".into(),
                source_path: "/s/s3.jsonl".into(),
                hash: "t3".into(),
                project: Some("beta".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "database migration postgresql".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
        ])?;

        let topics = db.get_topics(None, 10)?;
        assert!(!topics.is_empty());
        // "kubernetes" appears in 2 sessions, should be top
        assert_eq!(topics[0].term, "kubernetes");
        assert_eq!(topics[0].sessions, 2);

        Ok(())
    }

    #[test]
    fn test_get_topics_project_filter() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        db.sync_files(vec![
            ParsedFile {
                source: "session".into(),
                source_path: "/s/s1.jsonl".into(),
                hash: "f1".into(),
                project: Some("alpha".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "kubernetes deployment configuration".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
            ParsedFile {
                source: "session".into(),
                source_path: "/s/s2.jsonl".into(),
                hash: "f2".into(),
                project: Some("beta".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "database migration postgresql".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
        ])?;

        let alpha_topics = db.get_topics(Some("alpha"), 10)?;
        let terms: Vec<&str> = alpha_topics.iter().map(|t| t.term.as_str()).collect();
        assert!(terms.contains(&"kubernetes"));
        assert!(!terms.contains(&"postgresql"));

        Ok(())
    }

    #[test]
    fn test_get_topics_stopwords_excluded() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        db.sync_files(vec![ParsedFile {
            source: "session".into(),
            source_path: "/s/sw.jsonl".into(),
            hash: "sw1".into(),
            project: None,
            messages: vec![ParsedMessage {
                role: "user".into(),
                text: "about this function that should return the value from there".into(),
                ordinal: 0,
                uuid: None,
                timestamp: None,
            }],
        }])?;

        let topics = db.get_topics(None, 50)?;
        let terms: Vec<&str> = topics.iter().map(|t| t.term.as_str()).collect();
        // All words in the message are stopwords or <=3 chars — should be empty
        assert!(topics.is_empty(), "Expected no topics but got: {:?}", terms);

        Ok(())
    }

    #[test]
    fn test_list_sessions_global() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        db.sync_files(vec![
            ParsedFile {
                source: "session".into(),
                source_path: "/s/a.jsonl".into(),
                hash: "la".into(),
                project: Some("proj-a".into()),
                messages: vec![
                    ParsedMessage {
                        role: "user".into(),
                        text: "first".into(),
                        ordinal: 0,
                        uuid: None,
                        timestamp: Some("2026-01-01T10:00:00Z".into()),
                    },
                    ParsedMessage {
                        role: "assistant".into(),
                        text: "second".into(),
                        ordinal: 1,
                        uuid: None,
                        timestamp: Some("2026-01-01T10:05:00Z".into()),
                    },
                ],
            },
            ParsedFile {
                source: "session".into(),
                source_path: "/s/b.jsonl".into(),
                hash: "lb".into(),
                project: Some("proj-b".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "only one".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: Some("2026-01-02T10:00:00Z".into()),
                }],
            },
        ])?;

        let sessions = db.list_sessions(None, 10)?;
        assert_eq!(sessions.len(), 2);
        // Most recent first
        assert_eq!(sessions[0].source_path, "/s/b.jsonl");
        assert_eq!(sessions[0].messages, 1);
        assert_eq!(sessions[1].source_path, "/s/a.jsonl");
        assert_eq!(sessions[1].messages, 2);

        Ok(())
    }

    #[test]
    fn test_list_sessions_project_filter() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        db.sync_files(vec![
            ParsedFile {
                source: "session".into(),
                source_path: "/s/x.jsonl".into(),
                hash: "lx".into(),
                project: Some("alpha".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "alpha msg".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
            ParsedFile {
                source: "session".into(),
                source_path: "/s/y.jsonl".into(),
                hash: "ly".into(),
                project: Some("beta".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "beta msg".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
        ])?;

        let sessions = db.list_sessions(Some("alpha"), 10)?;
        assert_eq!(sessions.len(), 1);
        assert_eq!(sessions[0].project.as_deref(), Some("alpha"));

        Ok(())
    }

    #[test]
    fn test_get_project_breakdown() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        db.sync_files(vec![
            ParsedFile {
                source: "session".into(),
                source_path: "/s/p1.jsonl".into(),
                hash: "pb1".into(),
                project: Some("alpha".into()),
                messages: vec![
                    ParsedMessage {
                        role: "user".into(),
                        text: "msg1".into(),
                        ordinal: 0,
                        uuid: None,
                        timestamp: None,
                    },
                    ParsedMessage {
                        role: "assistant".into(),
                        text: "msg2".into(),
                        ordinal: 1,
                        uuid: None,
                        timestamp: None,
                    },
                ],
            },
            ParsedFile {
                source: "session".into(),
                source_path: "/s/p2.jsonl".into(),
                hash: "pb2".into(),
                project: Some("alpha".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "msg3".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
            ParsedFile {
                source: "session".into(),
                source_path: "/s/p3.jsonl".into(),
                hash: "pb3".into(),
                project: Some("beta".into()),
                messages: vec![ParsedMessage {
                    role: "user".into(),
                    text: "msg4".into(),
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                }],
            },
        ])?;

        let breakdown = db.get_project_breakdown()?;
        assert_eq!(breakdown.len(), 2);
        // alpha has 2 sessions, 3 messages — should be first
        assert_eq!(breakdown[0].project.as_deref(), Some("alpha"));
        assert_eq!(breakdown[0].sessions, 2);
        assert_eq!(breakdown[0].messages, 3);
        assert_eq!(breakdown[1].project.as_deref(), Some("beta"));
        assert_eq!(breakdown[1].sessions, 1);
        assert_eq!(breakdown[1].messages, 1);

        Ok(())
    }
}
