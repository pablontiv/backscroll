pub(crate) mod chunking;
pub mod embedding;
pub(crate) mod hybrid;
pub mod models;
pub mod plans;
pub mod reader;
pub mod sources;
pub mod sync;
pub mod tagging;

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
    pub content_type: String,
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
    pub embedding_count: i64,
    pub embedding_model: Option<String>,
    pub source_breakdown: Vec<SourceCount>,
}

#[derive(Debug, Serialize)]
pub struct SourceCount {
    pub source: String,
    pub count: i64,
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

#[derive(Debug)]
pub struct PurgeStats {
    pub deleted_items: i64,
    pub deleted_files: i64,
    pub size_before: i64,
    pub size_after: i64,
}

#[derive(Debug)]
pub struct ValidationReport {
    pub orphaned_items: i64,
    pub stale_files: i64,
    pub fts_inconsistencies: i64,
    pub missing_embeddings: i64,
}

impl ValidationReport {
    pub fn total_issues(&self) -> i64 {
        self.orphaned_items + self.stale_files + self.fts_inconsistencies + self.missing_embeddings
    }
}

#[derive(Debug, Serialize)]
pub struct DailyActivity {
    pub date: String,
    pub sessions: i64,
    pub messages: i64,
}

#[derive(Debug, Serialize)]
pub struct TagCount {
    pub tag: String,
    pub count: i64,
}

#[derive(Debug, Serialize)]
pub struct InsightData {
    pub total_sessions: i64,
    pub total_messages: i64,
    pub daily_activity: Vec<DailyActivity>,
    pub tag_distribution: Vec<TagCount>,
}

#[derive(Debug)]
pub struct SearchParams {
    pub project: Option<String>,
    pub source: Option<String>,
    pub after: Option<String>,
    pub before: Option<String>,
    pub role: Option<String>,
    pub content_type: Option<String>,
    pub tag: Option<String>,
    pub limit: usize,
    pub offset: usize,
    // Hybrid search fields
    pub hybrid: bool,
    pub similarity_threshold: f32,
    pub top_k: usize,
    pub rrf_k: Option<usize>,
}

impl Default for SearchParams {
    fn default() -> Self {
        Self {
            project: None,
            source: None,
            after: None,
            before: None,
            role: None,
            content_type: None,
            tag: None,
            limit: 20,
            offset: 0,
            hybrid: true,
            similarity_threshold: 0.3,
            top_k: 50,
            rrf_k: None,
        }
    }
}

pub trait SearchEngine {
    fn sync_files(&self, files: Vec<ParsedFile>) -> miette::Result<()>;
    fn search(&self, query: &str, params: &SearchParams) -> miette::Result<Vec<SearchResult>>;
    fn get_file_hashes(&self) -> miette::Result<HashMap<String, String>>;
    fn clear_hashes(&self) -> miette::Result<()>;
    fn purge(&self, before: &str) -> miette::Result<PurgeStats>;
    fn get_stats(&self) -> miette::Result<Stats>;
    fn get_session_id(&self, source_path: &str) -> miette::Result<Option<String>>;
    fn get_topics(&self, project: Option<&str>, limit: usize) -> miette::Result<Vec<TopicEntry>>;
    fn list_sessions(
        &self,
        project: Option<&str>,
        limit: usize,
    ) -> miette::Result<Vec<SessionEntry>>;
    fn get_project_breakdown(&self) -> miette::Result<Vec<ProjectBreakdown>>;
    fn validate(&self) -> miette::Result<ValidationReport>;
    fn optimize_fts(&self) -> miette::Result<()>;
    fn get_insights(&self, project: Option<&str>) -> miette::Result<InsightData>;
}
