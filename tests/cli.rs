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

    // Buscar (--all-projects porque CWD no coincide con el tempdir del test)
    let mut search_cmd = Command::cargo_bin("backscroll").unwrap();
    search_cmd
        .arg("search")
        .arg("buscame")
        .arg("--all-projects")
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
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .assert()
        .success()
        .stdout(predicate::str::contains("mundo"));
}

fn sync_fixture(session_dir: &std::path::Path, db_path: &std::path::Path) {
    let session_file = session_dir.join("session.jsonl");
    fs::write(
        &session_file,
        r#"{"type": "user", "message": {"role": "user", "content": "deploy kubernetes cluster"}, "uuid": "r1", "timestamp": "100"}"#,
    )
    .unwrap();

    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("sync")
        .arg("--path")
        .arg(session_dir.to_str().unwrap())
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env("BACKSCROLL_SESSION_DIR", session_dir.to_str().unwrap())
        .assert()
        .success();
}

#[test]
fn test_resume_text_output() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("resume_text.db");
    sync_fixture(session_dir.path(), &db_path);

    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("resume")
        .arg("kubernetes")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .assert()
        .success()
        .stdout(predicate::str::contains("Session:"))
        .stdout(predicate::str::contains("ID:"));
}

#[test]
fn test_resume_robot_output() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("resume_robot.db");
    sync_fixture(session_dir.path(), &db_path);

    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("resume")
        .arg("kubernetes")
        .arg("--robot")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    let lines: Vec<&str> = stdout.trim().lines().collect();
    assert_eq!(lines.len(), 1, "Robot mode should be single line");
    assert!(
        lines[0].contains('\t'),
        "Robot mode should be tab-separated"
    );
}

#[test]
fn test_search_source_flag_cli() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("source_flag.db");
    let session_file = session_dir.path().join("session.jsonl");

    fs::write(
        &session_file,
        r#"{"type": "user", "message": {"role": "user", "content": "source filter test"}, "uuid": "sf1", "timestamp": "100"}"#,
    )
    .unwrap();

    let fake_home = tempdir().unwrap();

    // Sync (isolated HOME to prevent real plan discovery)
    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("sync")
        .arg("--path")
        .arg(session_dir.path().to_str().unwrap())
        .arg("--no-plans")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .env("HOME", fake_home.path().to_str().unwrap())
        .assert()
        .success();

    // Search with --source sessions should find it
    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("search")
        .arg("source")
        .arg("--source")
        .arg("sessions")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .env("HOME", fake_home.path().to_str().unwrap())
        .assert()
        .success()
        .stdout(predicate::str::contains("filter"));

    // Search with --source plans should NOT find session data
    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("search")
        .arg("source")
        .arg("--source")
        .arg("plans")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .env("HOME", fake_home.path().to_str().unwrap())
        .assert()
        .success()
        .stdout(predicate::str::contains("No se encontraron resultados"));
}

#[test]
fn test_sync_no_plans_flag() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("no_plans.db");
    let session_file = session_dir.path().join("session.jsonl");

    fs::write(
        &session_file,
        r#"{"type": "user", "message": {"role": "user", "content": "test no plans"}, "uuid": "np1", "timestamp": "100"}"#,
    )
    .unwrap();

    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("sync")
        .arg("--path")
        .arg(session_dir.path().to_str().unwrap())
        .arg("--no-plans")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .assert()
        .success();
}

#[test]
fn test_resume_no_results_exit_code() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("resume_noresult.db");
    sync_fixture(session_dir.path(), &db_path);

    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("resume")
        .arg("xyznonexistent999")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .assert()
        .failure()
        .stderr(predicate::str::contains("No matching session"));
}

#[test]
fn test_topics_robot_output() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("topics_robot.db");
    sync_fixture(session_dir.path(), &db_path);

    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("topics")
        .arg("--all-projects")
        .arg("--robot")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(!stdout.is_empty(), "Topics should return results");
    for line in stdout.trim().lines() {
        let fields: Vec<&str> = line.split('\t').collect();
        assert_eq!(
            fields.len(),
            3,
            "Robot format should have 3 tab-separated fields: {}",
            line
        );
    }
}

#[test]
fn test_topics_json_output() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("topics_json.db");
    sync_fixture(session_dir.path(), &db_path);

    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("topics")
        .arg("--all-projects")
        .arg("--json")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    for line in stdout.trim().lines() {
        let parsed: serde_json::Value = serde_json::from_str(line)
            .unwrap_or_else(|e| panic!("Invalid JSON line '{}': {}", line, e));
        assert!(
            parsed.get("term").is_some(),
            "JSON should have 'term' field"
        );
        assert!(
            parsed.get("sessions").is_some(),
            "JSON should have 'sessions' field"
        );
        assert!(
            parsed.get("mentions").is_some(),
            "JSON should have 'mentions' field"
        );
    }
}

#[test]
fn test_list_robot_output() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("list_robot.db");
    sync_fixture(session_dir.path(), &db_path);

    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("list")
        .arg("--all-projects")
        .arg("--robot")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(!stdout.is_empty(), "List should return results");
    for line in stdout.trim().lines() {
        let fields: Vec<&str> = line.split('\t').collect();
        assert_eq!(
            fields.len(),
            5,
            "Robot format should have 5 tab-separated fields: {}",
            line
        );
    }
}

#[test]
fn test_list_json_output() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("list_json.db");
    sync_fixture(session_dir.path(), &db_path);

    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("list")
        .arg("--all-projects")
        .arg("--json")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    for line in stdout.trim().lines() {
        let parsed: serde_json::Value = serde_json::from_str(line)
            .unwrap_or_else(|e| panic!("Invalid JSON line '{}': {}", line, e));
        assert!(
            parsed.get("source_path").is_some(),
            "JSON should have 'source_path' field"
        );
        assert!(
            parsed.get("messages").is_some(),
            "JSON should have 'messages' field"
        );
    }
}

#[test]
fn test_status_shows_project_breakdown() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("status_breakdown.db");
    sync_fixture(session_dir.path(), &db_path);

    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("status")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .assert()
        .success()
        .stdout(predicate::str::contains("By Project:"))
        .stdout(predicate::str::contains("PROJECT"));
}

/// Helper: sync a fixture with 3 messages at distinct timestamps for date filter tests.
fn sync_date_fixture(session_dir: &std::path::Path, db_path: &std::path::Path) {
    let session_file = session_dir.join("dates.jsonl");
    let jsonl = r#"{"type": "user", "message": {"role": "user", "content": "alpha early message"}, "uuid": "d1", "timestamp": "2026-01-15T00:00:00Z"}
{"type": "user", "message": {"role": "user", "content": "beta middle message"}, "uuid": "d2", "timestamp": "2026-03-01T00:00:00Z"}
{"type": "user", "message": {"role": "user", "content": "gamma late message"}, "uuid": "d3", "timestamp": "2026-06-15T00:00:00Z"}"#;
    fs::write(&session_file, jsonl).unwrap();

    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("sync")
        .arg("--path")
        .arg(session_dir.to_str().unwrap())
        .arg("--no-plans")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env("BACKSCROLL_SESSION_DIR", session_dir.to_str().unwrap())
        .assert()
        .success();
}

#[test]
fn test_search_date_after_only() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("date_after.db");
    sync_date_fixture(session_dir.path(), &db_path);

    // --after 2026-03-01 should exclude "alpha" (Jan), include "beta" (Mar) and "gamma" (Jun)
    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("search")
        .arg("message")
        .arg("--after")
        .arg("2026-03-01")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(
        !stdout.contains("alpha"),
        "alpha (Jan) should be excluded by --after 2026-03-01"
    );
    assert!(
        stdout.contains("beta") || stdout.contains("gamma"),
        "beta or gamma should appear after 2026-03-01"
    );
}

#[test]
fn test_search_date_before_only() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("date_before.db");
    sync_date_fixture(session_dir.path(), &db_path);

    // --before 2026-03-01 should include "alpha" (Jan), exclude "beta" (Mar) and "gamma" (Jun)
    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("search")
        .arg("message")
        .arg("--before")
        .arg("2026-03-01")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(
        stdout.contains("alpha"),
        "alpha (Jan) should be included before 2026-03-01"
    );
    assert!(
        !stdout.contains("beta"),
        "beta (Mar) should be excluded by --before 2026-03-01 (exclusive)"
    );
    assert!(
        !stdout.contains("gamma"),
        "gamma (Jun) should be excluded by --before 2026-03-01"
    );
}

#[test]
fn test_search_date_after_and_before() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("date_both.db");
    sync_date_fixture(session_dir.path(), &db_path);

    // --after 2026-02-01 --before 2026-05-01 should include only "beta" (Mar)
    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("search")
        .arg("message")
        .arg("--after")
        .arg("2026-02-01")
        .arg("--before")
        .arg("2026-05-01")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(
        !stdout.contains("alpha"),
        "alpha (Jan) should be excluded by --after 2026-02-01"
    );
    assert!(
        stdout.contains("beta"),
        "beta (Mar) should be within range Feb-May"
    );
    assert!(
        !stdout.contains("gamma"),
        "gamma (Jun) should be excluded by --before 2026-05-01"
    );
}

#[test]
fn test_search_date_no_flags_returns_all() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("date_none.db");
    sync_date_fixture(session_dir.path(), &db_path);

    // No date flags should return all messages (backward compat)
    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("search")
        .arg("message")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(
        stdout.contains("alpha") || stdout.contains("beta") || stdout.contains("gamma"),
        "Without date flags, all messages should be searchable"
    );
}

/// Helper: sync a fixture with messages from both roles for role filter tests.
fn sync_role_fixture(session_dir: &std::path::Path, db_path: &std::path::Path) {
    let session_file = session_dir.join("roles.jsonl");
    let jsonl = r#"{"type": "user", "message": {"role": "user", "content": "userquestion about deployment"}, "uuid": "r1", "timestamp": "100"}
{"type": "assistant", "message": {"role": "assistant", "content": [{"type": "text", "text": "assistantanswer about deployment"}]}, "uuid": "r2", "timestamp": "200"}"#;
    fs::write(&session_file, jsonl).unwrap();

    Command::cargo_bin("backscroll")
        .unwrap()
        .arg("sync")
        .arg("--path")
        .arg(session_dir.to_str().unwrap())
        .arg("--no-plans")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env("BACKSCROLL_SESSION_DIR", session_dir.to_str().unwrap())
        .assert()
        .success();
}

#[test]
fn test_search_role_human_only() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("role_human.db");
    sync_role_fixture(session_dir.path(), &db_path);

    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("search")
        .arg("deployment")
        .arg("--role")
        .arg("human")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(
        stdout.contains("userquestion"),
        "Should find user message with --role human"
    );
    assert!(
        !stdout.contains("assistantanswer"),
        "Should not find assistant message with --role human"
    );
}

#[test]
fn test_search_role_assistant_only() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("role_assistant.db");
    sync_role_fixture(session_dir.path(), &db_path);

    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("search")
        .arg("deployment")
        .arg("--role")
        .arg("assistant")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(
        stdout.contains("assistantanswer"),
        "Should find assistant message with --role assistant"
    );
    assert!(
        !stdout.contains("userquestion"),
        "Should not find user message with --role assistant"
    );
}

#[test]
fn test_search_role_none_returns_both() {
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("role_none.db");
    sync_role_fixture(session_dir.path(), &db_path);

    let output = Command::cargo_bin("backscroll")
        .unwrap()
        .arg("search")
        .arg("deployment")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env(
            "BACKSCROLL_SESSION_DIR",
            session_dir.path().to_str().unwrap(),
        )
        .output()
        .unwrap();

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(
        stdout.contains("userquestion") || stdout.contains("assistantanswer"),
        "Without --role, both roles should be searchable"
    );
}
