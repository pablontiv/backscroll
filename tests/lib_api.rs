//! Integration test verifying the public library API surface.
//! Exercises the parse → sync → search pipeline as a library consumer (like Kedral).

use backscroll::core::sync::{filter_noise, parse_sessions};
use backscroll::core::{ParsedFile, SearchEngine, SearchParams, SearchResult};
use backscroll::storage::sqlite::Database;
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;
use tempfile::tempdir;

#[test]
fn test_library_parse_sync_search_pipeline() {
    // Setup: create a temp directory with a JSONL session file
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("lib_test.db");

    let session_content = r#"{"type":"human","message":{"role":"human","content":"How do I fix the authentication bug?"},"timestamp":"2026-03-01T10:00:00Z","session_id":"test-session-1"}
{"type":"assistant","message":{"role":"assistant","content":"The authentication bug is caused by an expired token. You need to refresh the OAuth token before making the API call."},"timestamp":"2026-03-01T10:01:00Z","session_id":"test-session-1"}"#;

    fs::write(session_dir.path().join("session.jsonl"), session_content).unwrap();

    // Step 1: Open database and setup schema (as a library consumer would)
    let db = Database::open(db_path.to_str().unwrap()).unwrap();
    db.setup_schema().unwrap();

    // Step 2: Parse sessions
    let hashes: HashMap<String, String> = db.get_file_hashes().unwrap();
    let files: Vec<ParsedFile> =
        parse_sessions(session_dir.path().to_str().unwrap(), &hashes, false).unwrap();
    assert!(!files.is_empty(), "Should parse at least one file");

    // Step 3: Sync to database
    db.sync_files(files).unwrap();

    // Step 4: Search
    let params = SearchParams {
        limit: 10,
        ..SearchParams::default()
    };
    let results: Vec<SearchResult> = db.search("authentication", &params).unwrap();
    assert!(
        !results.is_empty(),
        "Should find results for 'authentication'"
    );
}

#[test]
fn test_filter_noise_exposed() {
    // Verify filter_noise is accessible as a library function
    let clean = filter_noise("This is clean text");
    assert!(clean.is_some());

    // Noise should be filtered
    let noisy = filter_noise("<system-reminder>internal noise</system-reminder>");
    assert!(
        noisy.is_none(),
        "system-reminder tags should be filtered as noise"
    );
}

#[test]
fn test_open_readonly_existing_db() {
    let dir = tempdir().unwrap();
    let db_path = dir.path().join("readonly_test.db");

    // Create and populate DB in read-write mode
    {
        let db = Database::open(db_path.to_str().unwrap()).unwrap();
        db.setup_schema().unwrap();
    }

    // Reopen in read-only mode
    let db = Database::open_readonly(db_path.to_str().unwrap()).unwrap();
    let stats = db.get_stats().unwrap();
    // Fresh DB should have zero sessions
    assert_eq!(stats.file_count, 0);
}

#[test]
fn test_hybrid_search_pipeline() {
    let dir = tempfile::tempdir().unwrap();
    let db_path = dir.path().join("test.db");
    let db = backscroll::storage::sqlite::Database::open(db_path.to_str().unwrap()).unwrap();
    db.setup_schema().unwrap();

    let provider = backscroll::core::embedding::MockEmbeddingProvider::new(384);
    db.set_embedding_provider(Box::new(provider));

    let files = vec![
        backscroll::core::ParsedFile {
            source: "session".to_string(),
            source_path: "/test/session.jsonl".to_string(),
            hash: "s1".to_string(),
            project: Some("homeserver".to_string()),
            messages: vec![backscroll::core::ParsedMessage {
                role: "assistant".to_string(),
                text: "Fixed the Kyverno webhook timeout by increasing to 30 seconds".to_string(),
                ordinal: 0,
                uuid: None,
                timestamp: Some("2026-03-29T10:00:00Z".to_string()),
                content_type: "text".to_string(),
            }],
        },
        backscroll::core::ParsedFile {
            source: "ke".to_string(),
            source_path: "/test/KE-0042.md".to_string(),
            hash: "k1".to_string(),
            project: None,
            messages: vec![backscroll::core::ParsedMessage {
                role: "ke".to_string(),
                text: "Known error: admission webhook failures under cluster load".to_string(),
                ordinal: 0,
                uuid: None,
                timestamp: None,
                content_type: "text".to_string(),
            }],
        },
    ];

    use backscroll::core::SearchEngine;
    db.sync_files(files).unwrap();

    // Hybrid search should find results
    let params = backscroll::core::SearchParams {
        hybrid: true,
        ..Default::default()
    };
    let results = db.search("webhook", &params).unwrap();
    assert!(!results.is_empty(), "Hybrid search should find results");

    // Lexical-only should also find results
    let params = backscroll::core::SearchParams {
        hybrid: false,
        ..Default::default()
    };
    let results = db.search("webhook", &params).unwrap();
    assert!(!results.is_empty(), "Lexical search should find results");

    // Source filter: only KEs
    let params = backscroll::core::SearchParams {
        source: Some("ke".to_string()),
        hybrid: false,
        ..Default::default()
    };
    let results = db.search("webhook", &params).unwrap();
    assert!(!results.is_empty(), "KE source filter should find results");
}

#[test]
fn test_open_readonly_missing_db_fails() {
    let path = PathBuf::from("/tmp/backscroll_nonexistent_test.db");
    assert!(!path.exists());
    let result = Database::open_readonly(path.to_str().unwrap());
    assert!(result.is_err(), "open_readonly on missing file should fail");
}
