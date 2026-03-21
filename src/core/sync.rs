use crate::core::models::{MessageContent, SessionRecord};
use crate::core::{ParsedFile, ParsedMessage};
use miette::IntoDiagnostic;
use regex::Regex;
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::fs;
use std::path::Path;
use std::sync::LazyLock;
use walkdir::WalkDir;

static NOISE_TAG_PATTERNS: LazyLock<Vec<Regex>> = LazyLock::new(|| {
    [
        r"<system-reminder>[\s\S]*?</system-reminder>",
        r"<task-notification>[\s\S]*?</task-notification>",
        r"<caveat>[\s\S]*?</caveat>",
        r"<local-command-caveat>[\s\S]*?</local-command-caveat>",
        r"<command>[\s\S]*?</command>",
        r"<local-command-stdout>[\s\S]*?</local-command-stdout>",
        r"<command-name>[\s\S]*?</command-name>",
        r"<command-message>[\s\S]*?</command-message>",
        r"<command-args>[\s\S]*?</command-args>",
    ]
    .iter()
    .map(|p| Regex::new(p).expect("invalid noise tag pattern"))
    .collect()
});

static NOISE_LINE_PATTERNS: LazyLock<Vec<Regex>> = LazyLock::new(|| {
    [r"(?m)^Base directory:.*$", r"(?m)^Caveat:.*$"]
        .iter()
        .map(|p| Regex::new(p).expect("invalid noise line pattern"))
        .collect()
});

#[derive(serde::Deserialize)]
struct SessionIndexEntry {
    #[serde(rename = "projectPath")]
    project_path: Option<String>,
}

fn load_session_index(dir: &Path) -> HashMap<String, String> {
    let mut map = HashMap::new();
    let index_path = dir.join("sessions-index.json");
    if let Ok(content) = fs::read_to_string(index_path) {
        if let Ok(entries) = serde_json::from_str::<HashMap<String, SessionIndexEntry>>(&content) {
            for (session_id, entry) in entries {
                if let Some(p) = entry.project_path {
                    if let Some(slug) = Path::new(&p).file_name() {
                        map.insert(session_id, slug.to_string_lossy().to_string());
                    }
                }
            }
        }
    }
    map
}

pub fn filter_noise(text: &str) -> Option<String> {
    if text.contains("Request interrupted") {
        return None;
    }

    let mut result = text.to_string();

    for re in &*NOISE_TAG_PATTERNS {
        result = re.replace_all(&result, "").to_string();
    }

    for re in &*NOISE_LINE_PATTERNS {
        result = re.replace_all(&result, "").to_string();
    }

    let result = result.trim().to_string();
    if result.is_empty() {
        None
    } else {
        Some(result)
    }
}

pub fn compute_hash(path: impl AsRef<Path>) -> miette::Result<String> {
    let data = fs::read(path).into_diagnostic()?;
    let mut hasher = Sha256::new();
    hasher.update(data);
    Ok(hex::encode(hasher.finalize()))
}

#[tracing::instrument(skip(existing_hashes))]
pub fn parse_sessions(
    session_dir: &str,
    existing_hashes: &HashMap<String, String>,
    include_agents: bool,
) -> miette::Result<Vec<ParsedFile>> {
    let mut parsed_files = Vec::new();
    let mut files_processed = 0;
    let mut files_skipped = 0;

    let index_map = load_session_index(Path::new(session_dir));

    for entry in WalkDir::new(session_dir)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.file_type().is_file()
                && e.path()
                    .extension()
                    .is_some_and(|ext| ext == "json" || ext == "jsonl")
        })
    {
        let path_str = entry.path().to_string_lossy().to_string();

        if !include_agents && path_str.contains("/subagents/") {
            files_skipped += 1;
            continue;
        }
        let hash = compute_hash(entry.path())?;

        let is_changed = existing_hashes.get(&path_str) != Some(&hash);

        if is_changed {
            files_processed += 1;
            let content = fs::read_to_string(entry.path()).into_diagnostic()?;
            let mut messages = Vec::new();

            let mut file_uuid = None;

            for (ordinal, line) in content.lines().enumerate() {
                if let Ok(record) = serde_json::from_str::<SessionRecord>(line) {
                    if file_uuid.is_none() && record.uuid.is_some() {
                        file_uuid = record.uuid.clone();
                    }

                    if record.is_meta == Some(true) {
                        continue;
                    }

                    if record.record_type != "user" && record.record_type != "assistant" {
                        continue;
                    }

                    if let Some(msg) = record.message {
                        let (text_content, content_type) = match &msg.content {
                            MessageContent::Text(t) => (t.clone(), "text".to_string()),
                            MessageContent::Blocks(blocks) => {
                                let mut has_code = false;
                                let mut has_tool = false;
                                let parts: Vec<String> = blocks
                                    .iter()
                                    .filter(|b| {
                                        b.block_type != "tool_use" && b.block_type != "tool_result"
                                    })
                                    .filter_map(|b| {
                                        if b.block_type == "code" {
                                            has_code = true;
                                        }
                                        b.text.clone()
                                    })
                                    .collect();
                                for b in blocks {
                                    if b.block_type == "tool_use" || b.block_type == "tool_result" {
                                        has_tool = true;
                                    }
                                }
                                let ct = if has_code {
                                    "code"
                                } else if has_tool {
                                    "tool"
                                } else {
                                    "text"
                                };
                                (parts.join(" "), ct.to_string())
                            }
                        };

                        let cleaned_text = filter_noise(&text_content);

                        if let Some(text) = cleaned_text {
                            if !text.is_empty() {
                                messages.push(ParsedMessage {
                                    role: msg.role,
                                    text,
                                    ordinal,
                                    uuid: record.uuid,
                                    timestamp: record.timestamp,
                                    content_type,
                                });
                            }
                        }
                    }
                } else {
                    tracing::warn!("Could not parse line {} in {}", ordinal, path_str);
                }
            }

            let mut project = None;
            if let Some(uuid) = file_uuid {
                if let Some(p) = index_map.get(&uuid) {
                    project = Some(p.clone());
                }
            }
            if project.is_none() {
                // Fallback: derive from path
                // Look for a parent directory of the file, maybe up 2 levels if it's in `sessions`
                if let Some(parent) = entry.path().parent() {
                    if parent.ends_with("sessions") || parent.ends_with("subagents") {
                        if let Some(proj_dir) = parent.parent() {
                            if let Some(slug) = proj_dir.file_name() {
                                project = Some(slug.to_string_lossy().to_string());
                            }
                        }
                    } else if let Some(slug) = parent.file_name() {
                        project = Some(slug.to_string_lossy().to_string());
                    }
                }
            }

            let project_final = project.unwrap_or_else(|| "unknown".to_string());

            parsed_files.push(ParsedFile {
                source: "session".into(),
                source_path: path_str,
                hash,
                project: Some(project_final),
                messages,
            });
        } else {
            files_skipped += 1;
        }
    }
    tracing::info!(
        "Processed {} files, skipped {} files",
        files_processed,
        files_skipped
    );
    Ok(parsed_files)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_sync_workflow() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let file_path = dir.path().join("session.jsonl");

        fs::write(
            &file_path,
            r#"{"type": "user", "message": {"role": "user", "content": "hola"}}"#,
        )
        .into_diagnostic()?;

        let mut existing_hashes = HashMap::new();
        let files = parse_sessions(dir.path().to_str().unwrap(), &existing_hashes, false)?;

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].text, "hola");
        assert_eq!(files[0].messages[0].content_type, "text");

        // Simulate subsequent run
        existing_hashes.insert(files[0].source_path.clone(), files[0].hash.clone());
        let files2 = parse_sessions(dir.path().to_str().unwrap(), &existing_hashes, false)?;
        assert_eq!(files2.len(), 0);

        Ok(())
    }

    #[test]
    fn test_noise_filter_local_command_stdout() {
        // Non-empty stdout block is removed
        let input = "before<local-command-stdout>hook output here</local-command-stdout>after";
        assert_eq!(filter_noise(input), Some("beforeafter".to_string()));

        // Empty stdout block is removed
        let input = "keep this<local-command-stdout></local-command-stdout> too";
        assert_eq!(filter_noise(input), Some("keep this too".to_string()));

        // Multiline stdout block
        let input = "start<local-command-stdout>\nline1\nline2\n</local-command-stdout>end";
        assert_eq!(filter_noise(input), Some("startend".to_string()));
    }

    #[test]
    fn test_noise_filter_command_name_tags() {
        let input = "<command-name>foo</command-name><command-message>bar</command-message><command-args>baz</command-args>real content";
        assert_eq!(filter_noise(input), Some("real content".to_string()));

        // Each tag removed independently
        let input = "a<command-name>x</command-name>b<command-message>y</command-message>c";
        assert_eq!(filter_noise(input), Some("abc".to_string()));
    }

    #[test]
    fn test_noise_filter_caveat_prefix() {
        // Caveat: line at start is removed
        let input =
            "Caveat: The messages below were generated by local commands\nReal content here";
        assert_eq!(filter_noise(input), Some("Real content here".to_string()));

        // "the caveat is..." should NOT be removed (no Caveat: prefix)
        let input = "the caveat is important";
        assert_eq!(
            filter_noise(input),
            Some("the caveat is important".to_string())
        );

        // Mid-line Caveat: is NOT removed (pattern is anchored to line start)
        let input = "something Caveat: not at start";
        assert_eq!(
            filter_noise(input),
            Some("something Caveat: not at start".to_string())
        );
    }

    #[test]
    fn test_noise_filter_mixed_new_patterns() {
        let input = "<local-command-stdout>output</local-command-stdout>\n<command-name>clear</command-name>\nCaveat: ignore this\nUseful user message here";
        assert_eq!(
            filter_noise(input),
            Some("Useful user message here".to_string())
        );
    }
}
