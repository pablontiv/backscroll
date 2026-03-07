use assert_cmd::Command;
use predicates::prelude::*;
use std::fs;
use tempfile::tempdir;

#[test]
fn test_cli_help() {
    let mut cmd = Command::cargo_bin("backscroll").unwrap();
    cmd.arg("--help")
        .assert()
        .success()
        .stdout(predicate::str::contains("Tier 2 search"));
}

#[test]
fn test_cli_status() {
    let mut cmd = Command::cargo_bin("backscroll").unwrap();
    cmd.arg("status")
        .assert()
        .success()
        .stdout(predicate::str::contains("Estado del índice: OK"));
}

#[test]
fn test_cli_sync_and_search() {
    let dir = tempdir().unwrap();
    let db_path = dir.path().join("test_cli.db");
    let session_file = dir.path().join("session.jsonl");
    
    fs::write(&session_file, r#"{"role": "user", "content": "buscame esto"}"#).unwrap();

    // Sincronizar
    let mut sync_cmd = Command::cargo_bin("backscroll").unwrap();
    sync_cmd.arg("sync")
        .arg("--path")
        .arg(dir.path().to_str().unwrap())
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .assert()
        .success();

    // Buscar
    let mut search_cmd = Command::cargo_bin("backscroll").unwrap();
    search_cmd.arg("search")
        .arg("buscame")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .assert()
        .success()
        .stdout(predicate::str::contains("buscame esto"));
}
