use backscroll::input_config::InputConfig;
use tempfile::tempdir;

fn minimal_manifest(id: &str, root: &str) -> String {
    format!(
        r#"version = 1

[[inputs]]
id = "{id}"
source = "session"
active = true

[inputs.discover]
roots = ["{root}"]
include = ["**/*.jsonl"]
exclude = ["**/subagents/**"]

[inputs.decode]
format = "jsonl"

[inputs.record]
selector = "$"

[inputs.map]
role = "$.message.role"
uuid = "$.uuid"
timestamp = "$.timestamp"
session_id = "$.sessionId"

[inputs.content]
selector = "$.message.content"
string = "$"
default_content_type = "text"

[inputs.text]
join = "\n"
trim = true
drop_empty = true
"#
    )
}

#[test]
fn loads_o02_manifests_from_star_inputs_and_inputs_dir() -> miette::Result<()> {
    let dir = tempdir().unwrap();
    std::fs::write(
        dir.path().join("claude.inputs.toml"),
        minimal_manifest("claude", "/claude"),
    )
    .unwrap();
    std::fs::create_dir(dir.path().join("backscroll.inputs.d")).unwrap();
    std::fs::write(
        dir.path().join("backscroll.inputs.d/02-pi.toml"),
        minimal_manifest("pi", "/pi"),
    )
    .unwrap();

    let config = InputConfig::load_from_dir(dir.path())?;

    let active = config.active_inputs();
    assert_eq!(active.len(), 2);
    assert_eq!(active[0].id, "claude");
    assert_eq!(active[0].discover.roots, vec!["/claude"]);
    assert_eq!(active[1].id, "pi");
    assert_eq!(active[1].discover.roots, vec!["/pi"]);
    Ok(())
}

#[test]
fn rejects_invalid_active_manifest_with_clear_path_error() {
    let dir = tempdir().unwrap();
    std::fs::write(
        dir.path().join("broken.inputs.toml"),
        r#"version = 1

[[inputs]]
id = "claude"
source = "session"
active = true
"#,
    )
    .unwrap();

    let err =
        InputConfig::load_from_dir(dir.path()).expect_err("invalid active manifest must fail");
    let msg = err.to_string();
    assert!(msg.contains("broken.inputs.toml"), "{msg}");
    assert!(msg.contains("discover"), "{msg}");
}

#[test]
fn backscroll_toml_does_not_contribute_canonical_inputs() -> miette::Result<()> {
    let dir = tempdir().unwrap();
    std::fs::write(
        dir.path().join("backscroll.toml"),
        r#"database_path = "legacy.db"
session_dirs = ["/legacy"]

[[session_inputs]]
source = "session"
parser = "claude"
paths = ["/legacy-input"]
"#,
    )
    .unwrap();

    let config = InputConfig::load_from_dir(dir.path())?;
    assert!(config.inputs.is_empty());
    Ok(())
}
