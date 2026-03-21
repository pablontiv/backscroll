use crate::core::{ParsedFile, ParsedMessage};
use miette::IntoDiagnostic;
use sha2::{Digest, Sha256};
use std::path::Path;

/// Parse a markdown plan file into sections split by `## ` headers.
pub fn parse_plan(path: &Path) -> miette::Result<ParsedFile> {
    let content = std::fs::read_to_string(path).into_diagnostic()?;
    let hash = format!("{:x}", Sha256::digest(content.as_bytes()));
    let source_path = path.to_string_lossy().into_owned();

    let messages = split_by_headers(&content);

    Ok(ParsedFile {
        source: "plan".into(),
        source_path,
        hash,
        project: None,
        messages,
    })
}

fn split_by_headers(content: &str) -> Vec<ParsedMessage> {
    if content.trim().is_empty() {
        return Vec::new();
    }

    let mut sections: Vec<String> = Vec::new();
    let mut current = String::new();

    for line in content.lines() {
        if line.starts_with("## ") && !current.is_empty() {
            sections.push(std::mem::take(&mut current));
        }
        if !current.is_empty() {
            current.push('\n');
        }
        current.push_str(line);
    }
    if !current.is_empty() {
        sections.push(current);
    }

    sections
        .into_iter()
        .enumerate()
        .map(|(i, text)| ParsedMessage {
            role: "plan".into(),
            text,
            ordinal: i,
            uuid: None,
            timestamp: None,
            content_type: "text".into(),
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;

    #[test]
    fn test_parse_plan_single_section() {
        let dir = tempdir().unwrap();
        let plan = dir.path().join("plan.md");
        fs::write(&plan, "# My Plan\n\nSome content here.").unwrap();

        let result = parse_plan(&plan).unwrap();
        assert_eq!(result.messages.len(), 1);
        assert_eq!(result.messages[0].role, "plan");
        assert!(result.messages[0].text.contains("Some content"));
    }

    #[test]
    fn test_parse_plan_multiple_sections() {
        let dir = tempdir().unwrap();
        let plan = dir.path().join("plan.md");
        fs::write(
            &plan,
            "# Title\n\nIntro\n\n## Section A\n\nContent A\n\n## Section B\n\nContent B\n\n## Section C\n\nContent C",
        )
        .unwrap();

        let result = parse_plan(&plan).unwrap();
        assert_eq!(result.messages.len(), 4); // pre-header + 3 sections
        assert!(result.messages[0].text.contains("Intro"));
        assert!(result.messages[1].text.starts_with("## Section A"));
        assert!(result.messages[2].text.starts_with("## Section B"));
        assert!(result.messages[3].text.starts_with("## Section C"));
    }

    #[test]
    fn test_parse_plan_pre_header_content() {
        let dir = tempdir().unwrap();
        let plan = dir.path().join("plan.md");
        fs::write(&plan, "Preamble text\n\n## First\n\nBody").unwrap();

        let result = parse_plan(&plan).unwrap();
        assert_eq!(result.messages.len(), 2);
        assert!(result.messages[0].text.contains("Preamble"));
        assert!(result.messages[1].text.starts_with("## First"));
    }

    #[test]
    fn test_parse_plan_empty_file() {
        let dir = tempdir().unwrap();
        let plan = dir.path().join("empty.md");
        fs::write(&plan, "").unwrap();

        let result = parse_plan(&plan).unwrap();
        assert!(result.messages.is_empty());
    }

    #[test]
    fn test_parse_plan_snapshot() {
        let dir = tempdir().unwrap();
        let plan = dir.path().join("plan.md");
        fs::write(
            &plan,
            "# Architecture Plan\n\nOverview of the system.\n\n## Database Layer\n\nUse SQLite with FTS5.\n\n## API Layer\n\nREST endpoints with auth.\n",
        )
        .unwrap();

        let result = parse_plan(&plan).unwrap();
        let sections: Vec<(&str, &str)> = result
            .messages
            .iter()
            .map(|m| (m.role.as_str(), m.text.as_str()))
            .collect();
        insta::assert_debug_snapshot!(sections);
    }

    #[test]
    fn test_parse_plan_hash_computed() {
        let dir = tempdir().unwrap();
        let plan = dir.path().join("plan.md");
        fs::write(&plan, "test content").unwrap();

        let result = parse_plan(&plan).unwrap();
        assert!(!result.hash.is_empty());
        assert_eq!(result.hash.len(), 64); // SHA-256 hex
    }
}
