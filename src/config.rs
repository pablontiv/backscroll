use serde::Deserialize;
use figment::{Figment, providers::{Format, Toml, Env}};

#[derive(Deserialize, Debug)]
pub struct Config {
    pub database_path: String,
    pub session_dir: String,
}

impl Config {
    pub fn load() -> miette::Result<Self> {
        Figment::new()
            .merge(Toml::file("backscroll.toml"))
            .merge(Env::prefixed("BACKSCROLL_"))
            .extract()
            .map_err(|e| miette::miette!("Error al cargar configuración: {}", e))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn test_config_load_defaults() {
        // En ausencia de archivos y env vars, Figment fallará si no hay defaults
        // Pero podemos probar que el error es capturado
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
