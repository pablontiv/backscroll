use crate::config::SourcesConfig;
use crate::core::plans::split_by_headers;
use crate::core::{ParsedFile, ParsedMessage};
use miette::IntoDiagnostic;
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::path::Path;
use walkdir::WalkDir;

pub const SOURCE_KE: &str = "ke";
pub const SOURCE_DECISION: &str = "decision";
pub const SOURCE_MEMORY: &str = "memory";
pub const SOURCE_RULE: &str = "rule";
pub const SOURCE_SPEC: &str = "spec";
pub const SOURCE_BACKLOG: &str = "backlog";

/// Extract YAML frontmatter from markdown content.
/// Returns (frontmatter_as_json_string, rest_of_content).
pub fn parse_frontmatter(content: &str) -> (Option<String>, &str) {
    let trimmed = content.trim_start();
    if !trimmed.starts_with("---") {
        return (None, content);
    }
    // Find closing ---
    let after_open = &trimmed[3..];
    // skip optional newline after opening ---
    let after_open = after_open.trim_start_matches('\n').trim_start_matches('\r');
    if let Some(close_pos) = after_open.find("\n---") {
        let yaml_str = &after_open[..close_pos];
        let rest_start = close_pos + 4; // skip \n---
        let rest = &after_open[rest_start..];
        // strip optional trailing newline after closing ---
        let rest = rest.trim_start_matches('\n').trim_start_matches('\r');

        // Parse simple key: value YAML lines into a JSON object
        let json_str = {
            let mut map = serde_json::Map::new();
            for line in yaml_str.lines() {
                if let Some(colon_pos) = line.find(':') {
                    let key = line[..colon_pos].trim().to_owned();
                    let val = line[colon_pos + 1..].trim().to_owned();
                    if !key.is_empty() {
                        map.insert(key, serde_json::Value::String(val));
                    }
                }
            }
            if map.is_empty() {
                None
            } else {
                serde_json::to_string(&serde_json::Value::Object(map)).ok()
            }
        };
        // Find rest in original content to return a proper slice
        // We return rest as a &str pointing into the original content
        let offset = content.len() - content.trim_start().len()
            + (trimmed.len() - after_open.len())
            + close_pos
            + 4
            + (after_open[rest_start..].len() - rest.len());
        (json_str, &content[offset..])
    } else {
        (None, content)
    }
}

/// Parse a whole document as a single ParsedMessage.
fn parse_whole_document(path: &Path, source_type: &str) -> miette::Result<ParsedFile> {
    let content = std::fs::read_to_string(path).into_diagnostic()?;
    let hash = format!("{:x}", Sha256::digest(content.as_bytes()));
    let source_path = path.to_string_lossy().into_owned();

    let messages = if content.trim().is_empty() {
        Vec::new()
    } else {
        vec![ParsedMessage {
            role: source_type.to_owned(),
            text: content.clone(),
            ordinal: 0,
            uuid: None,
            timestamp: None,
            content_type: "text".into(),
        }]
    };

    Ok(ParsedFile {
        source: source_type.to_owned(),
        source_path,
        hash,
        project: None,
        messages,
    })
}

/// Parse a sectioned document (split by ## headers), reusing split_by_headers.
fn parse_sectioned_document(path: &Path, source_type: &str) -> miette::Result<ParsedFile> {
    let content = std::fs::read_to_string(path).into_diagnostic()?;
    let hash = format!("{:x}", Sha256::digest(content.as_bytes()));
    let source_path = path.to_string_lossy().into_owned();

    // split_by_headers returns messages with role="plan"; patch the role
    let messages = split_by_headers(&content)
        .into_iter()
        .map(|mut m| {
            m.role = source_type.to_owned();
            m
        })
        .collect();

    Ok(ParsedFile {
        source: source_type.to_owned(),
        source_path,
        hash,
        project: None,
        messages,
    })
}

pub fn parse_ke(path: &Path) -> miette::Result<ParsedFile> {
    parse_whole_document(path, SOURCE_KE)
}

pub fn parse_decision(path: &Path) -> miette::Result<ParsedFile> {
    parse_whole_document(path, SOURCE_DECISION)
}

pub fn parse_memory(path: &Path) -> miette::Result<ParsedFile> {
    parse_whole_document(path, SOURCE_MEMORY)
}

pub fn parse_rule(path: &Path) -> miette::Result<ParsedFile> {
    parse_whole_document(path, SOURCE_RULE)
}

pub fn parse_spec(path: &Path) -> miette::Result<ParsedFile> {
    parse_sectioned_document(path, SOURCE_SPEC)
}

pub fn parse_backlog(path: &Path) -> miette::Result<ParsedFile> {
    parse_whole_document(path, SOURCE_BACKLOG)
}

/// Walk a directory path (max depth 3) and collect .md files.
fn collect_md_files(dir: &str) -> Vec<std::path::PathBuf> {
    if dir.is_empty() {
        return Vec::new();
    }
    WalkDir::new(dir)
        .max_depth(3)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| e.file_type().is_file() && e.path().extension().is_some_and(|ext| ext == "md"))
        .map(|e| e.path().to_path_buf())
        .collect()
}

pub struct SourceRegistry {
    config: SourcesConfig,
}

impl SourceRegistry {
    pub fn from_config(sources: &SourcesConfig) -> Self {
        Self {
            config: sources.clone(),
        }
    }

    /// Parse all source files, skipping those whose hash is already in `existing_hashes`.
    pub fn parse_all(
        &self,
        existing_hashes: &HashMap<String, String>,
    ) -> miette::Result<Vec<ParsedFile>> {
        let mut results = Vec::new();

        type SourceParser = fn(&Path) -> miette::Result<ParsedFile>;
        let dirs_and_parsers: &[(&[String], SourceParser)] = &[
            (&self.config.ke, parse_ke),
            (&self.config.decisions, parse_decision),
            (&self.config.memories, parse_memory),
            (&self.config.rules, parse_rule),
            (&self.config.specs, parse_spec),
            (&self.config.backlog, parse_backlog),
        ];

        for (dirs, parser) in dirs_and_parsers {
            for dir in *dirs {
                if dir.is_empty() {
                    continue;
                }
                for file_path in collect_md_files(dir) {
                    let path_str = file_path.to_string_lossy().into_owned();
                    // Quick hash check: read content to get hash
                    let content = match std::fs::read_to_string(&file_path) {
                        Ok(c) => c,
                        Err(_) => continue,
                    };
                    let hash = format!("{:x}", Sha256::digest(content.as_bytes()));
                    if existing_hashes.get(&path_str) == Some(&hash) {
                        continue;
                    }
                    match parser(&file_path) {
                        Ok(parsed) => results.push(parsed),
                        Err(e) => {
                            eprintln!("Warning: failed to parse {path_str}: {e}");
                        }
                    }
                }
            }
        }

        Ok(results)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    fn fixture(name: &str) -> PathBuf {
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("tests/fixtures")
            .join(name)
    }

    #[test]
    fn test_parse_ke_whole_document() {
        let result = parse_ke(&fixture("ke-001.md")).unwrap();
        assert_eq!(result.source, SOURCE_KE);
        assert_eq!(result.messages.len(), 1);
        assert_eq!(result.messages[0].role, SOURCE_KE);
        assert_eq!(result.messages[0].content_type, "text");
        assert_eq!(result.messages[0].ordinal, 0);
        assert!(result.messages[0].text.contains("Kyverno"));
        assert!(!result.hash.is_empty());
    }

    #[test]
    fn test_parse_decision() {
        let result = parse_decision(&fixture("dec-001.md")).unwrap();
        assert_eq!(result.source, SOURCE_DECISION);
        assert_eq!(result.messages.len(), 1);
        assert_eq!(result.messages[0].role, SOURCE_DECISION);
        assert!(result.messages[0].text.contains("NFS"));
    }

    #[test]
    fn test_parse_memory() {
        let result = parse_memory(&fixture("memory-test.md")).unwrap();
        assert_eq!(result.source, SOURCE_MEMORY);
        assert_eq!(result.messages.len(), 1);
        assert!(result.messages[0].text.contains("feedback_example"));
    }

    #[test]
    fn test_parse_rule_no_frontmatter() {
        let result = parse_rule(&fixture("rule-test.md")).unwrap();
        assert_eq!(result.source, SOURCE_RULE);
        assert_eq!(result.messages.len(), 1);
        assert!(result.messages[0].text.contains("Test Rule"));
    }

    #[test]
    fn test_parse_spec_sectioned() {
        let result = parse_spec(&fixture("spec-test.md")).unwrap();
        assert_eq!(result.source, SOURCE_SPEC);
        // Should have 3 sections: preamble + Section One + Section Two
        assert_eq!(result.messages.len(), 3);
        assert!(result.messages[0].role == SOURCE_SPEC);
        assert!(result.messages[1].text.contains("Section One"));
        assert!(result.messages[2].text.contains("Section Two"));
    }

    #[test]
    fn test_parse_backlog() {
        let result = parse_backlog(&fixture("backlog-test.md")).unwrap();
        assert_eq!(result.source, SOURCE_BACKLOG);
        assert_eq!(result.messages.len(), 1);
        assert!(result.messages[0].text.contains("B099"));
    }

    #[test]
    fn test_parse_frontmatter_present() {
        let content = "---\nid: KE-0001\nestado: activo\n---\n\n# Title\n";
        let (fm, rest) = parse_frontmatter(content);
        assert!(fm.is_some());
        let fm_str = fm.unwrap();
        assert!(fm_str.contains("KE-0001"));
        assert!(rest.contains("Title"));
        assert!(!rest.contains("---"));
    }

    #[test]
    fn test_parse_frontmatter_absent() {
        let content = "# Title\n\nNo frontmatter here.";
        let (fm, rest) = parse_frontmatter(content);
        assert!(fm.is_none());
        assert!(rest.contains("Title"));
    }

    #[test]
    fn test_source_registry_dedup() {
        let fixtures_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("tests/fixtures")
            .to_string_lossy()
            .into_owned();

        let config = SourcesConfig {
            ke: vec![fixtures_dir.clone()],
            decisions: vec![],
            memories: vec![],
            rules: vec![],
            specs: vec![],
            backlog: vec![],
        };
        let registry = SourceRegistry::from_config(&config);

        // First parse: no existing hashes
        let first = registry.parse_all(&HashMap::new()).unwrap();
        assert!(!first.is_empty());

        // Second parse: all hashes present — should return empty
        let existing: HashMap<String, String> = first
            .iter()
            .map(|f| (f.source_path.clone(), f.hash.clone()))
            .collect();
        let second = registry.parse_all(&existing).unwrap();
        assert!(second.is_empty());
    }

    #[test]
    fn test_source_registry_empty_path_skipped() {
        let config = SourcesConfig {
            ke: vec!["".to_owned()],
            decisions: vec![],
            memories: vec![],
            rules: vec![],
            specs: vec![],
            backlog: vec![],
        };
        let registry = SourceRegistry::from_config(&config);
        let result = registry.parse_all(&HashMap::new()).unwrap();
        assert!(result.is_empty());
    }
}
