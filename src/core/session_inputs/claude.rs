//! Native Claude session input adapter.
//!
//! Supports Claude JSON/JSONL session files, `sessions-index.json` project
//! inference, existing noise filtering, `tool_use`/`tool_result` block removal,
//! and optional `/subagents/` inclusion. It does not execute external commands or
//! parse non-Claude schemas.

use crate::core::ParsedFile;
use crate::core::sync::{discover_candidate_files, parse_session_file_claude};
use regex::Regex;
use std::collections::HashMap;
use std::path::Path;

fn normalize_path(path: &Path) -> String {
    path.to_string_lossy().replace('\\', "/")
}

fn push_escaped_regex_char(regex: &mut String, ch: char) {
    if matches!(
        ch,
        '.' | '+' | '(' | ')' | '|' | '^' | '$' | '{' | '}' | '[' | ']' | '\\'
    ) {
        regex.push('\\');
    }
    regex.push(ch);
}

fn glob_to_regex(pattern: &str) -> Regex {
    let mut regex = String::from('^');
    let mut chars = pattern.chars().peekable();

    while let Some(ch) = chars.next() {
        match ch {
            '*' => {
                if chars.peek() == Some(&'*') {
                    chars.next();
                    if chars.peek() == Some(&'/') {
                        chars.next();
                        regex.push_str("(?:.*/)?");
                    } else {
                        regex.push_str(".*");
                    }
                } else {
                    regex.push_str("[^/]*");
                }
            }
            '?' => regex.push_str("[^/]"),
            '{' => {
                let mut alternatives = Vec::new();
                let mut current = String::new();
                let mut closed = false;
                for inner in chars.by_ref() {
                    match inner {
                        '}' => {
                            alternatives.push(std::mem::take(&mut current));
                            closed = true;
                            break;
                        }
                        ',' => {
                            alternatives.push(std::mem::take(&mut current));
                        }
                        _ => current.push(inner),
                    }
                }

                if closed {
                    regex.push_str("(?:");
                    for (index, alternative) in alternatives.iter().enumerate() {
                        if index > 0 {
                            regex.push('|');
                        }
                        for alt_ch in alternative.chars() {
                            push_escaped_regex_char(&mut regex, alt_ch);
                        }
                    }
                    regex.push(')');
                } else {
                    regex.push_str("\\{");
                    for alt_ch in current.chars() {
                        push_escaped_regex_char(&mut regex, alt_ch);
                    }
                }
            }
            _ => push_escaped_regex_char(&mut regex, ch),
        }
    }

    regex.push('$');
    Regex::new(&regex).expect("generated glob regex must be valid")
}

fn matches_glob(entry_path: &Path, root_path: &Path, glob: Option<&str>) -> bool {
    let Some(glob) = glob else {
        return true;
    };

    let regex = glob_to_regex(&glob.replace('\\', "/"));
    if glob.contains('/') || glob.contains('\\') {
        let relative_path = entry_path.strip_prefix(root_path).unwrap_or(entry_path);
        return regex.is_match(&normalize_path(relative_path));
    }

    entry_path
        .file_name()
        .is_some_and(|file_name| regex.is_match(&file_name.to_string_lossy()))
}

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
    let mut parsed = Vec::new();

    if paths.is_empty() {
        tracing::warn!("Claude input has no paths; skipping");
        return parsed;
    }

    for path in paths {
        let root_path = Path::new(path);
        if !root_path.exists() {
            tracing::warn!("Session input path does not exist: {}", path);
            continue;
        }

        let entries: Vec<_> = discover_candidate_files(root_path, include_agents)
            .into_iter()
            .filter(|entry| matches_glob(entry.path(), root_path, glob))
            .collect();
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
