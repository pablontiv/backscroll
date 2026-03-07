use serde::{Deserialize, Serialize};
use figment::{Figment, providers::{Format, Toml, Env}};
use std::path::PathBuf;

#[derive(Deserialize, Serialize, Debug)]
pub struct Config {
    pub database_path: String,
    pub session_dir: String,
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

    pub fn default_with_paths() -> Self {
        let mut db_path = PathBuf::from(std::env::var("HOME").unwrap_or_else(|_| ".".into()));
        db_path.push(".backscroll.db");

        Self {
            database_path: db_path.to_string_lossy().into(),
            session_dir: ".".into(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

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
        assert_eq!(config.session_dir, "/tmp");

        fs::remove_file("backscroll.toml").unwrap();
        Ok(())
    }
}
