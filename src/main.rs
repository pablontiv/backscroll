#![forbid(unsafe_code)]

mod config;
mod core;
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
        /// Directorios de entrada de las sesiones (repetir para múltiples)
        #[arg(short, long)]
        path: Vec<String>,
        /// Incluir sesiones de subagentes (ignora la exclusión por defecto de /subagents/)
        #[arg(long, default_value_t = false)]
        include_agents: bool,
    },
    /// Buscar en el historial de sesiones
    Search {
        /// Consulta de búsqueda
        query: String,
        /// Filtrar por proyecto (por defecto: derivado del directorio actual)
        #[arg(short, long)]
        project: Option<String>,
        /// Buscar en todos los proyectos (ignorar filtro por proyecto)
        #[arg(long, default_value_t = false)]
        all_projects: bool,
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
    /// Buscar y retornar la sesión más reciente para --resume
    Resume {
        /// Consulta de búsqueda
        query: String,
        /// Filtrar por proyecto
        #[arg(short, long)]
        project: Option<String>,
        /// Formato compacto tab-separated
        #[arg(long, default_value_t = false)]
        robot: bool,
    },
    /// Mostrar estado del índice
    Status,
}

fn create_engine(config: &Config) -> Result<Box<dyn SearchEngine>> {
    let db = Database::open(&config.database_path)?;
    db.setup_schema()?;
    Ok(Box::new(db))
}

fn extract_session_id(source_path: &str) -> String {
    // Extract UUID-like session ID from path like:
    // ~/.claude/projects/-opt-foo/04df2262-a48e-4549-97a9-11bcf4bb0257/session.jsonl
    std::path::Path::new(source_path)
        .components()
        .filter_map(|c| c.as_os_str().to_str())
        .find(|s| s.len() >= 32 && s.contains('-'))
        .unwrap_or(source_path)
        .to_string()
}

fn resolve_session_paths(cli_paths: &[String], config: &Config) -> Result<Vec<String>> {
    // 1. CLI --path overrides everything
    if !cli_paths.is_empty() {
        return Ok(cli_paths.to_vec());
    }

    // 2. Non-default config takes precedence
    if config.session_dirs != vec!["."] {
        return Ok(config.session_dirs.clone());
    }

    // 3. Auto-discovery fallback
    let discovered = Config::discover_session_dirs();
    if !discovered.is_empty() {
        return Ok(discovered
            .into_iter()
            .map(|p| p.to_string_lossy().into_owned())
            .collect());
    }

    // 4. No paths found
    Err(miette::miette!(
        "No session directories found. Use --path, configure session_dirs in backscroll.toml, or ensure ~/.claude/projects/ exists."
    ))
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
            let paths = resolve_session_paths(path, &config)?;
            let engine = create_engine(&config)?;
            for p in &paths {
                println!("Sincronizando sesiones desde: {}", p);
                let hashes = engine.get_file_hashes()?;
                let files = parse_sessions(p, &hashes, *include_agents)?;
                engine.sync_files(files)?;
            }
        }
        Commands::Search {
            query,
            project,
            all_projects,
            json,
            robot,
            fields,
            max_tokens,
        } => {
            let engine = create_engine(&config)?;

            // Auto-sync: indexar sesiones nuevas antes de buscar (incremental, rápido)
            if let Ok(paths) = resolve_session_paths(&[], &config) {
                for p in &paths {
                    let hashes = engine.get_file_hashes()?;
                    let files = parse_sessions(p, &hashes, false)?;
                    if !files.is_empty() {
                        engine.sync_files(files)?;
                    }
                }
            }

            // Proyecto: --all-projects → None, --project → explícito, default → CWD
            let effective_project = if *all_projects {
                None
            } else {
                match project {
                    Some(p) => Some(p.clone()),
                    None => std::env::current_dir()
                        .ok()
                        .map(|p| p.to_string_lossy().replace('/', "-")),
                }
            };

            if !json && !robot {
                println!("Buscando: '{}'...", query);
            }

            let results = engine.search(query, &effective_project)?;
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
        Commands::Resume {
            query,
            project,
            robot,
        } => {
            let engine = create_engine(&config)?;

            // Auto-sync before resume search
            if let Ok(paths) = resolve_session_paths(&[], &config) {
                for p in &paths {
                    let hashes = engine.get_file_hashes()?;
                    let files = parse_sessions(p, &hashes, false)?;
                    if !files.is_empty() {
                        engine.sync_files(files)?;
                    }
                }
            }

            let effective_project = project.clone().or_else(|| {
                std::env::current_dir()
                    .ok()
                    .map(|p| p.to_string_lossy().replace('/', "-"))
            });

            let results = engine.search(query, &effective_project)?;
            if let Some(result) = results.first() {
                let session_id = extract_session_id(&result.source_path);
                if *robot {
                    println!("{}\t{}", session_id, result.source_path);
                } else {
                    println!("Session: {}", result.source_path);
                    println!("ID: {}", session_id);
                    if let Some(snippet) = &result.match_snippet {
                        println!("Match: {}", snippet);
                    }
                }
            } else {
                eprintln!("No matching session found.");
                std::process::exit(1);
            }
        }
        Commands::Status => {
            println!("Backscroll v{}", env!("CARGO_PKG_VERSION"));
            println!("Base de datos: {}", config.database_path);
            println!("Directorio de sesiones: {}", config.session_dirs.join(", "));

            if let Ok(engine) = create_engine(&config) {
                // Auto-sync antes de mostrar stats
                if let Ok(paths) = resolve_session_paths(&[], &config) {
                    for p in &paths {
                        if let Ok(hashes) = engine.get_file_hashes() {
                            if let Ok(files) = parse_sessions(p, &hashes, false) {
                                if !files.is_empty() {
                                    let _ = engine.sync_files(files);
                                }
                            }
                        }
                    }
                }

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
