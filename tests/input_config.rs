use backscroll::core::sync::parse_input_definitions;
use backscroll::input_config::InputConfig;
use std::collections::HashMap;
use std::sync::{Mutex, MutexGuard};
use tempfile::tempdir;

static ENV_LOCK: Mutex<()> = Mutex::new(());

struct ConfigDirEnv {
    _guard: MutexGuard<'static, ()>,
    previous: Option<std::ffi::OsString>,
}

impl ConfigDirEnv {
    fn set(path: &std::path::Path) -> Self {
        let guard = ENV_LOCK.lock().unwrap();
        let previous = std::env::var_os("BACKSCROLL_CONFIG_DIR");
        unsafe {
            std::env::set_var("BACKSCROLL_CONFIG_DIR", path);
        }
        Self {
            _guard: guard,
            previous,
        }
    }
}

impl Drop for ConfigDirEnv {
    fn drop(&mut self) {
        unsafe {
            if let Some(previous) = &self.previous {
                std::env::set_var("BACKSCROLL_CONFIG_DIR", previous);
            } else {
                std::env::remove_var("BACKSCROLL_CONFIG_DIR");
            }
        }
    }
}

fn input_dir(config_dir: &std::path::Path) -> std::path::PathBuf {
    config_dir.join("backscroll").join("inputs")
}

fn write_input_manifest(config_dir: &std::path::Path, name: &str, manifest: String) {
    let dir = input_dir(config_dir);
    std::fs::create_dir_all(&dir).unwrap();
    std::fs::write(dir.join(name), manifest).unwrap();
}

fn toml_path(path: &std::path::Path) -> String {
    path.to_string_lossy().replace('\\', "\\\\")
}

fn minimal_manifest(id: &str, root: &str) -> String {
    minimal_manifest_with_roots(id, &[root])
}

fn minimal_manifest_with_roots(id: &str, roots: &[&str]) -> String {
    let roots = roots
        .iter()
        .map(|root| format!("\"{root}\""))
        .collect::<Vec<_>>()
        .join(", ");
    format!(
        r#"version = 1

[[inputs]]
id = "{id}"
source = "session"
active = true

[inputs.discover]
roots = [{roots}]
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
fn loads_global_inputs_from_config_dir_only() -> miette::Result<()> {
    let config_dir = tempdir().unwrap();
    let cwd = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    let claude_root = config_dir.path().join("claude-root");
    let pi_root = config_dir.path().join("pi-root");
    std::fs::create_dir_all(&claude_root).unwrap();
    std::fs::create_dir_all(&pi_root).unwrap();
    write_input_manifest(
        config_dir.path(),
        "01-claude.inputs.toml",
        minimal_manifest("claude", &toml_path(&claude_root)),
    );
    write_input_manifest(
        config_dir.path(),
        "02-pi.inputs.toml",
        minimal_manifest("pi", &toml_path(&pi_root)),
    );
    std::fs::write(
        input_dir(config_dir.path()).join("03-ignored.toml"),
        minimal_manifest("ignored", &toml_path(&pi_root)),
    )
    .unwrap();
    std::fs::write(
        cwd.path().join("poison.inputs.toml"),
        "this is not valid toml",
    )
    .unwrap();
    std::fs::create_dir(cwd.path().join("backscroll.inputs.d")).unwrap();
    std::fs::write(cwd.path().join("backscroll.inputs.d/poison.toml"), "[").unwrap();

    let old_cwd = std::env::current_dir().unwrap();
    std::env::set_current_dir(cwd.path()).unwrap();
    let config = InputConfig::load();
    std::env::set_current_dir(old_cwd).unwrap();
    let config = config?;

    let active = config.active_inputs();
    assert_eq!(active.len(), 2);
    assert_eq!(active[0].id, "claude");
    assert_eq!(active[0].discover.roots, vec![toml_path(&claude_root)]);
    assert_eq!(active[1].id, "pi");
    assert_eq!(active[1].discover.roots, vec![toml_path(&pi_root)]);
    Ok(())
}

#[test]
fn missing_global_inputs_dir_returns_empty_config() -> miette::Result<()> {
    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());

    let config = InputConfig::load()?;

    assert!(config.manifests.is_empty());
    assert!(config.inputs.is_empty());
    Ok(())
}

#[test]
fn resolves_relative_roots_against_manifest_location() -> miette::Result<()> {
    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    let root = input_dir(config_dir.path()).join("sessions");
    std::fs::create_dir_all(&root).unwrap();
    write_input_manifest(
        config_dir.path(),
        "claude.inputs.toml",
        minimal_manifest("claude", "sessions"),
    );

    let config = InputConfig::load()?;

    assert_eq!(
        config.active_inputs()[0].discover.roots,
        vec![toml_path(&root)]
    );
    Ok(())
}

#[test]
fn allows_missing_active_discover_root_at_load_time() -> miette::Result<()> {
    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    let missing_root = input_dir(config_dir.path()).join("missing");
    write_input_manifest(
        config_dir.path(),
        "missing-root.inputs.toml",
        minimal_manifest("claude", "missing"),
    );

    let config = InputConfig::load()?;

    assert_eq!(config.active_inputs().len(), 1);
    assert_eq!(
        config.active_inputs()[0].discover.roots,
        vec![toml_path(&missing_root)]
    );
    Ok(())
}

#[test]
fn input_sync_skips_missing_only_discovery_roots() -> miette::Result<()> {
    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    write_input_manifest(
        config_dir.path(),
        "missing-only.inputs.toml",
        minimal_manifest("claude", "missing"),
    );

    let config = InputConfig::load()?;
    let files = parse_input_definitions(&config.active_inputs(), &HashMap::new());

    assert!(files.is_empty());
    Ok(())
}

#[test]
fn input_sync_skips_missing_roots_and_indexes_existing_roots() -> miette::Result<()> {
    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    let existing_root = config_dir.path().join("sessions");
    std::fs::create_dir_all(&existing_root).unwrap();
    std::fs::write(
        existing_root.join("session.jsonl"),
        r#"{"type":"user","message":{"role":"user","content":"mixed roots survive"},"uuid":"mr1","timestamp":"100"}"#,
    )
    .unwrap();
    let missing_root = input_dir(config_dir.path()).join("missing");
    let missing_root = toml_path(&missing_root);
    let existing_root = toml_path(&existing_root);
    write_input_manifest(
        config_dir.path(),
        "mixed-roots.inputs.toml",
        minimal_manifest_with_roots("claude", &[&missing_root, &existing_root]),
    );

    let config = InputConfig::load()?;
    let files = parse_input_definitions(&config.active_inputs(), &HashMap::new());

    assert_eq!(files.len(), 1);
    assert_eq!(files[0].messages.len(), 1);
    assert_eq!(files[0].messages[0].text, "mixed roots survive");
    Ok(())
}

#[test]
fn rejects_invalid_active_jsonpath_selector_with_clear_error() {
    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    let root = config_dir.path().join("sessions");
    std::fs::create_dir_all(&root).unwrap();
    write_input_manifest(
        config_dir.path(),
        "invalid-selector.inputs.toml",
        minimal_manifest("claude", &toml_path(&root))
            .replace("selector = \"$.message.content\"", "selector = \"$[\""),
    );

    let err = InputConfig::load().expect_err("invalid selector must fail");
    let msg = err.to_string();
    assert!(msg.contains("invalid-selector.inputs.toml"), "{msg}");
    assert!(msg.contains("content.selector"), "{msg}");
    assert!(msg.contains("$["), "{msg}");
}

#[test]
fn rejects_invalid_active_encoding_even_when_discover_root_is_missing() {
    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    write_input_manifest(
        config_dir.path(),
        "invalid-encoding.inputs.toml",
        minimal_manifest("claude", "missing").replace(
            "format = \"jsonl\"",
            "format = \"jsonl\"\nencoding = \"latin-1\"",
        ),
    );

    let err = InputConfig::load().expect_err("invalid encoding must fail");
    let msg = err.to_string();
    assert!(msg.contains("invalid-encoding.inputs.toml"), "{msg}");
    assert!(msg.contains("decode.encoding"), "{msg}");
    assert!(msg.contains("latin-1"), "{msg}");
}

#[test]
fn rejects_invalid_active_discover_glob_with_clear_error() {
    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    let root = config_dir.path().join("sessions");
    std::fs::create_dir_all(&root).unwrap();
    write_input_manifest(
        config_dir.path(),
        "invalid-glob.inputs.toml",
        minimal_manifest("claude", &toml_path(&root))
            .replace("include = [\"**/*.jsonl\"]", "include = [\"[\"]"),
    );

    let err = InputConfig::load().expect_err("invalid glob must fail");
    let msg = err.to_string();
    assert!(msg.contains("invalid-glob.inputs.toml"), "{msg}");
    assert!(msg.contains("discover.include"), "{msg}");
    assert!(msg.contains("["), "{msg}");
}

#[test]
fn rejects_invalid_active_manifest_with_clear_path_error() {
    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    write_input_manifest(
        config_dir.path(),
        "broken.inputs.toml",
        r#"version = 1

[[inputs]]
id = "claude"
source = "session"
active = true
"#
        .to_string(),
    );

    let err = InputConfig::load().expect_err("invalid active manifest must fail");
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

    let config_dir = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    let manifest = manifest.replace(
        "roots = [\"projects\"]",
        &format!("roots = [\"{}\"]", toml_path(&fixture_dir.join("projects"))),
    );
    write_input_manifest(config_dir.path(), "claude.inputs.toml", manifest);

    let input_config = InputConfig::load()?;
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
    let config_dir = tempdir().unwrap();
    let cwd = tempdir().unwrap();
    let _env = ConfigDirEnv::set(config_dir.path());
    std::fs::write(
        cwd.path().join("backscroll.toml"),
        r#"database_path = "legacy.db"
session_dirs = ["/legacy"]

[[session_inputs]]
source = "session"
parser = "claude"
paths = ["/legacy-input"]
"#,
    )
    .unwrap();

    let old_cwd = std::env::current_dir().unwrap();
    std::env::set_current_dir(cwd.path()).unwrap();
    let config = InputConfig::load();
    std::env::set_current_dir(old_cwd).unwrap();
    let config = config?;

    assert!(config.inputs.is_empty());
    Ok(())
}
