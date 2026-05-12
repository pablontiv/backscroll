use assert_cmd::Command;
use std::fs;
use tempfile::tempdir;

fn input_manifest_dir(config_dir: &std::path::Path) -> std::path::PathBuf {
    config_dir.join("backscroll").join("inputs")
}

fn write_manifest_file(config_dir: &std::path::Path, name: &str, content: String) {
    let dir = input_manifest_dir(config_dir);
    fs::create_dir_all(&dir).unwrap();
    fs::write(dir.join(name), content).unwrap();
}

fn write_decisions_input_manifest(config_dir: &std::path::Path, root: &std::path::Path) {
    write_manifest_file(
        config_dir,
        "decisions.inputs.toml",
        format!(
            r#"version = 1

[[inputs]]
id = "decisions"
source = "decision"
active = true

[inputs.discover]
roots = ["{}"]
include = ["**/*.md"]

[inputs.decode]
format = "markdown"

[inputs.record]
# Decisions are indexed as whole documents
"#,
            root.display()
        ),
    );
}

fn isolated_empty_config_dir() -> &'static std::path::Path {
    static EMPTY_CONFIG_DIR: std::sync::OnceLock<std::path::PathBuf> = std::sync::OnceLock::new();

    EMPTY_CONFIG_DIR
        .get_or_init(|| tempdir().unwrap().keep())
        .as_path()
}

fn backscroll_cmd() -> Command {
    let mut cmd = Command::cargo_bin("backscroll").unwrap();
    cmd.env("BACKSCROLL_CONFIG_DIR", isolated_empty_config_dir())
        .env_remove("BACKSCROLL_SESSION_DIR")
        .env_remove("BACKSCROLL_SESSION_DIRS");
    cmd
}

#[test]
fn snapshot_decisions_query_output() {
    let work_dir = tempdir().unwrap();
    let decisions_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("decisions_snapshot.db");

    // Create test decision files
    fs::write(
        decisions_dir.path().join("auth.md"),
        r#"---
id: DEC-AUTH-001
status: accepted
scope: technical
---

# Implement OAuth 2.0

We've decided to implement OAuth 2.0 as the standard authentication mechanism for all external API access.

## Rationale

OAuth 2.0 provides industry-standard security guarantees and allows for flexible delegation of access rights.

## Consequences

All client applications must be updated to support OAuth 2.0.
"#,
    )
    .unwrap();

    fs::write(
        decisions_dir.path().join("cache.md"),
        r#"---
id: DEC-CACHE-001
status: proposed
scope: architectural
---

# Use Redis for Distributed Caching

This is a proposal to adopt Redis as the standard distributed cache layer.

## Advantages

- High performance key-value store
- Built-in replication and cluster support
- Minimal operational overhead

## Status

Under review by the architecture team.
"#,
    )
    .unwrap();

    write_decisions_input_manifest(work_dir.path(), decisions_dir.path());

    // Sync to index decisions
    backscroll_cmd()
        .current_dir(work_dir.path())
        .arg("sync")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env("BACKSCROLL_CONFIG_DIR", work_dir.path().to_str().unwrap())
        .assert()
        .success();

    // Get JSON output of query
    let assert = backscroll_cmd()
        .current_dir(work_dir.path())
        .arg("decisions")
        .arg("query")
        .arg("--json")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env("BACKSCROLL_CONFIG_DIR", work_dir.path().to_str().unwrap())
        .assert()
        .success();

    let output = String::from_utf8_lossy(&assert.get_output().stdout);
    // Normalize paths for consistent snapshots
    let normalized = output
        .lines()
        .map(|line| {
            let mut json: serde_json::Value = serde_json::from_str(line).unwrap_or_default();
            if let Some(obj) = json.as_object_mut() {
                if let Some(path) = obj.get_mut("source_path") {
                    if let Some(path_str) = path.as_str() {
                        let normalized_path = path_str
                            .split('/')
                            .last()
                            .unwrap_or("unknown.md");
                        *path = serde_json::Value::String(format!("/decisions/{}", normalized_path));
                    }
                }
            }
            json.to_string()
        })
        .collect::<Vec<_>>()
        .join("\n");
    insta::assert_snapshot!(normalized);
}

#[test]
fn snapshot_decisions_context_output() {
    let work_dir = tempdir().unwrap();
    let decisions_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("decisions_context_snapshot.db");

    fs::write(
        decisions_dir.path().join("decision1.md"),
        r#"---
id: DEC-001
status: accepted
scope: technical
---

# First Decision

This is the first important decision. It has been accepted and is now in effect.

It provides the foundation for subsequent decisions.
"#,
    )
    .unwrap();

    fs::write(
        decisions_dir.path().join("decision2.md"),
        r#"---
id: DEC-002
status: proposed
scope: organizational
---

# Second Decision

This decision is still under review and has not been accepted yet.

We are waiting for stakeholder feedback.
"#,
    )
    .unwrap();

    write_decisions_input_manifest(work_dir.path(), decisions_dir.path());

    backscroll_cmd()
        .current_dir(work_dir.path())
        .arg("sync")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env("BACKSCROLL_CONFIG_DIR", work_dir.path().to_str().unwrap())
        .assert()
        .success();

    let assert = backscroll_cmd()
        .current_dir(work_dir.path())
        .arg("decisions")
        .arg("context")
        .arg("--json")
        .arg("--all-projects")
        .env("BACKSCROLL_DATABASE_PATH", db_path.to_str().unwrap())
        .env("BACKSCROLL_CONFIG_DIR", work_dir.path().to_str().unwrap())
        .assert()
        .success();

    let output = String::from_utf8_lossy(&assert.get_output().stdout);
    // Normalize paths for consistent snapshots
    let mut json: serde_json::Value = serde_json::from_str(&output).unwrap_or_default();
    if let Some(obj) = json.as_object_mut() {
        if let Some(decisions) = obj.get_mut("decisions") {
            if let Some(arr) = decisions.as_array_mut() {
                for decision in arr {
                    if let Some(decision_obj) = decision.as_object_mut() {
                        if let Some(path) = decision_obj.get_mut("source_path") {
                            if let Some(path_str) = path.as_str() {
                                let normalized_path = path_str
                                    .split('/')
                                    .last()
                                    .unwrap_or("unknown.md");
                                *path = serde_json::Value::String(format!("/decisions/{}", normalized_path));
                            }
                        }
                    }
                }
            }
        }
    }
    let normalized = json.to_string();
    insta::assert_snapshot!(normalized);
}
