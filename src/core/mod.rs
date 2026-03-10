pub mod models;
pub mod plans;
pub mod reader;
pub mod sync;

use serde::Serialize;
use std::collections::HashMap;

#[derive(Debug, Serialize)]
pub struct SearchResult {
    pub source_path: String,
    pub text: String,
    pub match_snippet: Option<String>,
    pub score: f64,
    pub role: String,
    pub timestamp: Option<String>,
}

pub struct ParsedMessage {
    pub role: String,
    pub text: String,
    pub ordinal: usize,
    pub uuid: Option<String>,
    pub timestamp: Option<String>,
}

pub struct ParsedFile {
    pub source: String,
    pub source_path: String,
    pub hash: String,
    pub project: Option<String>,
    pub messages: Vec<ParsedMessage>,
}

#[derive(Debug, Serialize)]
pub struct Stats {
    pub file_count: i64,
    pub message_count: i64,
    pub db_size_bytes: i64,
    pub last_sync: Option<String>,
    pub project_count: i64,
}

#[derive(Debug, Serialize)]
pub struct TopicEntry {
    pub term: String,
    pub sessions: i64,
    pub mentions: i64,
}

#[derive(Debug, Serialize)]
pub struct SessionEntry {
    pub source_path: String,
    pub project: Option<String>,
    pub messages: i64,
    pub started: Option<String>,
    pub ended: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct ProjectBreakdown {
    pub project: Option<String>,
    pub sessions: i64,
    pub messages: i64,
}

#[derive(Debug, Default)]
pub struct SearchParams {
    pub project: Option<String>,
    pub source: Option<String>,
    pub after: Option<String>,
    pub before: Option<String>,
    pub role: Option<String>,
    pub limit: usize,
    pub offset: usize,
}

pub trait SearchEngine {
    fn sync_files(&self, files: Vec<ParsedFile>) -> miette::Result<()>;
    fn search(&self, query: &str, params: &SearchParams) -> miette::Result<Vec<SearchResult>>;
    fn get_file_hashes(&self) -> miette::Result<HashMap<String, String>>;
    fn clear_hashes(&self) -> miette::Result<()>;
    fn get_stats(&self) -> miette::Result<Stats>;
    fn get_session_id(&self, source_path: &str) -> miette::Result<Option<String>>;
    fn get_topics(&self, project: Option<&str>, limit: usize) -> miette::Result<Vec<TopicEntry>>;
    fn list_sessions(
        &self,
        project: Option<&str>,
        limit: usize,
    ) -> miette::Result<Vec<SessionEntry>>;
    fn get_project_breakdown(&self) -> miette::Result<Vec<ProjectBreakdown>>;
}
