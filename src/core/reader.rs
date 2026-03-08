use crate::core::ParsedMessage;
use crate::core::models::{MessageContent, SessionRecord};
use crate::core::sync::filter_noise;
use std::fs;
use std::path::Path;

pub fn read_session(path: &Path) -> miette::Result<Vec<ParsedMessage>> {
    let content = fs::read_to_string(path).map_err(|e| miette::miette!(e))?;
    let mut messages = Vec::new();

    for (ordinal, line) in content.lines().enumerate() {
        if let Ok(record) = serde_json::from_str::<SessionRecord>(line) {
            if record.is_meta == Some(true) {
                continue;
            }
            if record.record_type != "user" && record.record_type != "assistant" {
                continue;
            }

            if let Some(msg) = record.message {
                let text_content = match &msg.content {
                    MessageContent::Text(t) => t.clone(),
                    MessageContent::Blocks(blocks) => blocks
                        .iter()
                        .filter(|b| b.block_type != "tool_use" && b.block_type != "tool_result")
                        .filter_map(|b| b.text.clone())
                        .collect::<Vec<_>>()
                        .join(" "),
                };

                if let Some(cleaned) = filter_noise(&text_content) {
                    if !cleaned.is_empty() {
                        messages.push(ParsedMessage {
                            role: msg.role,
                            text: cleaned,
                            ordinal,
                            uuid: record.uuid,
                            timestamp: record.timestamp,
                        });
                    }
                }
            }
        }
    }
    Ok(messages)
}
