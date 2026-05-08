use crate::core::models::{MessageContent, SessionRecord};
use crate::core::session_inputs::claude;
use crate::core::{ParsedFile, ParsedMessage};
use crate::input_config::{
    DecodeFormat, DiscoverConfig, InputDefinition, Predicate, PredicateOp, PredicateValue,
    RemoveKind, SessionInput,
};
use globset::{Glob, GlobSet, GlobSetBuilder};
use miette::IntoDiagnostic;
use regex::Regex;
use serde_json::Value;
use serde_json_path::JsonPath;
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};
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
    path: &Path,
    file_uuid: Option<&String>,
    index_map: &HashMap<String, String>,
) -> String {
    if let Some(uuid) = file_uuid {
        if let Some(p) = index_map.get(uuid) {
            return p.clone();
        }
    }

    if let Some(parent) = path.parent() {
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

fn add_glob(builder: &mut GlobSetBuilder, pattern: &str) -> miette::Result<()> {
    let glob = Glob::new(pattern)
        .map_err(|err| miette::miette!("Invalid discovery glob '{}': {}", pattern, err))?;
    builder.add(glob);

    if let Some(stripped) = pattern.strip_prefix("**/") {
        let glob = Glob::new(stripped)
            .map_err(|err| miette::miette!("Invalid discovery glob '{}': {}", pattern, err))?;
        builder.add(glob);
    }

    Ok(())
}

fn build_glob_set(patterns: &[String], field: &str) -> miette::Result<GlobSet> {
    let mut builder = GlobSetBuilder::new();
    for pattern in patterns {
        add_glob(&mut builder, pattern).map_err(|err| {
            miette::miette!("Failed to build {} pattern '{}': {}", field, pattern, err)
        })?;
    }
    builder
        .build()
        .map_err(|err| miette::miette!("Failed to build {} globset: {}", field, err))
}

fn relative_candidate_path(root: &Path, candidate: &Path) -> PathBuf {
    if root.is_file() {
        return candidate
            .file_name()
            .map_or_else(|| candidate.to_path_buf(), PathBuf::from);
    }

    candidate
        .strip_prefix(root)
        .map_or_else(|_| candidate.to_path_buf(), Path::to_path_buf)
}

fn matches_discovery_globs(set: &GlobSet, root: &Path, candidate: &Path) -> bool {
    let relative = relative_candidate_path(root, candidate);
    set.is_match(&relative) || set.is_match(candidate)
}

pub(crate) fn discover_candidate_files(discover: &DiscoverConfig) -> miette::Result<Vec<PathBuf>> {
    if discover.include.is_empty() {
        return Err(miette::miette!(
            "Discovery requires at least one discover.include glob"
        ));
    }

    let include = build_glob_set(&discover.include, "discover.include")?;
    let exclude = build_glob_set(&discover.exclude, "discover.exclude")?;
    let mut candidates = Vec::new();

    for raw_root in &discover.roots {
        let root = Path::new(raw_root);
        if !root.exists() {
            return Err(miette::miette!(
                "Discovery root does not exist in discover.roots: {}",
                root.display()
            ));
        }

        for entry in WalkDir::new(root)
            .follow_links(discover.follow_symlinks)
            .into_iter()
        {
            let entry = entry.map_err(|err| {
                miette::miette!("Failed to walk discovery root {}: {}", root.display(), err)
            })?;
            if !entry.file_type().is_file() {
                continue;
            }

            let path = entry.path();
            if matches_discovery_globs(&include, root, path)
                && !matches_discovery_globs(&exclude, root, path)
            {
                candidates.push(path.to_path_buf());
            }
        }
    }

    candidates.sort_by(|a, b| a.to_string_lossy().cmp(&b.to_string_lossy()));
    candidates.dedup();
    Ok(candidates)
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
    path: &Path,
    existing_hashes: &HashMap<String, String>,
) -> Option<ParsedFile> {
    let path_str = path.to_string_lossy().to_string();

    let hash = match compute_hash(path) {
        Ok(hash) => hash,
        Err(err) => {
            tracing::warn!("Could not hash {}: {}", path_str, err);
            return None;
        }
    };

    if existing_hashes.get(&path_str) == Some(&hash) {
        return None;
    }

    let content = match fs::read_to_string(path) {
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

    let index_map = load_session_index(path.parent().unwrap_or_else(|| Path::new(".")));
    let project = infer_project(path, file_uuid.as_ref(), &index_map);

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

fn parse_pi_file(path: &Path, existing_hashes: &HashMap<String, String>) -> Option<ParsedFile> {
    let path_str = path.to_string_lossy().to_string();
    let hash = match compute_hash(path) {
        Ok(hash) => hash,
        Err(err) => {
            tracing::warn!("Could not hash {}: {}", path_str, err);
            return None;
        }
    };

    if existing_hashes.get(&path_str) == Some(&hash) {
        return None;
    }

    let content = match fs::read_to_string(path) {
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
        path,
        file_uuid.as_ref(),
        &load_session_index(path.parent().unwrap_or_else(|| Path::new("."))),
    );

    Some(ParsedFile {
        source: "session".into(),
        source_path: path_str,
        hash,
        project: Some(project),
        messages,
    })
}

fn parse_jsonpath(selector: &str, input_id: &str, field: &str) -> Option<JsonPath> {
    match JsonPath::parse(selector) {
        Ok(path) => Some(path),
        Err(err) => {
            tracing::warn!(
                "Input '{}' has invalid {} JSONPath selector '{}': {}",
                input_id,
                field,
                selector,
                err
            );
            None
        }
    }
}

fn query_nodes<'a>(path: &JsonPath, value: &'a Value) -> Vec<&'a Value> {
    path.query(value).all()
}

fn value_to_field_string(value: &Value) -> Option<String> {
    match value {
        Value::String(value) => Some(value.clone()),
        Value::Number(value) => Some(value.to_string()),
        Value::Bool(value) => Some(value.to_string()),
        Value::Null | Value::Array(_) | Value::Object(_) => None,
    }
}

fn select_first_string(
    selector: &str,
    value: &Value,
    input_id: &str,
    field: &str,
) -> Option<String> {
    let path = parse_jsonpath(selector, input_id, field)?;
    query_nodes(&path, value)
        .into_iter()
        .find_map(value_to_field_string)
}

fn select_optional_string(
    selector: Option<&String>,
    value: &Value,
    input_id: &str,
    field: &str,
) -> Option<String> {
    selector.and_then(|selector| select_first_string(selector, value, input_id, field))
}

fn aggregate_content_type(types: &[String], default: &str) -> String {
    if types.iter().any(|value| value == "code") {
        return "code".to_string();
    }
    if types
        .iter()
        .any(|value| matches!(value.as_str(), "tool" | "tool_use" | "tool_result"))
    {
        return "tool".to_string();
    }
    types
        .iter()
        .find(|value| !value.trim().is_empty())
        .cloned()
        .unwrap_or_else(|| default.to_string())
}

fn selected_strings(path: &JsonPath, value: &Value) -> Vec<String> {
    query_nodes(path, value)
        .into_iter()
        .filter_map(value_to_field_string)
        .collect()
}

fn predicate_value_to_json(value: &PredicateValue) -> Option<Value> {
    match value {
        PredicateValue::String(value) => Some(Value::String(value.clone())),
        PredicateValue::Bool(value) => Some(Value::Bool(*value)),
        PredicateValue::Integer(value) => Some(Value::Number((*value).into())),
        PredicateValue::Float(value) => serde_json::Number::from_f64(*value).map(Value::Number),
        PredicateValue::Array(values) => values
            .iter()
            .map(predicate_value_to_json)
            .collect::<Option<Vec<_>>>()
            .map(Value::Array),
    }
}

fn predicate_matches(input_id: &str, predicate: &Predicate, subject: &Value, field: &str) -> bool {
    let Some(path) = parse_jsonpath(&predicate.selector, input_id, field) else {
        return false;
    };
    let nodes = query_nodes(&path, subject);

    match predicate.op {
        PredicateOp::Exists => !nodes.is_empty(),
        PredicateOp::Missing => nodes.is_empty(),
        PredicateOp::Eq => predicate.value.as_ref().is_some_and(|expected| {
            predicate_value_to_json(expected)
                .as_ref()
                .is_some_and(|expected| nodes.contains(&expected))
        }),
        PredicateOp::Ne => predicate.value.as_ref().is_none_or(|expected| {
            predicate_value_to_json(expected)
                .as_ref()
                .is_none_or(|expected| {
                    nodes.is_empty() || nodes.iter().all(|value| *value != expected)
                })
        }),
        PredicateOp::In => {
            let Some(PredicateValue::Array(values)) = predicate.value.as_ref() else {
                tracing::warn!(
                    "Input '{}' predicate '{}' uses op='in' without an array value",
                    input_id,
                    field
                );
                return false;
            };
            let expected_values: Vec<Value> =
                values.iter().filter_map(predicate_value_to_json).collect();
            nodes
                .iter()
                .any(|value| expected_values.iter().any(|expected| *value == expected))
        }
    }
}

fn predicates_match_all(
    input_id: &str,
    predicates: &[Predicate],
    subject: &Value,
    field: &str,
) -> bool {
    predicates
        .iter()
        .all(|predicate| predicate_matches(input_id, predicate, subject, field))
}

fn predicates_match_any(
    input_id: &str,
    predicates: &[Predicate],
    subject: &Value,
    field: &str,
) -> bool {
    predicates
        .iter()
        .any(|predicate| predicate_matches(input_id, predicate, subject, field))
}

fn record_passes_predicates(input: &InputDefinition, record: &Value, ordinal: usize) -> bool {
    if !predicates_match_all(
        &input.id,
        &input.record.include_when,
        record,
        "record.include_when",
    ) {
        tracing::debug!(
            input_id = %input.id,
            ordinal,
            "Dropping record because record.include_when did not match"
        );
        return false;
    }
    if predicates_match_any(
        &input.id,
        &input.record.exclude_when,
        record,
        "record.exclude_when",
    ) {
        tracing::debug!(
            input_id = %input.id,
            ordinal,
            "Dropping record because record.exclude_when matched"
        );
        return false;
    }
    true
}

fn content_block_passes_predicates(
    input: &InputDefinition,
    block: &Value,
    ordinal: usize,
    block_index: usize,
) -> bool {
    if !predicates_match_all(
        &input.id,
        &input.content.include_when,
        block,
        "content.include_when",
    ) {
        tracing::debug!(
            input_id = %input.id,
            ordinal,
            block_index,
            "Dropping content block because content.include_when did not match"
        );
        return false;
    }
    if predicates_match_any(
        &input.id,
        &input.content.exclude_when,
        block,
        "content.exclude_when",
    ) {
        tracing::debug!(
            input_id = %input.id,
            ordinal,
            block_index,
            "Dropping content block because content.exclude_when matched"
        );
        return false;
    }
    true
}

fn extract_content_from_record(
    input: &InputDefinition,
    record: &Value,
    ordinal: usize,
) -> Option<(String, String)> {
    let content_selector = parse_jsonpath(&input.content.selector, &input.id, "content.selector")?;
    let content_nodes = query_nodes(&content_selector, record);
    if content_nodes.is_empty() {
        return None;
    }

    let mut parts = Vec::new();
    let mut content_types = Vec::new();

    if let Some(blocks_selector) = &input.content.blocks {
        if let Some(blocks_path) = parse_jsonpath(blocks_selector, &input.id, "content.blocks") {
            let block_nodes = query_nodes(&blocks_path, record);
            if !block_nodes.is_empty() {
                let block_text_path =
                    input.content.block_text.as_ref().and_then(|selector| {
                        parse_jsonpath(selector, &input.id, "content.block_text")
                    });
                let content_type_path = input.content.content_type.as_ref().and_then(|selector| {
                    parse_jsonpath(selector, &input.id, "content.content_type")
                });

                for (block_index, block) in block_nodes.into_iter().enumerate() {
                    if !content_block_passes_predicates(input, block, ordinal, block_index) {
                        continue;
                    }
                    if let Some(path) = &block_text_path {
                        parts.extend(selected_strings(path, block));
                    } else if let Some(text) = value_to_field_string(block) {
                        parts.push(text);
                    }
                    if let Some(path) = &content_type_path {
                        content_types.extend(selected_strings(path, block));
                    }
                }

                if !parts.is_empty() {
                    let text = parts.join(&input.text.join);
                    return Some((
                        text,
                        aggregate_content_type(&content_types, &input.content.default_content_type),
                    ));
                }
            }
        }
    }

    let string_path = parse_jsonpath(&input.content.string, &input.id, "content.string")?;
    let content_type_path = input
        .content
        .content_type
        .as_ref()
        .and_then(|selector| parse_jsonpath(selector, &input.id, "content.content_type"));

    for content in content_nodes {
        parts.extend(selected_strings(&string_path, content));
        if let Some(path) = &content_type_path {
            content_types.extend(selected_strings(path, content));
        }
    }

    if content_types.is_empty() {
        if let Some(path) = &content_type_path {
            content_types.extend(selected_strings(path, record));
        }
    }

    Some((
        parts.join(&input.text.join),
        aggregate_content_type(&content_types, &input.content.default_content_type),
    ))
}

fn normalize_extracted_text(input: &InputDefinition, text: &str) -> Option<String> {
    let mut normalized = text.to_string();

    for rule in &input.text.remove {
        match rule.kind {
            RemoveKind::Regex => match Regex::new(&rule.pattern) {
                Ok(regex) => {
                    normalized = regex.replace_all(&normalized, "").to_string();
                }
                Err(err) => tracing::warn!(
                    input_id = %input.id,
                    pattern = %rule.pattern,
                    "Ignoring invalid text.remove regex: {}",
                    err
                ),
            },
            RemoveKind::Prefix => {
                if let Some(stripped) = normalized.strip_prefix(&rule.pattern) {
                    normalized = stripped.to_string();
                }
            }
            RemoveKind::Suffix => {
                if let Some(stripped) = normalized.strip_suffix(&rule.pattern) {
                    normalized = stripped.to_string();
                }
            }
        }
    }

    if input.text.trim {
        normalized = normalized.trim().to_string();
    }

    if input.text.drop_empty && normalized.is_empty() {
        None
    } else {
        Some(normalized)
    }
}

fn parsed_message_from_record(
    input: &InputDefinition,
    record: &Value,
    ordinal: usize,
) -> Option<ParsedMessage> {
    let Some(raw_role) = select_first_string(&input.mapping.role, record, &input.id, "map.role")
    else {
        tracing::debug!(
            input_id = %input.id,
            ordinal,
            "Dropping message because map.role did not yield a scalar value"
        );
        return None;
    };
    let role = input
        .mapping
        .role_aliases
        .get(&raw_role)
        .cloned()
        .unwrap_or(raw_role);
    let Some((raw_text, content_type)) = extract_content_from_record(input, record, ordinal) else {
        tracing::debug!(
            input_id = %input.id,
            ordinal,
            "Dropping message because content selectors did not yield text"
        );
        return None;
    };
    let Some(text) = normalize_extracted_text(input, &raw_text) else {
        tracing::debug!(
            input_id = %input.id,
            ordinal,
            "Dropping message because text normalization produced an empty message"
        );
        return None;
    };

    Some(ParsedMessage {
        role,
        text,
        ordinal,
        uuid: select_optional_string(input.mapping.uuid.as_ref(), record, &input.id, "map.uuid"),
        timestamp: select_optional_string(
            input.mapping.timestamp.as_ref(),
            record,
            &input.id,
            "map.timestamp",
        ),
        content_type,
    })
}

fn parse_records_from_value<'a>(
    input: &InputDefinition,
    value: &'a Value,
    field: &str,
) -> Option<Vec<&'a Value>> {
    let record_path = parse_jsonpath(&input.record.selector, &input.id, field)?;
    Some(query_nodes(&record_path, value))
}

fn parse_generic_jsonl_content(
    input: &InputDefinition,
    content: &str,
) -> (Vec<ParsedMessage>, Option<String>, usize) {
    let mut messages = Vec::new();
    let mut project = None;
    let mut data_errors = 0;

    for (line_number, line) in content.lines().enumerate() {
        if line.trim().is_empty() {
            continue;
        }
        let value = match serde_json::from_str::<Value>(line) {
            Ok(value) => value,
            Err(err) => {
                data_errors += 1;
                tracing::warn!(
                    "Skipping invalid JSONL line {} for input '{}': {}",
                    line_number + 1,
                    input.id,
                    err
                );
                continue;
            }
        };
        let Some(records) = parse_records_from_value(input, &value, "record.selector") else {
            data_errors += 1;
            continue;
        };
        for record in records {
            if !record_passes_predicates(input, record, line_number) {
                continue;
            }
            if project.is_none() {
                project = select_optional_string(
                    input.mapping.project.as_ref(),
                    record,
                    &input.id,
                    "map.project",
                );
            }
            match parsed_message_from_record(input, record, line_number) {
                Some(message) => messages.push(message),
                None => data_errors += 1,
            }
        }
    }

    (messages, project, data_errors)
}

fn parse_generic_json_content(
    input: &InputDefinition,
    content: &str,
) -> (Vec<ParsedMessage>, Option<String>, usize) {
    let value = match serde_json::from_str::<Value>(content) {
        Ok(value) => value,
        Err(err) => {
            tracing::warn!(
                "Skipping invalid JSON file for input '{}': {}",
                input.id,
                err
            );
            return (Vec::new(), None, 1);
        }
    };
    let Some(records) = parse_records_from_value(input, &value, "record.selector") else {
        return (Vec::new(), None, 1);
    };

    let mut messages = Vec::new();
    let mut project = None;
    let mut data_errors = 0;
    for (ordinal, record) in records.into_iter().enumerate() {
        if !record_passes_predicates(input, record, ordinal) {
            continue;
        }
        if project.is_none() {
            project = select_optional_string(
                input.mapping.project.as_ref(),
                record,
                &input.id,
                "map.project",
            );
        }
        match parsed_message_from_record(input, record, ordinal) {
            Some(message) => messages.push(message),
            None => data_errors += 1,
        }
    }

    (messages, project, data_errors)
}

fn parse_generic_input_file(
    input: &InputDefinition,
    path: &Path,
    existing_hashes: &HashMap<String, String>,
) -> Option<ParsedFile> {
    let path_str = path.to_string_lossy().to_string();
    let hash = match compute_hash(path) {
        Ok(hash) => hash,
        Err(err) => {
            tracing::warn!("Could not hash {}: {}", path_str, err);
            return None;
        }
    };

    if existing_hashes.get(&path_str) == Some(&hash) {
        return None;
    }

    let content = match fs::read_to_string(path) {
        Ok(content) => content,
        Err(err) => {
            tracing::warn!("Could not read {}: {}", path_str, err);
            return None;
        }
    };

    let (messages, project, data_errors) = match input.decode.format {
        DecodeFormat::Jsonl => parse_generic_jsonl_content(input, &content),
        DecodeFormat::Json => parse_generic_json_content(input, &content),
    };

    if data_errors > 0 {
        tracing::warn!(
            "Skipped {} invalid records while parsing {} for input '{}'",
            data_errors,
            path_str,
            input.id
        );
    }

    Some(ParsedFile {
        source: input.source.clone(),
        source_path: path_str,
        hash,
        project: project.or_else(|| Some("unknown".to_string())),
        messages,
    })
}

fn parse_generic_input_definition(
    input: &InputDefinition,
    existing_hashes: &HashMap<String, String>,
) -> Vec<ParsedFile> {
    if !input.active {
        return Vec::new();
    }

    let mut entries = Vec::new();
    for root in &input.discover.roots {
        let mut single_root = input.discover.clone();
        single_root.roots = vec![root.clone()];
        match discover_candidate_files(&single_root) {
            Ok(root_entries) => entries.extend(root_entries),
            Err(err) => tracing::warn!(
                "Could not discover files for input '{}' in {}: {}",
                input.id,
                root,
                err
            ),
        }
    }
    entries.sort_by(|a, b| a.to_string_lossy().cmp(&b.to_string_lossy()));
    entries.dedup();

    entries
        .into_iter()
        .filter_map(|path| parse_generic_input_file(input, &path, existing_hashes))
        .collect()
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
        let parser = input.parser();
        let source = if input.source == "pi" && parser == "pi" {
            "session"
        } else {
            input.source.as_str()
        };
        Self::new(source, parser)
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
        claude::parse_discovered_paths(&input.discover_config(), existing_hashes)
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

        let discover = input.discover_config();
        let mut entries = Vec::new();
        for root in &discover.roots {
            let mut single_root = discover.clone();
            single_root.roots = vec![root.clone()];
            match discover_candidate_files(&single_root) {
                Ok(root_entries) => entries.extend(root_entries),
                Err(err) => {
                    tracing::warn!("Could not discover files for input root {}: {}", root, err)
                }
            }
        }
        entries.sort_by(|a, b| a.to_string_lossy().cmp(&b.to_string_lossy()));
        entries.dedup();

        if entries.is_empty() {
            tracing::warn!("No files found for input paths: {:?}", input.paths);
            return parsed;
        }

        for path in entries {
            if let Some(file) = parse_pi_file(&path, existing_hashes) {
                parsed.push(file);
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

pub fn parse_input_definitions(
    inputs: &[InputDefinition],
    existing_hashes: &HashMap<String, String>,
) -> Vec<ParsedFile> {
    let mut parsed = Vec::new();

    for input in inputs {
        parsed.extend(parse_generic_input_definition(input, existing_hashes));
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

    fn generic_input_definition(
        id: &str,
        root: &Path,
        format: crate::input_config::DecodeFormat,
        record_selector: &str,
        content_string: &str,
        content_blocks: Option<&str>,
        content_type: Option<&str>,
    ) -> crate::input_config::InputDefinition {
        crate::input_config::InputDefinition {
            id: id.into(),
            source: "session".into(),
            active: true,
            discover: crate::input_config::DiscoverConfig {
                roots: vec![root.to_string_lossy().into_owned()],
                include: vec!["**/*.{json,jsonl}".into()],
                exclude: Vec::new(),
                follow_symlinks: false,
            },
            decode: crate::input_config::DecodeConfig {
                format,
                encoding: "utf-8".into(),
            },
            record: crate::input_config::RecordConfig {
                selector: record_selector.into(),
                include_when: Vec::new(),
                exclude_when: Vec::new(),
            },
            mapping: crate::input_config::MapConfig {
                role: "$.role".into(),
                uuid: Some("$.uuid".into()),
                timestamp: Some("$.timestamp".into()),
                session_id: Some("$.session_id".into()),
                project: Some("$.project".into()),
                role_aliases: [("human".to_string(), "user".to_string())]
                    .into_iter()
                    .collect(),
            },
            content: crate::input_config::ContentConfig {
                selector: "$.content".into(),
                string: content_string.into(),
                blocks: content_blocks.map(str::to_string),
                block_text: Some("$.text".into()),
                content_type: content_type.map(str::to_string),
                include_when: Vec::new(),
                exclude_when: Vec::new(),
                default_content_type: "text".into(),
            },
            text: crate::input_config::TextConfig::default(),
        }
    }

    fn predicate(
        selector: &str,
        op: crate::input_config::PredicateOp,
        value: Option<crate::input_config::PredicateValue>,
    ) -> crate::input_config::Predicate {
        crate::input_config::Predicate {
            selector: selector.into(),
            op,
            value,
        }
    }

    fn string_value(value: &str) -> crate::input_config::PredicateValue {
        crate::input_config::PredicateValue::String(value.into())
    }

    fn bool_value(value: bool) -> crate::input_config::PredicateValue {
        crate::input_config::PredicateValue::Bool(value)
    }

    #[test]
    fn test_generic_jsonl_input_parses_string_content_role_aliases_and_defaults()
    -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let file_path = dir.path().join("generic.jsonl");
        fs::write(
            &file_path,
            r#"{"role":"human","uuid":"u1","timestamp":"2024-01-01T00:00:00Z","session_id":"s1","project":"project-a","content":" hello from jsonl "}
not-json
{"role":"assistant","uuid":"u2","timestamp":"2024-01-01T00:00:01Z","session_id":"s1","project":"project-a","content":"answer"}"#,
        )
        .into_diagnostic()?;

        let input = generic_input_definition(
            "generic-jsonl",
            dir.path(),
            crate::input_config::DecodeFormat::Jsonl,
            "$",
            "$",
            None,
            None,
        );

        let files = parse_input_definitions(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].source, "session");
        assert_eq!(files[0].project.as_deref(), Some("project-a"));
        assert_eq!(files[0].messages.len(), 2);
        assert_eq!(files[0].messages[0].role, "user");
        assert_eq!(files[0].messages[0].text, "hello from jsonl");
        assert_eq!(files[0].messages[0].uuid.as_deref(), Some("u1"));
        assert_eq!(
            files[0].messages[0].timestamp.as_deref(),
            Some("2024-01-01T00:00:00Z")
        );
        assert_eq!(files[0].messages[0].content_type, "text");
        assert_eq!(files[0].messages[1].role, "assistant");
        assert_eq!(files[0].messages[1].ordinal, 2);

        Ok(())
    }

    #[test]
    fn test_generic_jsonl_input_parses_object_and_array_content() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(
            dir.path().join("content-shapes.jsonl"),
            r#"{"role":"assistant","uuid":"u1","content":{"text":"object text"}}
{"role":"assistant","uuid":"u2","content":[{"type":"text","text":"array text"},{"type":"code","text":"fn main() {}"}]}"#,
        )
        .into_diagnostic()?;

        let input = generic_input_definition(
            "content-shapes",
            dir.path(),
            crate::input_config::DecodeFormat::Jsonl,
            "$",
            "$.text",
            Some("$.content[*]"),
            Some("$.type"),
        );

        let files = parse_input_definitions(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 2);
        assert_eq!(files[0].messages[0].text, "object text");
        assert_eq!(files[0].messages[0].content_type, "text");
        assert_eq!(files[0].messages[1].text, "array text\nfn main() {}");
        assert_eq!(files[0].messages[1].content_type, "code");

        Ok(())
    }

    #[test]
    fn test_generic_predicates_filter_records_with_all_mvp_operators() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(
            dir.path().join("predicate-records.jsonl"),
            r#"{"role":"user","type":"user","status":"active","archived":false,"required":"yes","drop":false,"content":"keep"}
{"role":"user","type":"summary","status":"active","archived":false,"required":"yes","content":"drop in"}
{"role":"user","type":"user","status":"inactive","archived":false,"required":"yes","content":"drop eq"}
{"role":"user","type":"user","status":"active","archived":true,"required":"yes","content":"drop ne"}
{"role":"user","type":"user","status":"active","archived":false,"content":"drop exists"}
{"role":"user","type":"user","status":"active","archived":false,"required":"yes","deleted":true,"content":"drop missing"}
{"role":"user","type":"user","status":"active","archived":false,"required":"yes","drop":true,"content":"drop exclude"}"#,
        )
        .into_diagnostic()?;

        let mut input = generic_input_definition(
            "predicate-records",
            dir.path(),
            crate::input_config::DecodeFormat::Jsonl,
            "$",
            "$",
            None,
            None,
        );
        input.record.include_when = vec![
            predicate(
                "$.type",
                crate::input_config::PredicateOp::In,
                Some(crate::input_config::PredicateValue::Array(vec![
                    string_value("user"),
                    string_value("assistant"),
                ])),
            ),
            predicate(
                "$.status",
                crate::input_config::PredicateOp::Eq,
                Some(string_value("active")),
            ),
            predicate(
                "$.archived",
                crate::input_config::PredicateOp::Ne,
                Some(bool_value(true)),
            ),
            predicate("$.required", crate::input_config::PredicateOp::Exists, None),
            predicate("$.deleted", crate::input_config::PredicateOp::Missing, None),
        ];
        input.record.exclude_when = vec![predicate(
            "$.drop",
            crate::input_config::PredicateOp::Eq,
            Some(bool_value(true)),
        )];

        let files = parse_input_definitions(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].text, "keep");
        Ok(())
    }

    #[test]
    fn test_generic_content_predicates_can_exclude_pi_think_blocks() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(
            dir.path().join("pi-blocks.jsonl"),
            r#"{"role":"assistant","content":{"blocks":[{"type":"text","text":"visible"},{"type":"think","text":"hidden reasoning"},{"type":"code","text":"let x = 1;"}]}}"#,
        )
        .into_diagnostic()?;

        let mut input = generic_input_definition(
            "pi-blocks",
            dir.path(),
            crate::input_config::DecodeFormat::Jsonl,
            "$",
            "$.text",
            Some("$.content.blocks[*]"),
            Some("$.type"),
        );
        input.content.exclude_when = vec![predicate(
            "$.type",
            crate::input_config::PredicateOp::Eq,
            Some(string_value("think")),
        )];

        let files = parse_input_definitions(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].text, "visible\nlet x = 1;");
        assert_eq!(files[0].messages[0].content_type, "code");
        Ok(())
    }

    #[test]
    fn test_generic_text_normalization_applies_each_remove_kind_trim_and_drop_empty()
    -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(
            dir.path().join("normalize.jsonl"),
            r#"{"role":"user","content":"PREFIX:  keep  <noise>drop</noise>  :SUFFIX"}
{"role":"user","content":"PREFIX:<noise>drop</noise>:SUFFIX"}"#,
        )
        .into_diagnostic()?;

        let mut input = generic_input_definition(
            "normalize",
            dir.path(),
            crate::input_config::DecodeFormat::Jsonl,
            "$",
            "$",
            None,
            None,
        );
        input.text.remove = vec![
            crate::input_config::RemoveRule {
                kind: crate::input_config::RemoveKind::Regex,
                pattern: r"<noise>[\s\S]*?</noise>".into(),
            },
            crate::input_config::RemoveRule {
                kind: crate::input_config::RemoveKind::Prefix,
                pattern: "PREFIX:".into(),
            },
            crate::input_config::RemoveRule {
                kind: crate::input_config::RemoveKind::Suffix,
                pattern: ":SUFFIX".into(),
            },
        ];

        let files = parse_input_definitions(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].text, "keep");
        Ok(())
    }

    #[test]
    fn test_claude_noise_can_be_expressed_with_manifest_predicates_and_text_remove()
    -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let data_dir = dir.path().join("data");
        fs::create_dir(&data_dir).into_diagnostic()?;
        fs::write(
            data_dir.join("claude.jsonl"),
            r#"{"type":"user","message":{"role":"user","content":"keep <system-reminder>drop</system-reminder> text"}}
{"type":"summary","message":{"role":"assistant","content":"drop by type"}}
{"type":"user","isMeta":true,"message":{"role":"user","content":"drop meta"}}
{"type":"assistant","message":{"role":"assistant","content":"<task-notification>drop all</task-notification>"}}"#,
        )
        .into_diagnostic()?;
        let data_root = data_dir.to_string_lossy().replace('\\', "\\\\");
        fs::write(
            dir.path().join("claude.inputs.toml"),
            format!(
                r#"version = 1

[[inputs]]
id = "claude-test"
source = "session"

[inputs.discover]
roots = ["{data_root}"]
include = ["**/*.jsonl"]

[inputs.decode]
format = "jsonl"

[inputs.record]
selector = "$"
include_when = [{{ selector = "$.type", op = "in", value = ["user", "assistant"] }}]
exclude_when = [{{ selector = "$.isMeta", op = "eq", value = true }}]

[inputs.map]
role = "$.message.role"

[inputs.content]
selector = "$.message.content"
string = "$"

[inputs.text]
trim = true
drop_empty = true
remove = [
  {{ kind = "regex", pattern = "<system-reminder>[\\s\\S]*?</system-reminder>" }},
  {{ kind = "regex", pattern = "<task-notification>[\\s\\S]*?</task-notification>" }},
]
"#
            ),
        )
        .into_diagnostic()?;

        let input_config = crate::input_config::InputConfig::load_from_dir(dir.path())?;
        let files = parse_input_definitions(&input_config.active_inputs(), &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].text, "keep  text");
        Ok(())
    }

    #[test]
    fn test_generic_record_selector_no_match_yields_empty_file() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(dir.path().join("records.json"), r#"{"records":[]}"#).into_diagnostic()?;

        let input = generic_input_definition(
            "generic-json-no-match",
            dir.path(),
            crate::input_config::DecodeFormat::Json,
            "$.missing[*]",
            "$",
            None,
            None,
        );

        let files = parse_input_definitions(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert!(files[0].messages.is_empty());

        Ok(())
    }

    #[test]
    fn test_generic_json_input_uses_record_selector() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(
            dir.path().join("records.json"),
            r#"{"records":[{"role":"user","uuid":"u1","content":"from json"},{"role":"assistant","uuid":"u2","content":"json answer"}]}"#,
        )
        .into_diagnostic()?;

        let input = generic_input_definition(
            "generic-json",
            dir.path(),
            crate::input_config::DecodeFormat::Json,
            "$.records[*]",
            "$",
            None,
            None,
        );

        let files = parse_input_definitions(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 2);
        assert_eq!(files[0].messages[0].text, "from json");
        assert_eq!(files[0].messages[1].text, "json answer");

        Ok(())
    }

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
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
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
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
            active: true,
        };
        let wrong_source_input = SessionInput {
            source: "unknown".into(),
            parser: "claude".into(),
            paths: vec![dir.path().to_str().unwrap().into()],
            glob: None,
            include_agents: false,
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
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
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
            active: true,
        };

        let files = parse_session_inputs(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].text, "still parsed");

        Ok(())
    }

    #[test]
    fn test_session_input_claude_compatibility_preserves_legacy_messages() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let file_path = dir.path().join("session.jsonl");
        fs::write(
            dir.path().join("sessions-index.json"),
            r#"{"session-compat":{"projectPath":"/workspace/compat-project"}}"#,
        )
        .into_diagnostic()?;

        fs::write(
            &file_path,
            r#"{"uuid":"session-compat","timestamp":"2024-01-01T00:00:00Z","type":"user","message":{"role":"user","content":"keep one<system-reminder>drop this</system-reminder> keep two"}}
{"uuid":"ignored-progress","type":"progress"}
{"uuid":"ignored-meta","timestamp":"2024-01-01T00:00:01Z","type":"user","isMeta":true,"message":{"role":"user","content":"meta should not index"}}
{"uuid":"ignored-noise","timestamp":"2024-01-01T00:00:02Z","type":"user","message":{"role":"user","content":"<task-notification>drop entire message</task-notification>"}}
{"uuid":"assistant-compatible","timestamp":"2024-01-01T00:00:03Z","type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"answer text"},{"type":"tool_use","text":"hidden tool"},{"type":"code","text":"fn main() {}"}]}}"#,
        )
        .into_diagnostic()?;

        let hashes = HashMap::new();
        let legacy = parse_sessions(dir.path().to_str().unwrap(), &hashes, false)?;
        let declarative = parse_session_inputs(
            &[SessionInput {
                source: "session".into(),
                parser: "claude".into(),
                paths: vec![dir.path().to_string_lossy().into_owned()],
                glob: None,
                include_agents: false,
                include: crate::input_config::default_discover_include(),
                exclude: crate::input_config::default_discover_exclude(),
                follow_symlinks: false,
                active: true,
            }],
            &hashes,
        );

        assert_eq!(declarative.len(), legacy.len());
        assert_eq!(declarative[0].source, legacy[0].source);
        assert_eq!(declarative[0].source, "session");
        assert_eq!(declarative[0].source_path, legacy[0].source_path);
        assert_eq!(declarative[0].hash, legacy[0].hash);
        assert_eq!(declarative[0].project, legacy[0].project);
        assert_eq!(declarative[0].project.as_deref(), Some("compat-project"));
        assert_eq!(declarative[0].messages.len(), legacy[0].messages.len());
        assert_eq!(declarative[0].messages.len(), 2);

        for (declarative_msg, legacy_msg) in declarative[0].messages.iter().zip(&legacy[0].messages)
        {
            assert_eq!(declarative_msg.role, legacy_msg.role);
            assert_eq!(declarative_msg.text, legacy_msg.text);
            assert_eq!(declarative_msg.ordinal, legacy_msg.ordinal);
            assert_eq!(declarative_msg.uuid, legacy_msg.uuid);
            assert_eq!(declarative_msg.timestamp, legacy_msg.timestamp);
            assert_eq!(declarative_msg.content_type, legacy_msg.content_type);
        }

        assert_eq!(declarative[0].messages[0].text, "keep one keep two");
        assert_eq!(declarative[0].messages[0].ordinal, 0);
        assert_eq!(
            declarative[0].messages[0].uuid.as_deref(),
            Some("session-compat")
        );
        assert_eq!(declarative[0].messages[0].content_type, "text");
        assert_eq!(declarative[0].messages[1].text, "answer text fn main() {}");
        assert_eq!(declarative[0].messages[1].ordinal, 4);
        assert_eq!(
            declarative[0].messages[1].uuid.as_deref(),
            Some("assistant-compatible")
        );
        assert_eq!(declarative[0].messages[1].content_type, "code");

        Ok(())
    }

    #[test]
    fn test_session_input_claude_glob_filters_candidate_files() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(
            dir.path().join("keep-session.jsonl"),
            r#"{"type":"user","message":{"role":"user","content":"keep me"}}"#,
        )
        .into_diagnostic()?;
        fs::write(
            dir.path().join("skip-session.jsonl"),
            r#"{"type":"user","message":{"role":"user","content":"skip me"}}"#,
        )
        .into_diagnostic()?;

        let input = SessionInput {
            source: "session".into(),
            parser: "claude".into(),
            paths: vec![dir.path().to_string_lossy().into_owned()],
            glob: Some("keep-*.jsonl".into()),
            include_agents: false,
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
            active: true,
        };

        let files = parse_session_inputs(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages[0].text, "keep me");
        assert!(files[0].source_path.ends_with("keep-session.jsonl"));

        Ok(())
    }

    #[test]
    fn test_session_input_claude_glob_supports_extension_alternatives() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(
            dir.path().join("keep-json.json"),
            r#"{"type":"user","message":{"role":"user","content":"json kept"}}"#,
        )
        .into_diagnostic()?;
        fs::write(
            dir.path().join("keep-jsonl.jsonl"),
            r#"{"type":"user","message":{"role":"user","content":"jsonl kept"}}"#,
        )
        .into_diagnostic()?;
        fs::write(
            dir.path().join("skip.txt"),
            r#"{"type":"user","message":{"role":"user","content":"txt skipped"}}"#,
        )
        .into_diagnostic()?;

        let input = SessionInput {
            source: "session".into(),
            parser: "claude".into(),
            paths: vec![dir.path().to_string_lossy().into_owned()],
            glob: Some("*.{json,jsonl}".into()),
            include_agents: false,
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
            active: true,
        };

        let mut texts: Vec<_> = parse_session_inputs(&[input], &HashMap::new())
            .into_iter()
            .flat_map(|file| file.messages.into_iter().map(|message| message.text))
            .collect();
        texts.sort_unstable();

        assert_eq!(texts, vec!["json kept", "jsonl kept"]);

        Ok(())
    }

    #[test]
    fn test_discovery_uses_include_exclude_globs_for_files_and_dirs() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let root = dir.path().join("root");
        let subagents = root.join("project/subagents");
        fs::create_dir_all(&subagents).into_diagnostic()?;
        fs::write(root.join("b.jsonl"), "{}").into_diagnostic()?;
        fs::write(root.join("a.jsonl"), "{}").into_diagnostic()?;
        fs::write(root.join("skip.txt"), "{}").into_diagnostic()?;
        fs::write(subagents.join("agent.jsonl"), "{}").into_diagnostic()?;
        let direct = dir.path().join("direct.jsonl");
        fs::write(&direct, "{}").into_diagnostic()?;

        let discovered = discover_candidate_files(&crate::input_config::DiscoverConfig {
            roots: vec![
                root.to_string_lossy().into_owned(),
                direct.to_string_lossy().into_owned(),
            ],
            include: vec!["**/*.jsonl".into()],
            exclude: vec!["**/subagents/**".into()],
            follow_symlinks: false,
        })?;

        assert_eq!(
            discovered,
            vec![direct, root.join("a.jsonl"), root.join("b.jsonl")]
        );
        Ok(())
    }

    #[test]
    fn test_discovery_include_glob_is_not_hardcoded_to_jsonl() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let root = dir.path();
        let data_file = root.join("session.data");
        fs::write(&data_file, "{}").into_diagnostic()?;
        fs::write(root.join("session.jsonl"), "{}").into_diagnostic()?;

        let discovered = discover_candidate_files(&crate::input_config::DiscoverConfig {
            roots: vec![root.to_string_lossy().into_owned()],
            include: vec!["**/*.data".into()],
            exclude: Vec::new(),
            follow_symlinks: false,
        })?;

        assert_eq!(discovered, vec![data_file]);
        Ok(())
    }

    #[cfg(unix)]
    #[test]
    fn test_discovery_does_not_follow_symlinks_by_default() -> miette::Result<()> {
        use std::os::unix::fs::symlink;

        let dir = tempdir().into_diagnostic()?;
        let target = dir.path().join("target");
        let root = dir.path().join("root");
        fs::create_dir_all(&target).into_diagnostic()?;
        fs::create_dir_all(&root).into_diagnostic()?;
        fs::write(target.join("linked.jsonl"), "{}").into_diagnostic()?;
        symlink(&target, root.join("link")).into_diagnostic()?;

        let mut discover = crate::input_config::DiscoverConfig {
            roots: vec![root.to_string_lossy().into_owned()],
            include: vec!["**/*.jsonl".into()],
            exclude: Vec::new(),
            follow_symlinks: false,
        };

        assert!(discover_candidate_files(&discover)?.is_empty());
        discover.follow_symlinks = true;
        assert_eq!(
            discover_candidate_files(&discover)?,
            vec![root.join("link/linked.jsonl")]
        );
        Ok(())
    }

    #[test]
    fn test_session_input_inactive_inputs_are_skipped() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(
            dir.path().join("inactive.jsonl"),
            r#"{"type":"user","message":{"role":"user","content":"do not parse"}}"#,
        )
        .into_diagnostic()?;

        let input = SessionInput {
            source: "session".into(),
            parser: "claude".into(),
            paths: vec![dir.path().to_string_lossy().into_owned()],
            glob: None,
            include_agents: false,
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
            active: false,
        };

        assert!(parse_session_inputs(&[input], &HashMap::new()).is_empty());

        Ok(())
    }

    #[test]
    fn test_session_input_include_agents_controls_subagent_paths() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let subagents_dir = dir.path().join("subagents");
        fs::create_dir(&subagents_dir).into_diagnostic()?;
        fs::write(
            dir.path().join("main.jsonl"),
            r#"{"type":"user","message":{"role":"user","content":"main session"}}"#,
        )
        .into_diagnostic()?;
        fs::write(
            subagents_dir.join("agent.jsonl"),
            r#"{"type":"user","message":{"role":"user","content":"agent session"}}"#,
        )
        .into_diagnostic()?;

        let without_agents = SessionInput {
            source: "session".into(),
            parser: "claude".into(),
            paths: vec![dir.path().to_string_lossy().into_owned()],
            glob: None,
            include_agents: false,
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
            active: true,
        };
        let with_agents = SessionInput {
            include_agents: true,
            exclude: Vec::new(),
            ..without_agents.clone()
        };

        let without = parse_session_inputs(&[without_agents], &HashMap::new());
        let with = parse_session_inputs(&[with_agents], &HashMap::new());
        let mut with_texts: Vec<_> = with
            .iter()
            .flat_map(|file| file.messages.iter().map(|message| message.text.as_str()))
            .collect();
        with_texts.sort_unstable();

        assert_eq!(without.len(), 1);
        assert_eq!(without[0].messages[0].text, "main session");
        assert_eq!(with.len(), 2);
        assert_eq!(with_texts, vec!["agent session", "main session"]);

        Ok(())
    }

    #[test]
    fn test_session_input_missing_path_does_not_abort_batch() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        fs::write(
            dir.path().join("valid.jsonl"),
            r#"{"type":"user","message":{"role":"user","content":"still parsed after missing path"}}"#,
        )
        .into_diagnostic()?;

        let input = SessionInput {
            source: "session".into(),
            parser: "claude".into(),
            paths: vec![
                dir.path().join("missing").to_string_lossy().into_owned(),
                dir.path().to_string_lossy().into_owned(),
            ],
            glob: None,
            include_agents: false,
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
            active: true,
        };

        let files = parse_session_inputs(&[input], &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].messages[0].text, "still parsed after missing path");

        Ok(())
    }

    #[test]
    fn test_session_input_deduplication_uses_source_path_and_hash() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let file_path = dir.path().join("session.jsonl");

        fs::write(
            &file_path,
            r#"{"uuid":"dedupe-source-path","timestamp":"2024-01-01T00:00:00Z","type":"user","message":{"role":"user","content":"dedupe stays path keyed"}}"#,
        )
        .into_diagnostic()?;

        let input = SessionInput {
            source: "session".into(),
            parser: "claude".into(),
            paths: vec![dir.path().to_string_lossy().into_owned()],
            glob: None,
            include_agents: false,
            include: crate::input_config::default_discover_include(),
            exclude: crate::input_config::default_discover_exclude(),
            follow_symlinks: false,
            active: true,
        };

        let parsed = parse_session_inputs(std::slice::from_ref(&input), &HashMap::new());
        assert_eq!(parsed.len(), 1);
        assert_eq!(parsed[0].source, "session");

        let mut existing_hashes = HashMap::new();
        existing_hashes.insert(
            "/different/source_path.jsonl".to_string(),
            parsed[0].hash.clone(),
        );
        let same_hash_different_path =
            parse_session_inputs(std::slice::from_ref(&input), &existing_hashes);
        assert_eq!(same_hash_different_path.len(), 1);

        existing_hashes.insert(parsed[0].source_path.clone(), parsed[0].hash.clone());
        let same_hash_same_path = parse_session_inputs(&[input], &existing_hashes);
        assert!(same_hash_same_path.is_empty());

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
