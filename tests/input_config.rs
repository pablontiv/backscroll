use backscroll::core::sync::parse_input_definitions;
use backscroll::input_config::InputConfig;
use std::collections::HashMap;
use tempfile::tempdir;

fn toml_path(path: &std::path::Path) -> String {
    path.to_string_lossy().replace('\\', "\\\\")
}

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
    let claude_root = dir.path().join("claude-root");
    let pi_root = dir.path().join("pi-root");
    std::fs::create_dir_all(&claude_root).unwrap();
    std::fs::create_dir_all(&pi_root).unwrap();
    std::fs::write(
        dir.path().join("claude.inputs.toml"),
        minimal_manifest("claude", &toml_path(&claude_root)),
    )
    .unwrap();
    std::fs::create_dir(dir.path().join("backscroll.inputs.d")).unwrap();
    std::fs::write(
        dir.path().join("backscroll.inputs.d/02-pi.toml"),
        minimal_manifest("pi", &toml_path(&pi_root)),
    )
    .unwrap();

    let config = InputConfig::load_from_dir(dir.path())?;

    let active = config.active_inputs();
    assert_eq!(active.len(), 2);
    assert_eq!(active[0].id, "claude");
    assert_eq!(active[0].discover.roots, vec![toml_path(&claude_root)]);
    assert_eq!(active[1].id, "pi");
    assert_eq!(active[1].discover.roots, vec![toml_path(&pi_root)]);
    Ok(())
}

#[test]
fn resolves_relative_roots_against_manifest_location() -> miette::Result<()> {
    let dir = tempdir().unwrap();
    let root = dir.path().join("sessions");
    std::fs::create_dir_all(&root).unwrap();
    std::fs::write(
        dir.path().join("claude.inputs.toml"),
        minimal_manifest("claude", "sessions"),
    )
    .unwrap();

    let config = InputConfig::load_from_dir(dir.path())?;

    assert_eq!(
        config.active_inputs()[0].discover.roots,
        vec![toml_path(&root)]
    );
    Ok(())
}

#[test]
fn rejects_missing_active_discover_root_with_clear_error() {
    let dir = tempdir().unwrap();
    std::fs::write(
        dir.path().join("missing-root.inputs.toml"),
        minimal_manifest("claude", "missing"),
    )
    .unwrap();

    let err = InputConfig::load_from_dir(dir.path()).expect_err("missing root must fail");
    let msg = err.to_string();
    assert!(msg.contains("missing-root.inputs.toml"), "{msg}");
    assert!(msg.contains("discover.roots"), "{msg}");
    assert!(msg.contains("missing"), "{msg}");
}

#[test]
fn rejects_invalid_active_jsonpath_selector_with_clear_error() {
    let dir = tempdir().unwrap();
    let root = dir.path().join("sessions");
    std::fs::create_dir_all(&root).unwrap();
    std::fs::write(
        dir.path().join("invalid-selector.inputs.toml"),
        minimal_manifest("claude", &toml_path(&root))
            .replace("selector = \"$.message.content\"", "selector = \"$[\""),
    )
    .unwrap();

    let err = InputConfig::load_from_dir(dir.path()).expect_err("invalid selector must fail");
    let msg = err.to_string();
    assert!(msg.contains("invalid-selector.inputs.toml"), "{msg}");
    assert!(msg.contains("content.selector"), "{msg}");
    assert!(msg.contains("$["), "{msg}");
}

#[test]
fn rejects_invalid_active_discover_glob_with_clear_error() {
    let dir = tempdir().unwrap();
    let root = dir.path().join("sessions");
    std::fs::create_dir_all(&root).unwrap();
    std::fs::write(
        dir.path().join("invalid-glob.inputs.toml"),
        minimal_manifest("claude", &toml_path(&root))
            .replace("include = [\"**/*.jsonl\"]", "include = [\"[\"]"),
    )
    .unwrap();

    let err = InputConfig::load_from_dir(dir.path()).expect_err("invalid glob must fail");
    let msg = err.to_string();
    assert!(msg.contains("invalid-glob.inputs.toml"), "{msg}");
    assert!(msg.contains("discover.include"), "{msg}");
    assert!(msg.contains("["), "{msg}");
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
fn claude_preset_indexes_fixture_through_generic_input_engine() -> miette::Result<()> {
    let fixture_dir =
        std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("tests/fixtures/claude-preset");
    let manifest_path = fixture_dir.join("claude.inputs.toml");
    let manifest = std::fs::read_to_string(&manifest_path).map_err(|err| {
        miette::miette!(
            "failed to read Claude preset fixture {}: {}",
            manifest_path.display(),
            err
        )
    })?;

    assert!(manifest.contains("source = \"session\""));
    assert!(manifest.contains("exclude = [\"**/subagents/**\"]"));
    for noise in [
        "<system-reminder>",
        "<task-notification>",
        "<caveat>",
        "<local-command-caveat>",
        "<command>",
        "<local-command-stdout>",
        "<command-name>",
        "<command-message>",
        "<command-args>",
        "Base directory:",
        "Caveat:",
        "Request interrupted",
    ] {
        assert!(manifest.contains(noise), "missing noise pattern {noise}");
    }

    let input_config = InputConfig::load_from_dir(&fixture_dir)?;
    let files = parse_input_definitions(&input_config.active_inputs(), &HashMap::new());

    assert_eq!(files.len(), 1);
    let file = &files[0];
    assert_eq!(file.source, "session");
    assert!(file.source_path.ends_with("session-main.jsonl"));
    assert!(!file.source_path.contains("subagents"));
    assert_eq!(file.messages.len(), 3);

    assert_eq!(file.messages[0].role, "user");
    assert_eq!(file.messages[0].text, "hello  world");
    assert_eq!(file.messages[0].uuid.as_deref(), Some("claude-u-1"));
    assert_eq!(
        file.messages[0].timestamp.as_deref(),
        Some("2024-01-02T03:04:05Z")
    );

    assert_eq!(file.messages[1].role, "assistant");
    assert_eq!(file.messages[1].text, "assistant visible text");
    assert_eq!(file.messages[1].uuid.as_deref(), Some("claude-a-1"));
    assert_eq!(
        file.messages[1].timestamp.as_deref(),
        Some("2024-01-02T03:04:06Z")
    );

    assert_eq!(file.messages[2].role, "user");
    assert_eq!(file.messages[2].text, "visible tail");
    assert_eq!(file.messages[2].uuid.as_deref(), Some("claude-u-2"));
    assert_eq!(
        file.messages[2].timestamp.as_deref(),
        Some("2024-01-02T03:04:07Z")
    );

    Ok(())
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
