pub mod models;
pub mod sync;

use miette::Result;
use serde::Serialize;

#[derive(Debug, Serialize)]
pub struct SearchResult {
    pub path: String,
    pub content: String,
    pub score: f32,
}

#[allow(dead_code)]
pub trait SearchEngine {
    fn index_message(
        &self,
        path: &str,
        role: &str,
        content: &str,
        project: Option<&str>,
    ) -> Result<()>;
    fn search(&self, query: &str, project: Option<&str>) -> Result<Vec<SearchResult>>;
}
