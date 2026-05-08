//! Native Claude session input adapter.
//!
//! Supports Claude JSON/JSONL session files, `sessions-index.json` project
//! inference, existing noise filtering, `tool_use`/`tool_result` block removal,
//! and optional `/subagents/` inclusion. It does not execute external commands or
//! parse non-Claude schemas.

use crate::core::ParsedFile;
use crate::core::sync::{discover_candidate_files, parse_session_file_claude};
use crate::input_config::DiscoverConfig;
use std::collections::HashMap;

pub fn parse_paths(
    paths: &[String],
    include_agents: bool,
    existing_hashes: &HashMap<String, String>,
) -> Vec<ParsedFile> {
    parse_paths_with_glob(paths, None, include_agents, existing_hashes)
}

pub fn parse_paths_with_glob(
    paths: &[String],
    glob: Option<&str>,
    include_agents: bool,
    existing_hashes: &HashMap<String, String>,
) -> Vec<ParsedFile> {
    let discover = DiscoverConfig {
        roots: paths.to_vec(),
        include: glob.map_or_else(crate::input_config::default_discover_include, |pattern| {
            vec![pattern.to_string()]
        }),
        exclude: if include_agents {
            Vec::new()
        } else {
            crate::input_config::default_discover_exclude()
        },
        follow_symlinks: false,
    };

    parse_discovered_paths(&discover, existing_hashes)
}

pub fn parse_discovered_paths(
    discover: &DiscoverConfig,
    existing_hashes: &HashMap<String, String>,
) -> Vec<ParsedFile> {
    let mut parsed = Vec::new();

    if discover.roots.is_empty() {
        tracing::warn!("Claude input has no paths; skipping");
        return parsed;
    }

    let mut entries = Vec::new();
    for root in &discover.roots {
        let mut single_root = discover.clone();
        single_root.roots = vec![root.clone()];
        match discover_candidate_files(&single_root) {
            Ok(root_entries) => entries.extend(root_entries),
            Err(err) => {
                tracing::warn!("Could not discover Claude input files in {}: {}", root, err);
            }
        }
    }
    entries.sort_by(|a, b| a.to_string_lossy().cmp(&b.to_string_lossy()));
    entries.dedup();

    if entries.is_empty() {
        tracing::warn!("No files found for input paths: {:?}", discover.roots);
        return parsed;
    }

    for path in entries {
        if let Some(file) = parse_session_file_claude(&path, existing_hashes) {
            parsed.push(file);
        }
    }

    parsed
}
