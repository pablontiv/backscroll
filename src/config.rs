use figment::{
    Figment,
    providers::{Env, Format, Toml},
};
use serde::{Deserialize, Deserializer, Serialize};
use std::fs;
use std::path::PathBuf;

#[derive(Deserialize)]
#[serde(untagged)]
enum StringOrVec {
    String(String),
    Vec(Vec<String>),
}

fn string_or_vec<'de, D>(deserializer: D) -> std::result::Result<Vec<String>, D::Error>
where
    D: Deserializer<'de>,
{
    match StringOrVec::deserialize(deserializer)? {
        StringOrVec::String(s) => Ok(vec![s]),
        StringOrVec::Vec(v) => Ok(v),
    }
}

fn optional_string_or_vec<'de, D>(
    deserializer: D,
) -> std::result::Result<Option<Vec<String>>, D::Error>
where
    D: Deserializer<'de>,
{
    match Option::<StringOrVec>::deserialize(deserializer)? {
        Some(StringOrVec::String(s)) => Ok(Some(vec![s])),
        Some(StringOrVec::Vec(v)) => Ok(Some(v)),
        None => Ok(None),
    }
}

fn default_session_dirs() -> Vec<String> {
    vec![".".into()]
}

#[derive(Debug, Deserialize, Serialize, Clone)]
#[serde(default)]
pub struct EmbeddingConfig {
    pub model_name: String,
    pub model_path: String,
    pub similarity_threshold: f32,
    pub top_k: usize,
    pub rrf_k: usize,
}

impl Default for EmbeddingConfig {
    fn default() -> Self {
        Self {
            model_name: "all-MiniLM-L6-v2".to_string(),
            model_path: String::new(),
            similarity_threshold: 0.3,
            top_k: 50,
            rrf_k: 60,
        }
    }
}

#[derive(Debug, Default, Deserialize, Serialize, Clone)]
#[serde(default)]
pub struct SourcesConfig {
    #[serde(deserialize_with = "string_or_vec")]
    pub ke: Vec<String>,
    #[serde(deserialize_with = "string_or_vec")]
    pub decisions: Vec<String>,
    #[serde(deserialize_with = "string_or_vec")]
    pub memories: Vec<String>,
    #[serde(deserialize_with = "string_or_vec")]
    pub rules: Vec<String>,
    #[serde(deserialize_with = "string_or_vec")]
    pub specs: Vec<String>,
    #[serde(deserialize_with = "string_or_vec")]
    pub backlog: Vec<String>,
}

#[derive(Deserialize, Default)]
#[serde(default)]
struct ConfigPresence {
    #[serde(alias = "session_dir", deserialize_with = "optional_string_or_vec")]
    session_dirs: Option<Vec<String>>,
}

#[derive(Deserialize, Serialize, Debug)]
pub struct Config {
    pub database_path: String,
    #[serde(
        alias = "session_dir",
        deserialize_with = "string_or_vec",
        default = "default_session_dirs"
    )]
    pub session_dirs: Vec<String>,
    #[serde(default)]
    pub embedding: EmbeddingConfig,
    #[serde(default)]
    pub sources: SourcesConfig,
    #[serde(skip)]
    pub session_dirs_explicit: bool,
}

impl Config {
    pub fn load() -> miette::Result<Self> {
        // Buscamos en:
        // 1. backscroll.toml (directorio actual)
        // 2. ~/.config/backscroll/config.toml (estándar Unix)
        // 3. Variables de entorno BACKSCROLL_*

        let mut home_config = PathBuf::from(std::env::var("HOME").unwrap_or_else(|_| ".".into()));
        home_config.push(".config/backscroll/config.toml");

        let figment = Figment::new()
            .merge(Toml::file("backscroll.toml"))
            .merge(Toml::file(home_config))
            .merge(Env::prefixed("BACKSCROLL_"));
        let session_dirs_explicit = figment
            .extract::<ConfigPresence>()
            .is_ok_and(|presence| presence.session_dirs.is_some());

        let mut cfg: Self = figment.extract().map_err(|e| {
            miette::miette!(
                "Error al cargar configuración: {}. Crea un 'backscroll.toml' o configura BACKSCROLL_DATABASE_PATH.",
                e,
            )
        })?;

        cfg.session_dirs_explicit = session_dirs_explicit;

        Ok(cfg)
    }

    pub fn discover_session_dirs() -> Vec<PathBuf> {
        let home = std::env::var("HOME").unwrap_or_else(|_| ".".into());
        let projects_dir = PathBuf::from(&home).join(".claude/projects");
        Self::discover_session_dirs_from(&projects_dir)
    }

    pub fn discover_session_dirs_from(projects_dir: &std::path::Path) -> Vec<PathBuf> {
        if !projects_dir.is_dir() {
            tracing::info!(
                "No Claude projects directory found at {}",
                projects_dir.display()
            );
            return Vec::new();
        }

        let dirs: Vec<PathBuf> = fs::read_dir(projects_dir)
            .into_iter()
            .flatten()
            .filter_map(|entry| entry.ok())
            .map(|entry| entry.path())
            .filter(|path| path.is_dir())
            .collect();

        if dirs.is_empty() {
            tracing::info!(
                "No session directories found under {}",
                projects_dir.display()
            );
        } else {
            tracing::info!(
                "Discovered {} session directories: {:?}",
                dirs.len(),
                dirs.iter()
                    .map(|d| d.display().to_string())
                    .collect::<Vec<_>>()
            );
        }

        dirs
    }

    pub fn default_with_paths() -> Self {
        let mut db_path = PathBuf::from(std::env::var("HOME").unwrap_or_else(|_| ".".into()));
        db_path.push(".backscroll.db");

        Self {
            database_path: db_path.to_string_lossy().into(),
            session_dirs: vec![".".into()],
            embedding: EmbeddingConfig::default(),
            sources: SourcesConfig::default(),
            session_dirs_explicit: false,
        }
    }

    pub fn has_explicit_session_dirs(&self) -> bool {
        self.session_dirs_explicit || self.session_dirs != default_session_dirs()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::sync::{Mutex, MutexGuard};
    use tempfile::tempdir;

    static FS_LOCK: Mutex<()> = Mutex::new(());

    fn lock_fs() -> MutexGuard<'static, ()> {
        FS_LOCK.lock().unwrap_or_else(|e| e.into_inner())
    }

    #[test]
    fn test_config_load_error_if_missing() {
        let _guard = lock_fs();
        let config = Config::load();
        assert!(config.is_err());
    }

    #[test]
    fn test_config_with_file() -> miette::Result<()> {
        let _guard = lock_fs();
        let toml_content = r#"
            database_path = "test.db"
            session_dir = "/tmp"
        "#;
        fs::write("backscroll.toml", toml_content).unwrap();

        let config = Config::load()?;
        assert_eq!(config.database_path, "test.db");
        assert_eq!(config.session_dirs, vec!["/tmp"]);

        fs::remove_file("backscroll.toml").unwrap();
        Ok(())
    }

    #[test]
    fn test_app_config_does_not_load_input_manifest_files() -> miette::Result<()> {
        let _guard = lock_fs();
        let dir = tempdir().unwrap();
        let old_dir = std::env::current_dir().unwrap();
        fs::write(
            dir.path().join("backscroll.toml"),
            "database_path = \"test.db\"\n",
        )
        .unwrap();
        fs::write(
            dir.path().join("claude.inputs.toml"),
            r#"version = 1

[[inputs]]
id = "claude"
source = "session"
active = true

[inputs.discover]
roots = ["/a"]
include = ["**/*.jsonl"]

[inputs.decode]
format = "jsonl"

[inputs.map]
role = "$.message.role"

[inputs.content]
selector = "$.message.content"
"#,
        )
        .unwrap();

        std::env::set_current_dir(dir.path()).unwrap();
        let config = Config::load();
        std::env::set_current_dir(old_dir).unwrap();
        let config = config?;

        assert_eq!(config.database_path, "test.db");
        Ok(())
    }

    #[test]
    fn test_app_config_ignores_invalid_input_manifest_files() -> miette::Result<()> {
        let _guard = lock_fs();
        let dir = tempdir().unwrap();
        let old_dir = std::env::current_dir().unwrap();
        fs::write(
            dir.path().join("backscroll.toml"),
            "database_path = \"test.db\"\n",
        )
        .unwrap();
        fs::write(dir.path().join("broken.inputs.toml"), "[[inputs]\n").unwrap();

        std::env::set_current_dir(dir.path()).unwrap();
        let config = Config::load();
        std::env::set_current_dir(old_dir).unwrap();

        assert_eq!(config?.database_path, "test.db");
        Ok(())
    }

    #[test]
    fn test_config_session_dirs_array() {
        let toml_content = r#"
            database_path = "test.db"
            session_dirs = ["/a", "/b"]
        "#;
        let config: Config = Figment::new()
            .merge(Toml::string(toml_content))
            .extract()
            .unwrap();
        assert_eq!(config.session_dirs, vec!["/a", "/b"]);
    }

    #[test]
    fn test_config_session_dir_legacy_string() {
        let toml_content = r#"
            database_path = "test.db"
            session_dir = "/legacy"
        "#;
        let config: Config = Figment::new()
            .merge(Toml::string(toml_content))
            .extract()
            .unwrap();
        assert_eq!(config.session_dirs, vec!["/legacy"]);
    }

    #[test]
    fn test_config_default_with_paths() {
        let config = Config::default_with_paths();
        assert_eq!(config.session_dirs, vec!["."]);
        assert!(config.database_path.ends_with(".backscroll.db"));
    }

    #[test]
    fn test_config_session_dirs_default_when_omitted() {
        let toml_content = r#"
            database_path = "test.db"
        "#;
        let config: Config = Figment::new()
            .merge(Toml::string(toml_content))
            .extract()
            .unwrap();
        assert_eq!(config.session_dirs, vec!["."]);
    }

    #[test]
    fn test_discover_finds_project_dirs() {
        let root = tempdir().unwrap();
        let projects = root.path().join(".claude/projects");
        fs::create_dir_all(projects.join("project-a")).unwrap();
        fs::create_dir_all(projects.join("project-b")).unwrap();
        // Create a file (should be ignored, only dirs)
        fs::write(projects.join("not-a-dir.txt"), "").unwrap();

        let dirs = Config::discover_session_dirs_from(&projects);
        assert_eq!(dirs.len(), 2);
        assert!(dirs.iter().any(|d| d.ends_with("project-a")));
        assert!(dirs.iter().any(|d| d.ends_with("project-b")));
    }

    #[test]
    fn test_discover_empty_when_no_projects() {
        let root = tempdir().unwrap();
        let projects = root.path().join(".claude/projects");
        fs::create_dir_all(&projects).unwrap();

        let dirs = Config::discover_session_dirs_from(&projects);
        assert!(dirs.is_empty());
    }

    #[test]
    fn test_discover_empty_when_dir_missing() {
        let root = tempdir().unwrap();
        let nonexistent = root.path().join("nope");

        let dirs = Config::discover_session_dirs_from(&nonexistent);
        assert!(dirs.is_empty());
    }

    #[test]
    fn test_embedding_config_defaults() {
        let config = EmbeddingConfig::default();
        assert_eq!(config.model_name, "all-MiniLM-L6-v2");
        assert_eq!(config.model_path, "");
        assert!((config.similarity_threshold - 0.3).abs() < f32::EPSILON);
        assert_eq!(config.top_k, 50);
        assert_eq!(config.rrf_k, 60);
    }

    #[test]
    fn test_sources_config_defaults() {
        let config = SourcesConfig::default();
        assert!(config.ke.is_empty());
        assert!(config.decisions.is_empty());
        assert!(config.memories.is_empty());
        assert!(config.rules.is_empty());
        assert!(config.specs.is_empty());
        assert!(config.backlog.is_empty());
    }
}
