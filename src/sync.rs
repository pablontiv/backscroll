use std::fs;
use std::path::Path;
use sha2::{Sha256, Digest};
use walkdir::WalkDir;
use miette::IntoDiagnostic;
use crate::db::Database;
use crate::models::{ClaudeMessage, MessageContent};

pub fn compute_hash(path: impl AsRef<Path>) -> miette::Result<String> {
    let data = fs::read(path).into_diagnostic()?;
    let mut hasher = Sha256::new();
    hasher.update(data);
    Ok(hex::encode(hasher.finalize()))
}

pub fn sync_sessions(db: &Database, session_dir: &str) -> miette::Result<()> {
    for entry in WalkDir::new(session_dir)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| e.file_type().is_file() && e.path().extension().map_or(false, |ext| ext == "json" || ext == "jsonl"))
    {
        let path_str = entry.path().to_string_lossy().to_string();
        let hash = compute_hash(entry.path())?;

        if db.is_file_changed(&path_str, &hash)? {
            let content = fs::read_to_string(entry.path()).into_diagnostic()?;
            
            for line in content.lines() {
                if let Ok(msg) = serde_json::from_str::<ClaudeMessage>(line) {
                    let text_content = match &msg.content {
                        MessageContent::Text(t) => t.clone(),
                        MessageContent::Blocks(blocks) => {
                            blocks.iter().filter_map(|b| b.text.clone()).collect::<Vec<_>>().join(" ")
                        }
                    };
                    
                    if !text_content.is_empty() {
                        db.index_message(&path_str, &msg.role, &text_content, None)?;
                    }
                }
            }
            
            db.mark_file_indexed(&path_str, &hash)?;
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_sync_workflow() -> miette::Result<()> {
        let db = Database::open(":memory:")?;
        db.setup_schema()?;

        let dir = tempdir().into_diagnostic()?;
        let file_path = dir.path().join("session.jsonl");
        
        fs::write(&file_path, r#"{"role": "user", "content": "hola"}"#).into_diagnostic()?;

        // Sincronizar por primera vez
        sync_sessions(&db, dir.path().to_str().unwrap())?;
        
        let results = db.search("hola", None)?;
        assert_eq!(results.len(), 1);

        // Sincronizar de nuevo sin cambios (no debería fallar ni duplicar)
        sync_sessions(&db, dir.path().to_str().unwrap())?;
        let results = db.search("hola", None)?;
        assert_eq!(results.len(), 1);

        Ok(())
    }
}
