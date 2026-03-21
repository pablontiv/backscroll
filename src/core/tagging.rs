use crate::core::ParsedMessage;
use regex::Regex;
use std::sync::LazyLock;

struct TagPattern {
    tag: &'static str,
    patterns: Vec<Regex>,
}

static TAG_PATTERNS: LazyLock<Vec<TagPattern>> = LazyLock::new(|| {
    vec![
        TagPattern {
            tag: "debugging",
            patterns: vec![
                Regex::new(r"(?i)\b(debug|debugg|bug|fix|error|exception|stack\s*trace|panic|crash|segfault|traceback)\b").unwrap(),
                Regex::new(r"(?i)\b(breakpoint|gdb|lldb|core\s*dump|assert.*fail)\b").unwrap(),
            ],
        },
        TagPattern {
            tag: "refactoring",
            patterns: vec![
                Regex::new(r"(?i)\b(refactor|restructur|reorganiz|cleanup|clean\s*up|simplif|extract|inline|rename|move\s+to)\b").unwrap(),
                Regex::new(r"(?i)\b(dead\s*code|tech\s*debt|code\s*smell)\b").unwrap(),
            ],
        },
        TagPattern {
            tag: "feature",
            patterns: vec![
                Regex::new(r"(?i)\b(implement|add\s+(a\s+)?feature|new\s+feature|build|create|develop)\b").unwrap(),
                Regex::new(r"(?i)\b(user\s*story|requirement|spec|design)\b").unwrap(),
            ],
        },
        TagPattern {
            tag: "testing",
            patterns: vec![
                Regex::new(r"(?i)\b(unit\s*test|integration\s*test|test\s+case|test\s+coverage|assert|mock|fixture|snapshot\s*test)\b").unwrap(),
                Regex::new(r"(?i)\b(tdd|test.driven|cargo\s+test|pytest|jest|rspec)\b").unwrap(),
            ],
        },
        TagPattern {
            tag: "docs",
            patterns: vec![
                Regex::new(r"(?i)\b(document|readme|changelog|docstring|jsdoc|rustdoc|api\s*doc)\b").unwrap(),
            ],
        },
        TagPattern {
            tag: "config",
            patterns: vec![
                Regex::new(r"(?i)\b(config|configur|settings?|\.toml|\.yaml|\.yml|\.env|environment\s*var)\b").unwrap(),
                Regex::new(r"(?i)\b(ci.cd|pipeline|deploy|docker|kubernetes|helm)\b").unwrap(),
            ],
        },
    ]
});

/// Minimum number of pattern matches required to assign a tag.
const TAG_THRESHOLD: usize = 2;

/// Detect tags for a session based on message content heuristics.
pub fn detect_tags(messages: &[ParsedMessage]) -> Vec<String> {
    let combined: String = messages
        .iter()
        .map(|m| m.text.as_str())
        .collect::<Vec<_>>()
        .join(" ");

    let mut tags = Vec::new();

    for tp in &*TAG_PATTERNS {
        let total_matches: usize = tp
            .patterns
            .iter()
            .map(|re| re.find_iter(&combined).count())
            .sum();

        if total_matches >= TAG_THRESHOLD {
            tags.push(tp.tag.to_string());
        }
    }

    tags
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_msg(text: &str) -> ParsedMessage {
        ParsedMessage {
            role: "user".into(),
            text: text.into(),
            ordinal: 0,
            uuid: None,
            timestamp: None,
            content_type: "text".into(),
        }
    }

    #[test]
    fn test_detect_debugging_tag() {
        let messages = vec![
            make_msg("I have a bug in the authentication code"),
            make_msg("The error message says null pointer exception"),
            make_msg("Let me debug this stack trace"),
        ];
        let tags = detect_tags(&messages);
        assert!(tags.contains(&"debugging".to_string()));
    }

    #[test]
    fn test_detect_refactoring_tag() {
        let messages = vec![
            make_msg("Let's refactor this module to be cleaner"),
            make_msg("We should extract this into a separate function"),
            make_msg("This cleanup will help with maintainability"),
        ];
        let tags = detect_tags(&messages);
        assert!(tags.contains(&"refactoring".to_string()));
    }

    #[test]
    fn test_detect_testing_tag() {
        let messages = vec![
            make_msg("Write a unit test for the parser"),
            make_msg("The test case should cover edge cases"),
            make_msg("Run cargo test to verify"),
        ];
        let tags = detect_tags(&messages);
        assert!(tags.contains(&"testing".to_string()));
    }

    #[test]
    fn test_no_tag_for_generic_content() {
        let messages = vec![make_msg("Hello, how are you today?")];
        let tags = detect_tags(&messages);
        assert!(tags.is_empty());
    }

    #[test]
    fn test_multiple_tags_possible() {
        let messages = vec![
            make_msg("Fix this bug in the test suite"),
            make_msg("The error in the unit test needs debugging"),
            make_msg("Let me write another test case for this fix"),
        ];
        let tags = detect_tags(&messages);
        assert!(tags.contains(&"debugging".to_string()));
        assert!(tags.contains(&"testing".to_string()));
    }
}
