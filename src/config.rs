use figment::{
    Figment,
    providers::{Env, Format, Toml},
};
use serde::{Deserialize, Deserializer, Serialize};
use std::fs;
use std::path::Path;
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

fn default_true() -> bool {
    true
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

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(default, deny_unknown_fields)]
pub struct SessionInput {
    pub source: String,
    #[serde(default = "SessionInput::default_parser")]
    pub parser: String,
    #[serde(default, deserialize_with = "string_or_vec")]
    pub paths: Vec<String>,
    #[serde(default)]
    pub glob: Option<String>,
    #[serde(default)]
    pub include_agents: bool,
    #[serde(default = "default_true")]
    pub active: bool,
}

impl SessionInput {
    fn default_parser() -> String {
        "claude".to_string()
    }

    pub fn is_active(&self) -> bool {
        self.active
    }

    pub fn parser(&self) -> &str {
        if self.parser.is_empty() {
            "claude"
        } else {
            self.parser.as_str()
        }
    }
}

impl Default for SessionInput {
    fn default() -> Self {
        Self {
            source: "session".to_string(),
            parser: Self::default_parser(),
            paths: Vec::new(),
            glob: None,
            include_agents: false,
            active: true,
        }
    }
}

#[derive(Debug, Serialize, Deserialize, Clone)]
#[serde(deny_unknown_fields)]
struct InputsFile {
    #[serde(default)]
    pub inputs: Vec<SessionInput>,
    #[serde(default)]
    pub session_inputs: Vec<SessionInput>,
}

impl InputsFile {
    fn all_inputs(&self) -> Vec<SessionInput> {
        if !self.inputs.is_empty() {
            self.inputs.clone()
        } else {
            self.session_inputs.clone()
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
    #[serde(default)]
    pub session_inputs: Vec<SessionInput>,
    #[serde(skip)]
    pub session_dirs_explicit: bool,
}

impl Config {
    fn read_inputs_file(path: &Path) -> miette::Result<Vec<SessionInput>> {
        let content = fs::read_to_string(path)
            .map_err(|err| miette::miette!("Failed to read {}: {}", path.display(), err))?;
        let file_inputs = Figment::new()
            .merge(Toml::string(&content))
            .extract::<InputsFile>()
            .map_err(|err| miette::miette!("Failed to parse {}: {}", path.display(), err))?;
        Ok(file_inputs.all_inputs())
    }

    fn collect_backscroll_inputs() -> miette::Result<Vec<SessionInput>> {
        let mut inputs: Vec<SessionInput> = Vec::new();

        let legacy_file = Path::new("backscroll.inputs.toml");
        if legacy_file.is_file() {
            inputs.extend(Self::read_inputs_file(legacy_file)?);
        }

        let dir = Path::new("backscroll.inputs.d");
        if dir.is_dir() {
            let mut entries: Vec<_> = fs::read_dir(dir)
                .map_err(|err| miette::miette!("Failed to read {}: {}", dir.display(), err))?
                .collect::<std::result::Result<Vec<_>, _>>()
                .map_err(|err| miette::miette!("Failed to read {}: {}", dir.display(), err))?;

            entries.retain(|entry| entry.path().extension().is_some_and(|ext| ext == "toml"));
            entries.sort_by_key(|entry| entry.path());
            for entry in entries {
                inputs.extend(Self::read_inputs_file(&entry.path())?);
            }
        }

        Ok(inputs)
    }

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
        cfg.session_inputs
            .extend(Self::collect_backscroll_inputs()?);

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
            session_inputs: Vec::new(),
            session_dirs_explicit: false,
        }
    }

    pub fn has_explicit_session_dirs(&self) -> bool {
        self.session_dirs_explicit || self.session_dirs != default_session_dirs()
    }

    pub fn active_session_inputs(&self) -> Vec<SessionInput> {
        self.session_inputs
            .iter()
            .filter(|i| i.is_active())
            .filter(|i| i.source == "session")
            .cloned()
            .collect()
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
    fn test_config_session_inputs_from_file() {
        let _guard = lock_fs();
        let dir = tempdir().unwrap();
        let old_dir = std::env::current_dir().unwrap();
        fs::write(
            dir.path().join("backscroll.toml"),
            "database_path = \"test.db\"\n",
        )
        .unwrap();

        let inputs_toml = r#"
        [[session_inputs]]
        source = "session"
        parser = "claude"
        paths = ["/a", "/b"]
        glob = "**/*.jsonl"
        include_agents = true
        active = true

        [[session_inputs]]
        source = "session"
        parser = "pi"
        paths = "/tmp/pi.jsonl"
        active = false
        "#;
        fs::write(dir.path().join("backscroll.inputs.toml"), inputs_toml).unwrap();

        std::env::set_current_dir(dir.path()).unwrap();
        let config = Config::load().unwrap();
        std::env::set_current_dir(old_dir).unwrap();

        eprintln!("inputs={:?}", config.session_inputs);
        assert_eq!(config.session_inputs.len(), 2);
        assert!(config.session_inputs[0].is_active());
        assert_eq!(config.session_inputs[0].paths.len(), 2);
        assert_eq!(config.session_inputs[0].paths[0], "/a");
        assert_eq!(config.session_inputs[0].glob.as_deref(), Some("**/*.jsonl"));
        assert!(!config.session_inputs[1].is_active());
    }

    #[test]
    fn test_invalid_inputs_file_returns_controlled_parse_error() {
        let _guard = lock_fs();
        let dir = tempdir().unwrap();
        let old_dir = std::env::current_dir().unwrap();
        fs::write(
            dir.path().join("backscroll.toml"),
            "database_path = \"test.db\"\n",
        )
        .unwrap();
        fs::write(dir.path().join("backscroll.inputs.toml"), "[[inputs]\n").unwrap();

        std::env::set_current_dir(dir.path()).unwrap();
        let result = Config::load();
        std::env::set_current_dir(old_dir).unwrap();

        let err = result.expect_err("invalid input manifests must fail Config::load");
        let msg = err.to_string();
        assert!(msg.contains("backscroll.inputs.toml"), "{msg}");
    }

    #[test]
    fn test_inputs_reject_unknown_fields() {
        let _guard = lock_fs();
        let dir = tempdir().unwrap();
        let old_dir = std::env::current_dir().unwrap();
        fs::write(
            dir.path().join("backscroll.toml"),
            "database_path = \"test.db\"\n",
        )
        .unwrap();
        fs::write(
            dir.path().join("backscroll.inputs.toml"),
            "[[inputs]]\nsource = \"session\"\npaths = [\"/a\"]\nunknown_key = true\n",
        )
        .unwrap();

        std::env::set_current_dir(dir.path()).unwrap();
        let result = Config::load();
        std::env::set_current_dir(old_dir).unwrap();

        let err = result.expect_err("unknown input keys must be rejected");
        let msg = err.to_string();
        assert!(
            msg.contains("unknown_key") || msg.contains("unknown"),
            "{msg}"
        );
    }

    #[test]
    fn test_config_session_inputs_from_dir() -> miette::Result<()> {
        let dir = tempdir().unwrap();
        fs::write(
            dir.path().join("backscroll.toml"),
            "database_path = \"test.db\"\n",
        )
        .unwrap();
        fs::create_dir(dir.path().join("backscroll.inputs.d")).unwrap();

        let file1 = dir.path().join("backscroll.inputs.d/01-one.toml");
        let file2 = dir.path().join("backscroll.inputs.d/02-two.toml");

        fs::write(
            &file1,
            "[[inputs]]\nsource = \"session\"\nparser = \"claude\"\npaths = [\"/one\"]\n",
        )
        .unwrap();
        fs::write(
            &file2,
            "[[inputs]]\nsource = \"session\"\nparser = \"pi\"\npaths = [\"/two\"]\n",
        )
        .unwrap();

        let _guard = lock_fs();
        let original_dir = std::env::current_dir().unwrap();
        std::env::set_current_dir(dir.path()).unwrap();

        let config = Config::load().unwrap();
        assert_eq!(config.session_inputs.len(), 2);
        let active_paths: Vec<_> = config
            .active_session_inputs()
            .into_iter()
            .map(|input| input.paths[0].clone())
            .collect();
        assert_eq!(active_paths, vec!["/one", "/two"]);

        std::env::set_current_dir(original_dir).unwrap();
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
