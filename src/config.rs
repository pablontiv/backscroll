use figment::{
    Figment,
    providers::{Env, Format, Toml},
};
use serde::{Deserialize, Deserializer, Serialize};
use std::path::PathBuf;

fn string_or_vec<'de, D>(deserializer: D) -> std::result::Result<Vec<String>, D::Error>
where
    D: Deserializer<'de>,
{
    #[derive(Deserialize)]
    #[serde(untagged)]
    enum StringOrVec {
        String(String),
        Vec(Vec<String>),
    }

    match StringOrVec::deserialize(deserializer)? {
        StringOrVec::String(s) => Ok(vec![s]),
        StringOrVec::Vec(v) => Ok(v),
    }
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
}

fn default_session_dirs() -> Vec<String> {
    vec![".".into()]
}

impl Config {
    pub fn load() -> miette::Result<Self> {
        // Buscamos en:
        // 1. backscroll.toml (directorio actual)
        // 2. ~/.config/backscroll/config.toml (estándar Unix)
        // 3. Variables de entorno BACKSCROLL_*

        let mut home_config = PathBuf::from(std::env::var("HOME").unwrap_or_else(|_| ".".into()));
        home_config.push(".config/backscroll/config.toml");

        Figment::new()
            .merge(Toml::file("backscroll.toml"))
            .merge(Toml::file(home_config))
            .merge(Env::prefixed("BACKSCROLL_"))
            .extract()
            .map_err(|e| miette::miette!("Error al cargar configuración: {}. Crea un 'backscroll.toml' o configura BACKSCROLL_DATABASE_PATH.", e))
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

        let dirs: Vec<PathBuf> = std::fs::read_dir(projects_dir)
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
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;

    #[test]
    fn test_config_load_error_if_missing() {
        let config = Config::load();
        assert!(config.is_err());
    }

    #[test]
    fn test_config_with_file() -> miette::Result<()> {
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
}
