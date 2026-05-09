use crate::core::plans::split_by_headers;
use crate::core::{ParsedFile, ParsedMessage};
use crate::input_config::{
    DecodeFormat, DiscoverConfig, InputDefinition, Predicate, PredicateOp, PredicateValue,
    RemoveKind,
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
use walkdir::WalkDir;

pub fn compute_hash(path: impl AsRef<Path>) -> miette::Result<String> {
    let data = fs::read(path).into_diagnostic()?;
    let mut hasher = Sha256::new();
    hasher.update(data);
    Ok(hex::encode(hasher.finalize()))
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
            tracing::warn!(
                "Skipping missing discovery root in discover.roots: {}",
                root.display()
            );
            continue;
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
    let Some(content) = &input.content else {
        return false;
    };
    if !predicates_match_all(
        &input.id,
        &content.include_when,
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
        &content.exclude_when,
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
    let content = input.content.as_ref()?;
    let content_selector = parse_jsonpath(&content.selector, &input.id, "content.selector")?;
    let content_nodes = query_nodes(&content_selector, record);
    if content_nodes.is_empty() {
        return None;
    }

    let mut parts = Vec::new();
    let mut content_types = Vec::new();

    if let Some(blocks_selector) = &content.blocks {
        if let Some(blocks_path) = parse_jsonpath(blocks_selector, &input.id, "content.blocks") {
            let block_nodes = query_nodes(&blocks_path, record);
            if !block_nodes.is_empty() {
                let block_text_path = content
                    .block_text
                    .as_ref()
                    .and_then(|selector| parse_jsonpath(selector, &input.id, "content.block_text"));
                let content_type_path = content.content_type.as_ref().and_then(|selector| {
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
                        aggregate_content_type(&content_types, &content.default_content_type),
                    ));
                }
            }
        }
    }

    let string_path = parse_jsonpath(&content.string, &input.id, "content.string")?;
    let content_type_path = content
        .content_type
        .as_ref()
        .and_then(|selector| parse_jsonpath(selector, &input.id, "content.content_type"));

    for content_node in content_nodes {
        parts.extend(selected_strings(&string_path, content_node));
        if let Some(path) = &content_type_path {
            content_types.extend(selected_strings(path, content_node));
        }
    }

    if content_types.is_empty() {
        if let Some(path) = &content_type_path {
            content_types.extend(selected_strings(path, record));
        }
    }

    Some((
        parts.join(&input.text.join),
        aggregate_content_type(&content_types, &content.default_content_type),
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
    let Some(mapping) = &input.mapping else {
        tracing::debug!(
            input_id = %input.id,
            ordinal,
            "Dropping message because map configuration is missing"
        );
        return None;
    };
    let Some(raw_role) = select_first_string(&mapping.role, record, &input.id, "map.role") else {
        tracing::debug!(
            input_id = %input.id,
            ordinal,
            "Dropping message because map.role did not yield a scalar value"
        );
        return None;
    };
    let role = mapping
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
        uuid: select_optional_string(mapping.uuid.as_ref(), record, &input.id, "map.uuid"),
        timestamp: select_optional_string(
            mapping.timestamp.as_ref(),
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
                project = input.mapping.as_ref().and_then(|mapping| {
                    select_optional_string(
                        mapping.project.as_ref(),
                        record,
                        &input.id,
                        "map.project",
                    )
                });
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
            project = input.mapping.as_ref().and_then(|mapping| {
                select_optional_string(mapping.project.as_ref(), record, &input.id, "map.project")
            });
        }
        match parsed_message_from_record(input, record, ordinal) {
            Some(message) => messages.push(message),
            None => data_errors += 1,
        }
    }

    (messages, project, data_errors)
}

#[derive(Debug, serde::Serialize)]
pub struct InputDryRunDrop {
    pub scope: String,
    pub ordinal: Option<usize>,
    pub block_index: Option<usize>,
    pub reason: String,
}

#[derive(Debug, serde::Serialize)]
pub struct InputDryRunReport {
    pub input_id: String,
    pub source: String,
    pub file: String,
    pub records_read: usize,
    pub records_emitted: usize,
    pub records_dropped: usize,
    pub blocks_read: usize,
    pub blocks_dropped: usize,
    pub drop_reasons: Vec<InputDryRunDrop>,
    pub messages: Vec<ParsedMessage>,
}

impl InputDryRunReport {
    fn new(input: &InputDefinition, path: &Path) -> Self {
        Self {
            input_id: input.id.clone(),
            source: input.source.clone(),
            file: path.to_string_lossy().into_owned(),
            records_read: 0,
            records_emitted: 0,
            records_dropped: 0,
            blocks_read: 0,
            blocks_dropped: 0,
            drop_reasons: Vec::new(),
            messages: Vec::new(),
        }
    }

    fn record_drop(&mut self, ordinal: usize, reason: impl Into<String>) {
        self.records_dropped += 1;
        self.drop_reasons.push(InputDryRunDrop {
            scope: "record".to_string(),
            ordinal: Some(ordinal),
            block_index: None,
            reason: reason.into(),
        });
    }

    fn block_drop(&mut self, ordinal: usize, block_index: usize, reason: impl Into<String>) {
        self.blocks_dropped += 1;
        self.drop_reasons.push(InputDryRunDrop {
            scope: "block".to_string(),
            ordinal: Some(ordinal),
            block_index: Some(block_index),
            reason: reason.into(),
        });
    }

    fn file_drop(&mut self, reason: impl Into<String>) {
        self.drop_reasons.push(InputDryRunDrop {
            scope: "file".to_string(),
            ordinal: None,
            block_index: None,
            reason: reason.into(),
        });
    }
}

fn record_passes_predicates_dry_run(
    input: &InputDefinition,
    record: &Value,
    ordinal: usize,
    report: &mut InputDryRunReport,
) -> bool {
    if !predicates_match_all(
        &input.id,
        &input.record.include_when,
        record,
        "record.include_when",
    ) {
        report.record_drop(ordinal, "record.include_when did not match");
        return false;
    }
    if predicates_match_any(
        &input.id,
        &input.record.exclude_when,
        record,
        "record.exclude_when",
    ) {
        report.record_drop(ordinal, "record.exclude_when matched");
        return false;
    }
    true
}

fn content_block_passes_predicates_dry_run(
    input: &InputDefinition,
    block: &Value,
    ordinal: usize,
    block_index: usize,
    report: &mut InputDryRunReport,
) -> bool {
    let Some(content) = &input.content else {
        report.block_drop(ordinal, block_index, "content configuration is missing");
        return false;
    };
    if !predicates_match_all(
        &input.id,
        &content.include_when,
        block,
        "content.include_when",
    ) {
        report.block_drop(ordinal, block_index, "content.include_when did not match");
        return false;
    }
    if predicates_match_any(
        &input.id,
        &content.exclude_when,
        block,
        "content.exclude_when",
    ) {
        report.block_drop(ordinal, block_index, "content.exclude_when matched");
        return false;
    }
    true
}

fn extract_content_from_record_dry_run(
    input: &InputDefinition,
    record: &Value,
    ordinal: usize,
    report: &mut InputDryRunReport,
) -> Option<(String, String)> {
    let content = input.content.as_ref()?;
    let content_selector = parse_jsonpath(&content.selector, &input.id, "content.selector")?;
    let content_nodes = query_nodes(&content_selector, record);
    if content_nodes.is_empty() {
        return None;
    }

    let mut parts = Vec::new();
    let mut content_types = Vec::new();

    if let Some(blocks_selector) = &content.blocks {
        if let Some(blocks_path) = parse_jsonpath(blocks_selector, &input.id, "content.blocks") {
            let block_nodes = query_nodes(&blocks_path, record);
            if !block_nodes.is_empty() {
                let block_text_path = content
                    .block_text
                    .as_ref()
                    .and_then(|selector| parse_jsonpath(selector, &input.id, "content.block_text"));
                let content_type_path = content.content_type.as_ref().and_then(|selector| {
                    parse_jsonpath(selector, &input.id, "content.content_type")
                });

                for (block_index, block) in block_nodes.into_iter().enumerate() {
                    report.blocks_read += 1;
                    if !content_block_passes_predicates_dry_run(
                        input,
                        block,
                        ordinal,
                        block_index,
                        report,
                    ) {
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
                        aggregate_content_type(&content_types, &content.default_content_type),
                    ));
                }
            }
        }
    }

    let string_path = parse_jsonpath(&content.string, &input.id, "content.string")?;
    let content_type_path = content
        .content_type
        .as_ref()
        .and_then(|selector| parse_jsonpath(selector, &input.id, "content.content_type"));

    for content_node in content_nodes {
        parts.extend(selected_strings(&string_path, content_node));
        if let Some(path) = &content_type_path {
            content_types.extend(selected_strings(path, content_node));
        }
    }

    if content_types.is_empty() {
        if let Some(path) = &content_type_path {
            content_types.extend(selected_strings(path, record));
        }
    }

    Some((
        parts.join(&input.text.join),
        aggregate_content_type(&content_types, &content.default_content_type),
    ))
}

fn parsed_message_from_record_dry_run(
    input: &InputDefinition,
    record: &Value,
    ordinal: usize,
    report: &mut InputDryRunReport,
) -> Option<ParsedMessage> {
    let Some(mapping) = &input.mapping else {
        report.record_drop(ordinal, "map configuration is missing");
        return None;
    };
    let Some(raw_role) = select_first_string(&mapping.role, record, &input.id, "map.role") else {
        report.record_drop(ordinal, "map.role did not yield a scalar value");
        return None;
    };
    let role = mapping
        .role_aliases
        .get(&raw_role)
        .cloned()
        .unwrap_or(raw_role);
    let Some((raw_text, content_type)) =
        extract_content_from_record_dry_run(input, record, ordinal, report)
    else {
        report.record_drop(ordinal, "content selectors did not yield text");
        return None;
    };
    let Some(text) = normalize_extracted_text(input, &raw_text) else {
        report.record_drop(ordinal, "text normalization produced an empty message");
        return None;
    };

    Some(ParsedMessage {
        role,
        text,
        ordinal,
        uuid: select_optional_string(mapping.uuid.as_ref(), record, &input.id, "map.uuid"),
        timestamp: select_optional_string(
            mapping.timestamp.as_ref(),
            record,
            &input.id,
            "map.timestamp",
        ),
        content_type,
    })
}

fn parse_markdown_content(input: &InputDefinition, content: &str) -> Vec<ParsedMessage> {
    if content.trim().is_empty() {
        return Vec::new();
    }

    let content_type = input.content.as_ref().map_or_else(
        || "text".to_string(),
        |content| content.default_content_type.clone(),
    );

    match input.decode.format {
        DecodeFormat::Markdown => {
            normalize_extracted_text(input, content).map_or_else(Vec::new, |text| {
                vec![ParsedMessage {
                    role: input.source.clone(),
                    text,
                    ordinal: 0,
                    uuid: None,
                    timestamp: None,
                    content_type,
                }]
            })
        }
        DecodeFormat::MarkdownSections => split_by_headers(content)
            .into_iter()
            .filter_map(|mut message| {
                let text = normalize_extracted_text(input, &message.text)?;
                message.role = input.source.clone();
                message.text = text;
                message.content_type = content_type.clone();
                Some(message)
            })
            .collect(),
        DecodeFormat::Jsonl | DecodeFormat::Json => Vec::new(),
    }
}

fn dry_run_markdown_content(
    input: &InputDefinition,
    content: &str,
    report: &mut InputDryRunReport,
) {
    report.records_read = if content.trim().is_empty() { 0 } else { 1 };
    report.messages = parse_markdown_content(input, content);
}

fn dry_run_jsonl_content(input: &InputDefinition, content: &str, report: &mut InputDryRunReport) {
    for (line_number, line) in content.lines().enumerate() {
        if line.trim().is_empty() {
            continue;
        }
        let value = match serde_json::from_str::<Value>(line) {
            Ok(value) => value,
            Err(err) => {
                report.file_drop(format!("invalid JSONL line {}: {}", line_number + 1, err));
                continue;
            }
        };
        let Some(records) = parse_records_from_value(input, &value, "record.selector") else {
            report.file_drop("record.selector is invalid");
            continue;
        };
        for record in records {
            report.records_read += 1;
            if !record_passes_predicates_dry_run(input, record, line_number, report) {
                continue;
            }
            if let Some(message) =
                parsed_message_from_record_dry_run(input, record, line_number, report)
            {
                report.messages.push(message);
            }
        }
    }
}

fn dry_run_json_content(input: &InputDefinition, content: &str, report: &mut InputDryRunReport) {
    let value = match serde_json::from_str::<Value>(content) {
        Ok(value) => value,
        Err(err) => {
            report.file_drop(format!("invalid JSON file: {err}"));
            return;
        }
    };
    let Some(records) = parse_records_from_value(input, &value, "record.selector") else {
        report.file_drop("record.selector is invalid");
        return;
    };
    for (ordinal, record) in records.into_iter().enumerate() {
        report.records_read += 1;
        if !record_passes_predicates_dry_run(input, record, ordinal, report) {
            continue;
        }
        if let Some(message) = parsed_message_from_record_dry_run(input, record, ordinal, report) {
            report.messages.push(message);
        }
    }
}

pub fn dry_run_input_definition(
    input: &InputDefinition,
    path: &Path,
) -> miette::Result<InputDryRunReport> {
    if !input.active {
        return Err(miette::miette!(
            "Input '{}' is inactive; enable it before running inputs test",
            input.id
        ));
    }
    let content = fs::read_to_string(path).map_err(|err| {
        miette::miette!(
            "Failed to read dry-run sample file {}: {}",
            path.display(),
            err
        )
    })?;
    let mut report = InputDryRunReport::new(input, path);
    match input.decode.format {
        DecodeFormat::Jsonl => dry_run_jsonl_content(input, &content, &mut report),
        DecodeFormat::Json => dry_run_json_content(input, &content, &mut report),
        DecodeFormat::Markdown | DecodeFormat::MarkdownSections => {
            dry_run_markdown_content(input, &content, &mut report);
        }
    }
    report.records_emitted = report.messages.len();
    Ok(report)
}

pub(crate) fn parse_input_file_with_definition(
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
        DecodeFormat::Markdown | DecodeFormat::MarkdownSections => {
            (parse_markdown_content(input, &content), None, 0)
        }
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
        project: match input.decode.format {
            DecodeFormat::Markdown | DecodeFormat::MarkdownSections => project,
            DecodeFormat::Jsonl | DecodeFormat::Json => {
                project.or_else(|| Some("unknown".to_string()))
            }
        },
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
        .filter_map(|path| parse_input_file_with_definition(input, &path, existing_hashes))
        .collect()
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
            mapping: Some(crate::input_config::MapConfig {
                role: "$.role".into(),
                uuid: Some("$.uuid".into()),
                timestamp: Some("$.timestamp".into()),
                session_id: Some("$.session_id".into()),
                project: Some("$.project".into()),
                role_aliases: [("human".to_string(), "user".to_string())]
                    .into_iter()
                    .collect(),
            }),
            content: Some(crate::input_config::ContentConfig {
                selector: "$.content".into(),
                string: content_string.into(),
                blocks: content_blocks.map(str::to_string),
                block_text: Some("$.text".into()),
                content_type: content_type.map(str::to_string),
                include_when: Vec::new(),
                exclude_when: Vec::new(),
                default_content_type: "text".into(),
            }),
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
        input.content.as_mut().unwrap().exclude_when = vec![predicate(
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
    fn test_pi_input_preset_indexes_fixture_through_generic_engine() -> miette::Result<()> {
        let fixtures_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures");
        let input_config =
            crate::input_config::InputConfig::load_from_inputs_dir_for_tests(&fixtures_dir)?;
        let inputs: Vec<_> = input_config
            .active_inputs()
            .into_iter()
            .filter(|input| input.id == "pi")
            .collect();

        assert_eq!(
            inputs.len(),
            1,
            "expected tests/fixtures/pi.inputs.toml to declare one active pi input"
        );

        let files = parse_input_definitions(&inputs, &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].source, "session");
        assert_eq!(files[0].messages.len(), 2);
        assert_eq!(files[0].messages[0].role, "user");
        assert_eq!(files[0].messages[0].text, "pi manifest fixture signal");
        assert_eq!(files[0].messages[0].uuid.as_deref(), Some("pi-fixture-1"));
        assert_eq!(
            files[0].messages[0].timestamp.as_deref(),
            Some("2024-02-03T04:05:06Z")
        );
        assert_eq!(files[0].messages[1].role, "assistant");
        assert_eq!(files[0].messages[1].text, "pi visible answer");
        assert!(!files[0].messages[1].text.contains("hidden reasoning"));
        assert_eq!(files[0].messages[1].content_type, "text");
        assert_eq!(files[0].messages[1].uuid.as_deref(), Some("pi-fixture-2"));
        assert_eq!(
            files[0].messages[1].timestamp.as_deref(),
            Some("2024-02-03T04:05:07Z")
        );
        Ok(())
    }

    fn write_shipped_manifest_with_root(
        dir: &Path,
        filename: &str,
        manifest: &str,
        shipped_root: &str,
        test_root: &Path,
    ) -> miette::Result<()> {
        let test_root = test_root.to_string_lossy().replace('\\', "\\\\");
        let manifest = manifest.replace(shipped_root, &test_root);
        fs::write(dir.join(filename), manifest).into_diagnostic()
    }

    #[test]
    fn test_shipped_claude_and_pi_presets_emit_session_source() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let fixtures_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures");
        let claude_root = fixtures_dir.join("claude-preset/projects");
        let pi_root = dir.path().join("pi-sessions");
        fs::create_dir(&pi_root).into_diagnostic()?;
        fs::copy(
            fixtures_dir.join("pi-session.jsonl"),
            pi_root.join("pi-session.jsonl"),
        )
        .into_diagnostic()?;

        write_shipped_manifest_with_root(
            dir.path(),
            "claude.inputs.toml",
            include_str!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/inputs/claude.inputs.toml"
            )),
            "~/.claude/projects",
            &claude_root,
        )?;
        write_shipped_manifest_with_root(
            dir.path(),
            "pi.inputs.toml",
            include_str!(concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/inputs/pi.inputs.toml"
            )),
            "~/.pi/agent/sessions",
            &pi_root,
        )?;

        let input_config =
            crate::input_config::InputConfig::load_from_inputs_dir_for_tests(dir.path())?;

        for input_id in ["claude", "pi"] {
            let inputs: Vec<_> = input_config
                .active_inputs()
                .into_iter()
                .filter(|input| input.id == input_id)
                .collect();
            assert_eq!(inputs.len(), 1, "expected shipped {input_id} preset");
            assert_eq!(inputs[0].source, "session");

            let files = parse_input_definitions(&inputs, &HashMap::new());
            assert_eq!(files.len(), 1, "expected {input_id} fixture file");
            assert_eq!(files[0].source, "session");
            assert!(
                !files[0].messages.is_empty(),
                "expected {input_id} preset to emit messages"
            );
        }

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

        let input_config =
            crate::input_config::InputConfig::load_from_inputs_dir_for_tests(dir.path())?;
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
    fn test_generic_markdown_input_indexes_whole_document() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let docs = dir.path().join("docs");
        fs::create_dir(&docs).into_diagnostic()?;
        fs::write(
            docs.join("ke.md"),
            "# Knowledge Entry\n\nDeclarative markdown source signal.",
        )
        .into_diagnostic()?;
        fs::write(
            dir.path().join("docs.inputs.toml"),
            format!(
                r#"version = 1

[[inputs]]
id = "ke-docs"
source = "ke"

[inputs.discover]
roots = ["{}"]
include = ["**/*.md"]

[inputs.decode]
format = "markdown"
"#,
                docs.display()
            ),
        )
        .into_diagnostic()?;

        let input_config =
            crate::input_config::InputConfig::load_from_inputs_dir_for_tests(dir.path())?;
        let files = parse_input_definitions(&input_config.active_inputs(), &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].source, "ke");
        assert_eq!(files[0].project, None);
        assert_eq!(files[0].messages.len(), 1);
        assert_eq!(files[0].messages[0].role, "ke");
        assert!(files[0].messages[0].text.contains("markdown source signal"));
        Ok(())
    }

    #[test]
    fn test_generic_markdown_sections_input_splits_headers() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let docs = dir.path().join("plans");
        fs::create_dir(&docs).into_diagnostic()?;
        fs::write(
            docs.join("plan.md"),
            "# Plan\n\nIntro\n\n## First\n\nFirst body\n\n## Second\n\nSecond body",
        )
        .into_diagnostic()?;
        fs::write(
            dir.path().join("plans.inputs.toml"),
            format!(
                r#"version = 1

[[inputs]]
id = "plans"
source = "plan"

[inputs.discover]
roots = ["{}"]
include = ["**/*.md"]

[inputs.decode]
format = "markdown_sections"
"#,
                docs.display()
            ),
        )
        .into_diagnostic()?;

        let input_config =
            crate::input_config::InputConfig::load_from_inputs_dir_for_tests(dir.path())?;
        let files = parse_input_definitions(&input_config.active_inputs(), &HashMap::new());

        assert_eq!(files.len(), 1);
        assert_eq!(files[0].source, "plan");
        assert_eq!(files[0].messages.len(), 3);
        assert!(files[0].messages[0].text.contains("Intro"));
        assert!(files[0].messages[1].text.starts_with("## First"));
        assert!(files[0].messages[2].text.starts_with("## Second"));
        assert!(
            files[0]
                .messages
                .iter()
                .all(|message| message.role == "plan")
        );
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
    fn test_discovery_skips_missing_roots_and_keeps_existing_roots() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;
        let root = dir.path().join("root");
        fs::create_dir_all(&root).into_diagnostic()?;
        fs::write(root.join("session.jsonl"), "{}").into_diagnostic()?;

        let discovered = discover_candidate_files(&crate::input_config::DiscoverConfig {
            roots: vec![
                dir.path().join("missing").to_string_lossy().into_owned(),
                root.to_string_lossy().into_owned(),
            ],
            include: vec!["**/*.jsonl".into()],
            exclude: Vec::new(),
            follow_symlinks: false,
        })?;

        assert_eq!(discovered, vec![root.join("session.jsonl")]);
        Ok(())
    }

    #[test]
    fn test_discovery_skips_missing_only_roots() -> miette::Result<()> {
        let dir = tempdir().into_diagnostic()?;

        let discovered = discover_candidate_files(&crate::input_config::DiscoverConfig {
            roots: vec![dir.path().join("missing").to_string_lossy().into_owned()],
            include: vec!["**/*.jsonl".into()],
            exclude: Vec::new(),
            follow_symlinks: false,
        })?;

        assert!(discovered.is_empty());
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
}
