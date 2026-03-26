//! Integration test verifying the public library API surface.
//! Exercises the parse → sync → search pipeline as a library consumer (like Kedral).

use backscroll::core::sync::{filter_noise, parse_sessions};
use backscroll::core::{ParsedFile, SearchEngine, SearchParams, SearchResult};
use backscroll::storage::sqlite::Database;
use std::collections::HashMap;
use std::fs;
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
