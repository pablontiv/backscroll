use serde::Deserialize;

#[derive(Deserialize, Debug, PartialEq, serde::Serialize)]
#[serde(untagged)]
pub enum MessageContent {
    Text(String),
    Blocks(Vec<ContentBlock>),
}

#[derive(Deserialize, Debug, PartialEq, serde::Serialize)]
pub struct ContentBlock {
    #[serde(rename = "type")]
    pub block_type: String,
    pub text: Option<String>,
}

#[derive(Deserialize, Debug, PartialEq, serde::Serialize)]
pub struct ClaudeMessage {
    pub role: String,
    pub content: MessageContent,
}

#[derive(Deserialize, Debug, PartialEq, serde::Serialize)]
pub struct SessionRecord {
    #[serde(rename = "type")]
    pub record_type: String,
    pub message: Option<ClaudeMessage>,
    pub uuid: Option<String>,
    pub timestamp: Option<String>,
    #[serde(rename = "sessionId")]
    pub session_id: Option<String>,
    pub slug: Option<String>,
    #[serde(rename = "isMeta", default)]
    pub is_meta: Option<bool>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_text_content() {
        let json = r#"{
            "role": "user",
            "content": "Hola mundo"
        }"#;
        let msg: ClaudeMessage = serde_json::from_str(json).unwrap();
        assert_eq!(msg.role, "user");
        assert!(matches!(msg.content, MessageContent::Text(_)));
    }

    #[test]
    fn test_parse_block_content() {
        let json = r#"{
            "role": "assistant",
            "content": [{"type": "text", "text": "Respuesta en bloques"}]
        }"#;
        let msg: ClaudeMessage = serde_json::from_str(json).unwrap();
        assert_eq!(msg.role, "assistant");
        if let MessageContent::Blocks(blocks) = msg.content {
            assert_eq!(blocks[0].text.as_deref(), Some("Respuesta en bloques"));
        } else {
            panic!("Debería ser un bloque");
        }
    }

    #[test]
    fn test_message_snapshots() {
        let json = r#"{
            "role": "assistant",
            "content": [
                {"type": "text", "text": "Iniciando proceso..."}
            ]
        }"#;
        let msg: ClaudeMessage = serde_json::from_str(json).unwrap();
        insta::assert_json_snapshot!(msg);
    }
}
