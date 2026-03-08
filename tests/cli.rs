#![allow(deprecated)]
use assert_cmd::Command;
use predicates::prelude::*;
use std::fs;
use tempfile::tempdir;

#[test]
fn test_cli_help() {
    let dir = tempdir().unwrap();
    let db_path = dir.path().join("help.db");

    let mut cmd = Command::cargo_bin("backscroll").unwrap();
    cmd.arg("--help")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env("BACKSCROLL_SESSION_DIR", dir.path().to_str().unwrap())
        .assert()
        .success()
        .stdout(predicate::str::contains("Tier 2 search"));
}

#[test]
fn test_cli_status() {
    let dir = tempdir().unwrap();
    let db_path = dir.path().join("status.db");

    let mut cmd = Command::cargo_bin("backscroll").unwrap();
    cmd.arg("status")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env("BACKSCROLL_SESSION_DIR", dir.path().to_str().unwrap())
        .assert()
        .success()
        .stdout(predicate::str::contains("Backscroll Index Status"));
}

#[test]
fn test_cli_sync_and_search() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("test_cli.db");
    let session_file = session_dir.path().join("session.jsonl");

    fs::write(
        &session_file,
        r#"{"type": "user", "message": {"role": "user", "content": "buscame esto"}, "uuid": "123", "timestamp": "12345"}"#,
    )
    .unwrap();

    // Sincronizar
    let mut sync_cmd = Command::cargo_bin("backscroll").unwrap();
    sync_cmd
        .arg("sync")
        .arg("--path")
        .arg(session_dir.path().to_str().unwrap())
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .assert()
        .success();

    // Buscar
    let mut search_cmd = Command::cargo_bin("backscroll").unwrap();
    search_cmd
        .arg("search")
        .arg("buscame")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .assert()
        .success()
        .stdout(predicate::str::contains("esto"));
}

#[test]
fn test_parse_real_jsonl() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("test_real_jsonl.db");
    let session_file = session_dir.path().join("session.jsonl");

    let real_jsonl = r#"
{"type": "user", "message": {"role": "user", "content": "hola"}, "uuid": "abc", "timestamp": "123"}
{"type": "progress", "uuid": "def"}
{"type": "assistant", "message": {"role": "assistant", "content": [{"type": "text", "text": "mundo"}]}, "uuid": "ghi", "timestamp": "456"}
"#;
    fs::write(&session_file, real_jsonl).unwrap();

    let mut sync_cmd = Command::cargo_bin("backscroll").unwrap();
    sync_cmd
        .arg("sync")
        .arg("--path")
        .arg(session_dir.path().to_str().unwrap())
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .assert()
        .success();

    let mut search_cmd = Command::cargo_bin("backscroll").unwrap();
    search_cmd
        .arg("search")
        .arg("mundo")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .assert()
        .success()
        .stdout(predicate::str::contains("mundo"));
}
