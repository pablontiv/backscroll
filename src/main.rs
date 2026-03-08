mod config;
mod core;
mod errors;
mod output;
mod storage;

use crate::core::SearchEngine;
use crate::core::sync::parse_sessions;
use crate::output::{OutputFormat, OutputOptions, format_results};
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
        /// Incluir sesiones de subagentes (ignora la exclusión por defecto de /subagents/)
        #[arg(long, default_value_t = false)]
        include_agents: bool,
    },
    /// Buscar en el historial de sesiones
    Search {
        /// Consulta de búsqueda
        query: String,
        /// Filtrar por proyecto
        #[arg(short, long)]
        project: Option<String>,
        /// Formato de salida JSON lines
        #[arg(long, default_value_t = false)]
        json: bool,
        /// Formato de salida compacto tab-separated
        #[arg(long, default_value_t = false)]
        robot: bool,
        /// Campos a mostrar: minimal o full
        #[arg(long, default_value = "minimal")]
        fields: String,
        /// Máximo de tokens aproximados a mostrar
        #[arg(long)]
        max_tokens: Option<usize>,
    },
    /// Leer una sesión individual filtrada
    Read {
        /// Ruta al archivo JSONL de la sesión
        path: std::path::PathBuf,
    },
    /// Mostrar estado del índice
    Status,
}

fn create_engine(config: &Config) -> Result<Box<dyn SearchEngine>> {
    let db = Database::open(&config.database_path)?;
    db.setup_schema()?;
    Ok(Box::new(db))
}

use tracing_subscriber::EnvFilter;

fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env())
        .init();

    let cli = Cli::parse();

    // Cargar configuración (no requiere DB)
    let config = Config::load().unwrap_or_else(|_| Config::default_with_paths());

    match &cli.command {
        Commands::Sync {
            path,
            include_agents,
        } => {
            let session_path = path.as_deref().unwrap_or(&config.session_dir);
            println!("Sincronizando sesiones desde: {}", session_path);

            let engine = create_engine(&config)?;
            let hashes = engine.get_file_hashes()?;
            let files = parse_sessions(session_path, &hashes, *include_agents)?;
            engine.sync_files(files)?;
        }
        Commands::Search {
            query,
            project,
            json,
            robot,
            fields,
            max_tokens,
        } => {
            if !json && !robot {
                println!("Buscando: '{}'...", query);
            }

            let engine = create_engine(&config)?;
            let results = engine.search(query, project)?;
            if results.is_empty() && !json && !robot {
                println!("No se encontraron resultados.");
            } else {
                let format = if *json {
                    OutputFormat::Json
                } else if *robot {
                    OutputFormat::Robot
                } else {
                    OutputFormat::Text
                };

                let options = OutputOptions {
                    format,
                    fields: fields.clone(),
                    max_tokens: *max_tokens,
                };

                format_results(&results, &options);
            }
        }
        Commands::Read { path } => {
            let messages = crate::core::reader::read_session(path)?;
            for msg in messages {
                println!("[{}]", msg.role);
                println!("{}", msg.text);
                println!();
            }
        }
        Commands::Status => {
            println!("Backscroll v{}", env!("CARGO_PKG_VERSION"));
            println!("Base de datos: {}", config.database_path);
            println!("Directorio de sesiones: {}", config.session_dir);

            if let Ok(engine) = create_engine(&config) {
                if let Ok(stats) = engine.get_stats() {
                    println!("\nBackscroll Index Status");
                    println!("  Files indexed:  {}", stats.file_count);
                    println!("  Messages:       {}", stats.message_count);
                    println!("  Projects:       {}", stats.project_count);

                    let size_mb = stats.db_size_bytes as f64 / 1_048_576.0;
                    println!("  Database size:  {:.2} MB", size_mb);

                    println!(
                        "  Last sync:      {}",
                        stats.last_sync.unwrap_or_else(|| "N/A".to_string())
                    );
                }
            } else {
                println!("Estado del índice: OK (no se pudo acceder a las métricas)");
            }
        }
    }

    Ok(())
}
