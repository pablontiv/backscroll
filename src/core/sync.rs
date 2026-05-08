use crate::config::SessionInput;
use crate::core::models::{MessageContent, SessionRecord};
use crate::core::session_inputs::claude;
use crate::core::{ParsedFile, ParsedMessage};
use miette::IntoDiagnostic;
use regex::Regex;
use serde_json::Value;
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

fn infer_project(
    entry: &walkdir::DirEntry,
    file_uuid: Option<&String>,
    index_map: &HashMap<String, String>,
) -> String {
    if let Some(uuid) = file_uuid {
        if let Some(p) = index_map.get(uuid) {
            return p.clone();
        }
    }

    if let Some(parent) = entry.path().parent() {
        if parent.ends_with("sessions") || parent.ends_with("subagents") {
            if let Some(proj_dir) = parent.parent() {
                if let Some(slug) = proj_dir.file_name() {
                    return slug.to_string_lossy().to_string();
                }
            }
        } else if let Some(slug) = parent.file_name() {
            return slug.to_string_lossy().to_string();
        }
    }

    "unknown".to_string()
}

pub(crate) fn discover_candidate_files(
    session_dir: &Path,
    include_agents: bool,
) -> Vec<walkdir::DirEntry> {
    if !session_dir.exists() {
        return Vec::new();
    }

    let target_extensions = |path: &Path| {
        path.extension()
            .is_some_and(|ext| ext == "json" || ext == "jsonl")
    };

    if session_dir.is_file() {
        if !target_extensions(session_dir) {
            return Vec::new();
        }

        return WalkDir::new(session_dir)
            .into_iter()
            .filter_map(|e| e.ok())
            .filter(|entry| entry.file_type().is_file())
            .filter(|entry| {
                include_agents || !entry.path().to_string_lossy().contains("/subagents/")
            })
            .collect();
    }

    WalkDir::new(session_dir)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| {
            if !e.file_type().is_file() {
                return false;
            }
            let is_target_ext = target_extensions(e.path());
            if !is_target_ext {
                return false;
            }
            if !include_agents && e.path().to_string_lossy().contains("/subagents/") {
                return false;
            }
            true
        })
        .collect()
}

type ClaudeMessageLine = (
    String,
    String,
    String,
    usize,
    Option<String>,
    Option<String>,
);

fn parse_claude_message_lines(
    content: &str,
    file_uuid: &mut Option<String>,
) -> Vec<ClaudeMessageLine> {
    let mut out = Vec::new();

    for (ordinal, raw_line) in content.lines().enumerate() {
        match serde_json::from_str::<SessionRecord>(raw_line) {
            Ok(record) => {
                if file_uuid.is_none() && record.uuid.is_some() {
                    file_uuid.clone_from(&record.uuid);
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
                                    if b.block_type == "tool_use" || b.block_type == "tool_result" {
                                        has_tool = true;
                                    }
                                    b.text.clone()
                                })
                                .collect();

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

                    out.push((
                        msg.role,
                        text_content,
                        content_type,
                        ordinal,
                        record.uuid,
                        record.timestamp,
                    ));
                }
            }
            Err(_) => {
                tracing::warn!("Could not parse line {} in {}", ordinal, "session file");
            }
        }
    }

    out
}

pub(crate) fn parse_session_file_claude(
    entry: walkdir::DirEntry,
    existing_hashes: &HashMap<String, String>,
    include_agents: bool,
) -> Option<ParsedFile> {
    let path_str = entry.path().to_string_lossy().to_string();

    if !include_agents && path_str.contains("/subagents/") {
        return None;
    }

    let hash = match compute_hash(entry.path()) {
        Ok(hash) => hash,
        Err(err) => {
            tracing::warn!("Could not hash {}: {}", path_str, err);
            return None;
        }
    };

    if existing_hashes.get(&path_str) == Some(&hash) {
        return None;
    }

    let content = match fs::read_to_string(entry.path()) {
        Ok(c) => c,
        Err(err) => {
            tracing::warn!("Could not read {}: {}", path_str, err);
            return None;
        }
    };

    let mut messages = Vec::new();
    let mut file_uuid = None;

    for (role, text_content, content_type, ordinal, uuid, timestamp) in
        parse_claude_message_lines(&content, &mut file_uuid)
    {
        let cleaned_text = filter_noise(&text_content);
        if let Some(text) = cleaned_text {
            if !text.is_empty() {
                messages.push(ParsedMessage {
                    role,
                    text,
                    ordinal,
                    uuid,
                    timestamp,
                    content_type,
                });
            }
        }
    }

    let index_map = load_session_index(entry.path().parent().unwrap_or_else(|| Path::new(".")));
    let project = infer_project(&entry, file_uuid.as_ref(), &index_map);

    Some(ParsedFile {
        source: "session".into(),
        source_path: path_str,
        hash,
        project: Some(project),
        messages,
    })
}

fn parse_pi_value(value: &Value) -> Option<(String, String)> {
    let content = match value {
        Value::String(s) => return Some((s.clone(), "text".into())),
        Value::Array(arr) => {
            let mut has_code = false;
            let mut has_tool = false;
            let mut parts = Vec::new();

            for item in arr {
                if let Value::Object(obj) = item {
                    if let Some(block_type) = obj.get("type").and_then(Value::as_str) {
                        if block_type == "code" {
                            has_code = true;
                        }
                        if block_type == "tool_use" || block_type == "tool_result" {
                            has_tool = true;
                        }
                    }
                    if let Some(text) = obj.get("text").and_then(Value::as_str) {
                        parts.push(text.to_string());
                    }
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
        Value::Object(obj) => {
            if let Some(text) = obj.get("text").and_then(Value::as_str) {
                return Some((text.to_string(), "text".into()));
            }
            let blocks: Vec<String> = obj
                .get("blocks")
                .and_then(Value::as_array)
                .into_iter()
                .flat_map(|arr| {
                    arr.iter().filter_map(|item| {
                        let t = item.get("type").and_then(Value::as_str)?;
                        let _ = t;
                        item.get("text").and_then(Value::as_str).map(str::to_owned)
                    })
                })
                .collect();
            if blocks.is_empty() {
                ("".to_string(), "text".to_string())
            } else {
                (blocks.join(" "), "text".to_string())
            }
        }
        _ => return None,
    };

    Some(content)
}

fn parse_pi_file(
    entry: walkdir::DirEntry,
    existing_hashes: &HashMap<String, String>,
    _include_agents: bool,
) -> Option<ParsedFile> {
    let path_str = entry.path().to_string_lossy().to_string();
    let hash = match compute_hash(entry.path()) {
        Ok(hash) => hash,
        Err(err) => {
            tracing::warn!("Could not hash {}: {}", path_str, err);
            return None;
        }
    };

    if existing_hashes.get(&path_str) == Some(&hash) {
        return None;
    }

    let content = match fs::read_to_string(entry.path()) {
        Ok(c) => c,
        Err(err) => {
            tracing::warn!("Could not read {}: {}", path_str, err);
            return None;
        }
    };

    let mut messages = Vec::new();
    let mut file_uuid: Option<String> = None;

    for (ordinal, line) in content.lines().enumerate() {
        if line.trim().is_empty() {
            continue;
        }

        let value: Value = match serde_json::from_str(line) {
            Ok(v) => v,
            Err(_) => Value::String(line.to_string()),
        };

        let role = value
            .get("role")
            .and_then(Value::as_str)
            .or_else(|| {
                value
                    .get("message")
                    .and_then(|m| m.get("role"))
                    .and_then(Value::as_str)
            })
            .unwrap_or("assistant")
            .to_string();

        if file_uuid.is_none() {
            if let Some(u) = value.get("uuid").and_then(Value::as_str) {
                file_uuid = Some(u.to_string());
            }
            if let Some(u) = value.get("session_id").and_then(Value::as_str) {
                file_uuid = Some(u.to_string());
            }
        }

        let raw_content = value
            .get("message")
            .and_then(|m| m.get("content"))
            .or_else(|| value.get("content"));

        let (text_content, ct) = if let Some(raw) = raw_content {
            parse_pi_value(raw).unwrap_or_else(|| (raw.to_string(), "text".to_string()))
        } else if let Value::String(s) = &value {
            (s.clone(), "text".to_string())
        } else {
            (String::new(), "text".to_string())
        };

        if let Some(cleaned) = filter_noise(&text_content) {
            if cleaned.is_empty() {
                continue;
            }
            let timestamp = value
                .get("timestamp")
                .and_then(Value::as_str)
                .map(std::string::ToString::to_string);

            let uuid = value
                .get("uuid")
                .and_then(Value::as_str)
                .map(std::string::ToString::to_string);

            messages.push(ParsedMessage {
                role,
                text: cleaned,
                ordinal,
                uuid,
                timestamp,
                content_type: ct,
            });
        }
    }

    let project = infer_project(
        &entry,
        file_uuid.as_ref(),
        &load_session_index(entry.path().parent().unwrap_or_else(|| Path::new("."))),
    );

    Some(ParsedFile {
        source: "session".into(),
        source_path: path_str,
        hash,
        project: Some(project),
        messages,
    })
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct SessionInputParserIdentity {
    source: String,
    parser: String,
}

impl SessionInputParserIdentity {
    pub fn new(source: &str, parser: &str) -> Self {
        Self {
            source: source.to_lowercase(),
            parser: parser.to_lowercase(),
        }
    }

    fn from_input(input: &SessionInput) -> Self {
        Self::new(&input.source, input.parser())
    }
}

pub trait SessionInputParser {
    fn source(&self) -> &'static str;
    fn parser(&self) -> &'static str;
    fn parse(
        &self,
        input: &SessionInput,
        existing_hashes: &HashMap<String, String>,
    ) -> Vec<ParsedFile>;
}

pub struct SessionInputParserRegistry {
    parsers: HashMap<SessionInputParserIdentity, Box<dyn SessionInputParser>>,
}

impl Default for SessionInputParserRegistry {
    fn default() -> Self {
        let mut registry = Self::new();
        registry.register(Box::new(ClaudeInputParser));
        registry.register(Box::new(PiInputParser));
        registry
    }
}

impl SessionInputParserRegistry {
    pub fn new() -> Self {
        Self {
            parsers: HashMap::new(),
        }
    }

    pub fn register(&mut self, parser: Box<dyn SessionInputParser>) {
        let identity = SessionInputParserIdentity::new(parser.source(), parser.parser());
        self.parsers.insert(identity, parser);
    }

    pub fn parse_input(
        &self,
        input: &SessionInput,
        existing_hashes: &HashMap<String, String>,
    ) -> Vec<ParsedFile> {
        let identity = SessionInputParserIdentity::from_input(input);
        match self.parsers.get(&identity) {
            Some(parser) => parser.parse(input, existing_hashes),
            None => {
                tracing::warn!(
                    "No parser registered for source='{}' parser='{}'. Skipping input in {:?}",
                    input.source,
                    input.parser(),
                    input.paths
                );
                Vec::new()
            }
        }
    }
}

struct ClaudeInputParser;

impl SessionInputParser for ClaudeInputParser {
    fn source(&self) -> &'static str {
        "session"
    }

    fn parser(&self) -> &'static str {
        "claude"
    }

    fn parse(
        &self,
        input: &SessionInput,
        existing_hashes: &HashMap<String, String>,
    ) -> Vec<ParsedFile> {
        claude::parse_paths(&input.paths, input.include_agents, existing_hashes)
    }
}

struct PiInputParser;

impl SessionInputParser for PiInputParser {
    fn source(&self) -> &'static str {
        "session"
    }

    fn parser(&self) -> &'static str {
        "pi"
    }

    fn parse(
        &self,
        input: &SessionInput,
        existing_hashes: &HashMap<String, String>,
    ) -> Vec<ParsedFile> {
        let mut parsed = Vec::new();

        if input.paths.is_empty() {
            tracing::warn!("Input {:?} has no paths; skipping", input);
            return parsed;
        }

        for path in &input.paths {
            let mut any_found = false;
            let entries = discover_candidate_files(Path::new(path), input.include_agents);
            for entry in entries {
                if let Some(file) = parse_pi_file(entry, existing_hashes, input.include_agents) {
                    parsed.push(file);
                    any_found = true;
                }
            }
            if !any_found {
                tracing::warn!("No files found for input path: {}", path);
            }
        }

        parsed
    }
}

pub fn parse_session_inputs(
    inputs: &[SessionInput],
    existing_hashes: &HashMap<String, String>,
) -> Vec<ParsedFile> {
    let registry = SessionInputParserRegistry::default();
    let mut parsed = Vec::new();

    for input in inputs {
        if !input.is_active() {
            continue;
        }
        parsed.extend(registry.parse_input(input, existing_hashes));
    }

    parsed
}

#[tracing::instrument(skip(existing_hashes))]
pub fn parse_sessions(
    session_dir: &str,
    existing_hashes: &HashMap<String, String>,
    include_agents: bool,
) -> miette::Result<Vec<ParsedFile>> {
    Ok(claude::parse_paths(
        &[session_dir.to_string()],
        include_agents,
        existing_hashes,
    ))
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
    fn test_parse_session_inputs_pi() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let file_path = dir.path().join("pi.jsonl");

        fs::write(
            &file_path,
            r#"{"role":"assistant","content":"pi content","uuid":"u1","timestamp":"2024-01-01T00:00:00Z"}"#,
        )
        .into_diagnostic()?;

        let input = SessionInput {
            source: "session".into(),
            parser: "pi".into(),
            paths: vec![dir.path().to_str().unwrap().into()],
            glob: None,
            include_agents: false,
            active: true,
        };

        let files = parse_session_inputs(&[input], &HashMap::new());
        assert_eq!(files.len(), 1);
        assert_eq!(files[0].source, "session");
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].text, "pi content");

        Ok(())
    }

    #[test]
    fn test_session_input_parser_selection_uses_source_and_parser() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let file_path = dir.path().join("session.jsonl");

        fs::write(
            &file_path,
            r#"{"type":"user","message":{"role":"user","content":"selected"}}"#,
        )
        .into_diagnostic()?;

        let matching_input = SessionInput {
            source: "session".into(),
            parser: "claude".into(),
            paths: vec![dir.path().to_str().unwrap().into()],
            glob: None,
            include_agents: false,
            active: true,
        };
        let wrong_source_input = SessionInput {
            source: "unknown".into(),
            parser: "claude".into(),
            paths: vec![dir.path().to_str().unwrap().into()],
            glob: None,
            include_agents: false,
            active: true,
        };

        let files = parse_session_inputs(&[matching_input, wrong_source_input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].text, "selected");

        Ok(())
    }

    #[test]
    fn test_session_input_invalid_file_does_not_break_batch() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(dir.path().join("invalid.jsonl"), b"\xFF\xFE\xFD").into_diagnostic()?;
        fs::write(
            dir.path().join("valid.jsonl"),
            r#"{"type":"user","message":{"role":"user","content":"still parsed"}}"#,
        )
        .into_diagnostic()?;

        let input = SessionInput {
            source: "session".into(),
            parser: "claude".into(),
            paths: vec![dir.path().to_str().unwrap().into()],
            glob: None,
            include_agents: false,
            active: true,
        };

        let files = parse_session_inputs(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].text, "still parsed");

        Ok(())
    }

    #[test]
    fn test_claude_input_parser_matches_parse_sessions() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let file_path = dir.path().join("session.jsonl");

        fs::write(
            &file_path,
            r#"{"uuid":"u1","timestamp":"2024-01-01T00:00:00Z","type":"user","message":{"role":"user","content":"hola"}}
{"uuid":"u2","timestamp":"2024-01-01T00:00:01Z","type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"respuesta"}]}}"#,
        )
        .into_diagnostic()?;

        let hashes = HashMap::new();
        let legacy = parse_sessions(dir.path().to_str().unwrap(), &hashes, false)?;
        let native = crate::core::session_inputs::claude::parse_paths(
            &[dir.path().to_string_lossy().to_string()],
            false,
            &hashes,
        );

        assert_eq!(native.len(), legacy.len());
        assert_eq!(native[0].source, legacy[0].source);
        assert_eq!(native[0].source_path, legacy[0].source_path);
        assert_eq!(native[0].hash, legacy[0].hash);
        assert_eq!(native[0].project, legacy[0].project);
        assert_eq!(native[0].messages.len(), legacy[0].messages.len());
        for (native_msg, legacy_msg) in native[0].messages.iter().zip(&legacy[0].messages) {
            assert_eq!(native_msg.role, legacy_msg.role);
            assert_eq!(native_msg.text, legacy_msg.text);
            assert_eq!(native_msg.ordinal, legacy_msg.ordinal);
            assert_eq!(native_msg.uuid, legacy_msg.uuid);
            assert_eq!(native_msg.timestamp, legacy_msg.timestamp);
            assert_eq!(native_msg.content_type, legacy_msg.content_type);
        }

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
