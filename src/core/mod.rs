pub mod models;
pub mod sync;

use serde::Serialize;
use miette::Result;

#[derive(Debug, Serialize)]
pub struct SearchResult {
    pub path: String,
    pub content: String,
    pub score: f32,
}

pub trait SearchEngine {
    fn index_message(&self, path: &str, role: &str, content: &str, project: Option<&str>) -> Result<()>;
    fn search(&self, query: &str, project: Option<&str>) -> Result<Vec<SearchResult>>;
}
