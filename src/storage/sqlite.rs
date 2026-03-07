use rusqlite::{Connection, Result, params, Row};
use std::path::Path;
use std::time::Duration;
use miette::IntoDiagnostic;
use crate::core::SearchResult;

pub struct Database {
    conn: Connection,
}

impl Database {
    pub fn open(path: impl AsRef<Path>) -> miette::Result<Self> {
        let conn = Connection::open(path).into_diagnostic()?;
        
        let _: String = conn.query_row("PRAGMA journal_mode=WAL;", [], |row| row.get(0)).into_diagnostic()?;
        conn.execute("PRAGMA synchronous=NORMAL;", []).into_diagnostic()?;
        conn.busy_timeout(Duration::from_millis(5000)).into_diagnostic()?;

        Ok(Self { conn })
    }

    pub fn setup_schema(&self) -> miette::Result<()> {
        self.conn.execute(
            "CREATE TABLE IF NOT EXISTS indexed_files (
                path TEXT PRIMARY KEY,
                hash TEXT NOT NULL,
                last_indexed DATETIME DEFAULT CURRENT_TIMESTAMP
            )",
            [],
        ).into_diagnostic()?;

        self.conn.execute(
            "CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
                path,
                role,
                content,
                project,
                tokenize='unicode61'
            )",
            [],
        ).into_diagnostic()?;

        Ok(())
    }

    pub fn index_message(&self, path: &str, role: &str, content: &str, project: Option<&str>) -> miette::Result<()> {
        self.conn.execute(
            "INSERT INTO messages_fts (path, role, content, project) VALUES (?, ?, ?, ?)",
            (path, role, content, project),
        ).into_diagnostic()?;
        Ok(())
    }

    fn map_search_row(row: &Row<'_>) -> Result<SearchResult> {
        Ok(SearchResult {
            path: row.get(0)?,
            content: row.get(1)?,
            score: row.get(2)?,
        })
    }

    pub fn search(&self, query_str: &str, project: Option<&str>) -> miette::Result<Vec<SearchResult>> {
        let mut results = Vec::new();

        if let Some(p) = project {
            let sql = "SELECT path, content, rank FROM messages_fts WHERE messages_fts MATCH ? AND project = ? ORDER BY rank";
            let mut stmt = self.conn.prepare(sql).into_diagnostic()?;
            let rows = stmt.query_map(params![query_str, p], Self::map_search_row).into_diagnostic()?;
            for row in rows {
                results.push(row.into_diagnostic()?);
            }
        } else {
            let sql = "SELECT path, content, rank FROM messages_fts WHERE messages_fts MATCH ? ORDER BY rank";
            let mut stmt = self.conn.prepare(sql).into_diagnostic()?;
            let rows = stmt.query_map(params![query_str], Self::map_search_row).into_diagnostic()?;
            for row in rows {
                results.push(row.into_diagnostic()?);
            }
        }

        Ok(results)
    }

    pub fn is_file_changed(&self, path: &str, current_hash: &str) -> miette::Result<bool> {
        let mut stmt = self.conn.prepare("SELECT hash FROM indexed_files WHERE path = ?").into_diagnostic()?;
        let res: Result<String> = stmt.query_row([path], |row| row.get(0));
        
        match res {
            Ok(old_hash) => Ok(old_hash != current_hash),
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(true),
            Err(e) => Err(e).into_diagnostic(),
        }
    }

    pub fn mark_file_indexed(&self, path: &str, hash: &str) -> miette::Result<()> {
        self.conn.execute(
            "INSERT OR REPLACE INTO indexed_files (path, hash, last_indexed) VALUES (?, ?, CURRENT_TIMESTAMP)",
            [path, hash],
        ).into_diagnostic()?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_db_workflow() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let path = "test.json";
        let hash = "abc";
        assert!(db.is_file_changed(path, hash)?);
        
        db.mark_file_indexed(path, hash)?;
        assert!(!db.is_file_changed(path, hash)?);
        assert!(db.is_file_changed(path, "def")?);

        db.index_message(path, "user", "hola mundo rust", Some("project-x"))?;
        let results = db.search("hola", None)?;
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].content, "hola mundo rust");

        Ok(())
    }
}
