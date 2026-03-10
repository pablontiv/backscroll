#![forbid(unsafe_code)]

mod config;
mod core;
mod output;
mod storage;

use crate::core::SearchEngine;
use crate::core::plans::parse_plan;
use crate::core::sync::parse_sessions;
use crate::output::{OutputFormat, OutputOptions, format_results};
use clap::{Parser, Subcommand};
use config::Config;
use miette::Result;
use std::path::PathBuf;
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
        /// No indexar archivos de plan (~/.claude/plans/)
        #[arg(long, default_value_t = false)]
        no_plans: bool,
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
        /// Filtrar por fuente: sessions, plans, o all
        #[arg(long, default_value = "all")]
        source: String,
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
        /// Buscar en todos los proyectos
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Formato compacto tab-separated
        #[arg(long, default_value_t = false)]
        robot: bool,
        /// Filtrar por fuente: sessions, plans, o all
        #[arg(long, default_value = "all")]
        source: String,
    },
    /// Listar sesiones indexadas con metadata
    List {
        /// Filtrar por proyecto
        #[arg(short, long)]
        project: Option<String>,
        /// Listar sesiones de todos los proyectos
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Número de sesiones recientes a mostrar
        #[arg(short, long, default_value_t = 20)]
        recent: usize,
        /// Formato JSON lines
        #[arg(long, default_value_t = false)]
        json: bool,
        /// Formato compacto tab-separated
        #[arg(long, default_value_t = false)]
        robot: bool,
    },
    /// Mostrar temas frecuentes del corpus indexado
    Topics {
        /// Filtrar por proyecto
        #[arg(short, long)]
        project: Option<String>,
        /// Mostrar temas de todos los proyectos (ignorar filtro)
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Número máximo de temas a mostrar
        #[arg(short, long, default_value_t = 30)]
        limit: usize,
        /// Formato JSON lines
        #[arg(long, default_value_t = false)]
        json: bool,
        /// Formato compacto tab-separated
        #[arg(long, default_value_t = false)]
        robot: bool,
    },
    /// Mostrar estado del índice
    Status,
}

fn discover_plan_files() -> Vec<PathBuf> {
    let home = std::env::var("HOME").unwrap_or_else(|_| ".".into());
    let plans_dir = PathBuf::from(&home).join(".claude/plans");
    discover_plan_files_from(&plans_dir)
}

fn discover_plan_files_from(plans_dir: &std::path::Path) -> Vec<PathBuf> {
    if !plans_dir.is_dir() {
        return Vec::new();
    }
    walkdir::WalkDir::new(plans_dir)
        .into_iter()
        .filter_map(|e| e.ok())
        .filter(|e| {
            e.file_type().is_file()
                && e.path()
                    .extension()
                    .is_some_and(|ext| ext == "md" || ext == "markdown")
        })
        .map(|e| e.into_path())
        .collect()
}

fn sync_plans(
    engine: &dyn SearchEngine,
    hashes: &std::collections::HashMap<String, String>,
) -> Result<()> {
    let plan_files = discover_plan_files();
    if plan_files.is_empty() {
        return Ok(());
    }

    let mut parsed = Vec::new();
    for path in &plan_files {
        let path_str = path.to_string_lossy();
        let existing_hash = hashes.get(path_str.as_ref());

        // Compute hash to check if changed
        let content =
            std::fs::read(path).map_err(|e| miette::miette!("Failed to read plan: {}", e))?;
        let hash = format!("{:x}", <sha2::Sha256 as sha2::Digest>::digest(&content));

        if existing_hash.is_some_and(|h| h == &hash) {
            continue;
        }

        match parse_plan(path) {
            Ok(file) => parsed.push(file),
            Err(e) => tracing::warn!("Failed to parse plan {}: {}", path_str, e),
        }
    }

    if !parsed.is_empty() {
        println!(
            "Sincronizando {} plans desde ~/.claude/plans/",
            parsed.len()
        );
        engine.sync_files(parsed)?;
    }

    Ok(())
}

fn create_engine(config: &Config) -> Result<Box<dyn SearchEngine>> {
    let db = Database::open(&config.database_path)?;
    db.setup_schema()?;
    Ok(Box::new(db))
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
            no_plans,
        } => {
            let paths = resolve_session_paths(path, &config)?;
            let engine = create_engine(&config)?;
            for p in &paths {
                println!("Sincronizando sesiones desde: {}", p);
                let hashes = engine.get_file_hashes()?;
                let files = parse_sessions(p, &hashes, *include_agents)?;
                engine.sync_files(files)?;
            }
            if !no_plans {
                let hashes = engine.get_file_hashes()?;
                sync_plans(engine.as_ref(), &hashes)?;
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
            source,
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
            // Auto-sync plans
            if let Ok(hashes) = engine.get_file_hashes() {
                let _ = sync_plans(engine.as_ref(), &hashes);
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

            let source_filter = if source == "all" {
                None
            } else {
                Some(source.clone())
            };
            let results = engine.search(query, &effective_project, &source_filter)?;
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
            all_projects,
            robot,
            source,
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
            if let Ok(hashes) = engine.get_file_hashes() {
                let _ = sync_plans(engine.as_ref(), &hashes);
            }

            let effective_project = if *all_projects {
                None
            } else {
                project.clone().or_else(|| {
                    std::env::current_dir()
                        .ok()
                        .map(|p| p.to_string_lossy().replace('/', "-"))
                })
            };

            let source_filter = if source == "all" {
                None
            } else {
                Some(source.clone())
            };
            let results = engine.search(query, &effective_project, &source_filter)?;
            if let Some(result) = results.first() {
                let session_id = engine
                    .get_session_id(&result.source_path)?
                    .unwrap_or_else(|| result.source_path.clone());
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
        Commands::List {
            project,
            all_projects,
            recent,
            json,
            robot,
        } => {
            let engine = create_engine(&config)?;

            // Auto-sync before list
            if let Ok(paths) = resolve_session_paths(&[], &config) {
                for p in &paths {
                    let hashes = engine.get_file_hashes()?;
                    let files = parse_sessions(p, &hashes, false)?;
                    if !files.is_empty() {
                        engine.sync_files(files)?;
                    }
                }
            }

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

            let sessions = engine.list_sessions(effective_project.as_deref(), *recent)?;

            if sessions.is_empty() {
                if !json && !robot {
                    println!("No se encontraron sesiones.");
                }
            } else if *json {
                for s in &sessions {
                    println!("{}", serde_json::to_string(s).unwrap_or_default());
                }
            } else if *robot {
                for s in &sessions {
                    println!(
                        "{}\t{}\t{}\t{}\t{}",
                        s.source_path,
                        s.project.as_deref().unwrap_or("-"),
                        s.messages,
                        s.started.as_deref().unwrap_or("-"),
                        s.ended.as_deref().unwrap_or("-"),
                    );
                }
            } else {
                println!(
                    "{:<60} {:<20} {:>5} {:<20}",
                    "PATH", "PROJECT", "MSGS", "LAST ACTIVITY"
                );
                println!("{}", "-".repeat(107));
                for s in &sessions {
                    let proj = s.project.as_deref().unwrap_or("-");
                    let proj_short = if proj.len() > 18 {
                        &proj[proj.len() - 18..]
                    } else {
                        proj
                    };
                    let path_short = if s.source_path.len() > 58 {
                        &s.source_path[s.source_path.len() - 58..]
                    } else {
                        &s.source_path
                    };
                    println!(
                        "{:<60} {:<20} {:>5} {:<20}",
                        path_short,
                        proj_short,
                        s.messages,
                        s.ended.as_deref().unwrap_or("-"),
                    );
                }
            }
        }
        Commands::Topics {
            project,
            all_projects,
            limit,
            json,
            robot,
        } => {
            let engine = create_engine(&config)?;

            // Auto-sync before topics
            if let Ok(paths) = resolve_session_paths(&[], &config) {
                for p in &paths {
                    let hashes = engine.get_file_hashes()?;
                    let files = parse_sessions(p, &hashes, false)?;
                    if !files.is_empty() {
                        engine.sync_files(files)?;
                    }
                }
            }

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

            let topics = engine.get_topics(effective_project.as_deref(), *limit)?;

            if topics.is_empty() {
                if !json && !robot {
                    println!("No se encontraron temas.");
                }
            } else if *json {
                for topic in &topics {
                    println!("{}", serde_json::to_string(topic).unwrap_or_default());
                }
            } else if *robot {
                for topic in &topics {
                    println!("{}\t{}\t{}", topic.term, topic.sessions, topic.mentions);
                }
            } else {
                println!("{:<30} {:>10} {:>10}", "TERM", "SESSIONS", "MENTIONS");
                println!("{}", "-".repeat(52));
                for topic in &topics {
                    println!(
                        "{:<30} {:>10} {:>10}",
                        topic.term, topic.sessions, topic.mentions
                    );
                }
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

                if let Ok(breakdown) = engine.get_project_breakdown() {
                    if !breakdown.is_empty() {
                        println!("\nBy Project:");
                        println!("  {:<40} {:>10} {:>10}", "PROJECT", "SESSIONS", "MESSAGES");
                        println!("  {}", "-".repeat(62));
                        for entry in &breakdown {
                            let proj = entry.project.as_deref().unwrap_or("(none)");
                            let proj_short = if proj.len() > 38 {
                                &proj[proj.len() - 38..]
                            } else {
                                proj
                            };
                            println!(
                                "  {:<40} {:>10} {:>10}",
                                proj_short, entry.sessions, entry.messages
                            );
                        }
                    }
                }
            } else {
                println!("Estado del índice: OK (no se pudo acceder a las métricas)");
            }
        }
    }

    Ok(())
}
