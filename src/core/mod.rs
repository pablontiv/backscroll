pub mod models;
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
}

pub struct ParsedMessage {
    pub role: String,
    pub text: String,
    pub ordinal: usize,
    pub uuid: Option<String>,
    pub timestamp: Option<String>,
}

pub struct ParsedFile {
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

pub trait SearchEngine {
    fn sync_files(&self, files: Vec<ParsedFile>) -> miette::Result<()>;
    fn search(&self, query: &str, project: &Option<String>) -> miette::Result<Vec<SearchResult>>;
    fn get_file_hashes(&self) -> miette::Result<HashMap<String, String>>;
    fn get_stats(&self) -> miette::Result<Stats>;
    fn get_session_id(&self, source_path: &str) -> miette::Result<Option<String>>;
}
