mod config;
mod core;
mod errors;
mod storage;

use crate::core::sync::sync_sessions;
use clap::{Parser, Subcommand};
use config::Config;
use miette::Result;
use storage::sqlite::Database;

#[derive(Parser)]
#[command(name = "backscroll")]
#[command(version, about = "Tier 2 search for Claude Code sessions", long_about = None)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Sincronizar sesiones de Claude Code
    Sync {
        /// Directorio de entrada de las sesiones
        #[arg(short, long)]
        path: Option<String>,
    },
    /// Buscar en el historial de sesiones
    Search {
        /// Consulta de búsqueda
        query: String,
        /// Filtrar por proyecto
        #[arg(short, long)]
        project: Option<String>,
    },
    /// Mostrar estado del índice
    Status,
}

fn main() -> Result<()> {
    let config = Config::load().unwrap_or_else(|_| Config::default_with_paths());

    let db = Database::open(&config.database_path)?;
    db.setup_schema()?;

    let cli = Cli::parse();

    match &cli.command {
        Commands::Sync { path } => {
            let session_path = path.as_deref().unwrap_or(&config.session_dir);
            println!("Sincronizando sesiones desde: {}", session_path);
            sync_sessions(&db, session_path)?;
        }
        Commands::Search { query, project } => {
            println!("Buscando: '{}'...", query);
            let results = db.search(query, project.as_deref())?;
            if results.is_empty() {
                println!("No se encontraron resultados.");
            } else {
                for res in results {
                    println!("---");
                    println!("Archivo: {}", res.path);
                    println!("Contenido: {}", res.content);
                }
            }
        }
        Commands::Status => {
            println!("Base de datos: {}", config.database_path);
            println!("Directorio de sesiones: {}", config.session_dir);
            println!("Estado del índice: OK");
        }
    }

    Ok(())
}
