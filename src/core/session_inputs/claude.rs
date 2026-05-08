//! Native Claude session input adapter.
//!
//! Supports Claude JSON/JSONL session files, `sessions-index.json` project
//! inference, existing noise filtering, `tool_use`/`tool_result` block removal,
//! and optional `/subagents/` inclusion. It does not execute external commands or
//! parse non-Claude schemas.

use crate::core::ParsedFile;
use crate::core::sync::{discover_candidate_files, parse_session_file_claude};
use std::collections::HashMap;
use std::path::Path;

pub fn parse_paths(
    paths: &[String],
    include_agents: bool,
    existing_hashes: &HashMap<String, String>,
) -> Vec<ParsedFile> {
    let mut parsed = Vec::new();

    if paths.is_empty() {
        tracing::warn!("Claude input has no paths; skipping");
        return parsed;
    }

    for path in paths {
        let entries = discover_candidate_files(Path::new(path), include_agents);
        if entries.is_empty() {
            tracing::warn!("No files found for input path: {}", path);
            continue;
        }
        for entry in entries {
            if let Some(file) = parse_session_file_claude(entry, existing_hashes, include_agents) {
                parsed.push(file);
            }
        }
    }

    parsed
}
