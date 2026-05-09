#![deny(unsafe_code)]

mod output;

use crate::output::{OutputFormat, OutputOptions, format_results};
use backscroll::config::Config;
use backscroll::core::sync::{dry_run_input_definition, parse_input_definitions};
use backscroll::core::{ParsedMessage, SearchEngine, SearchParams};
use backscroll::input_config::InputConfig;
use backscroll::storage::sqlite::Database;
use clap::{Parser, Subcommand};
use miette::Result;
use serde::Serialize;
use std::path::PathBuf;

#[derive(Parser)]
#[command(name = "backscroll")]
#[command(version, about = "Tier 2 search for Claude Code sessions", long_about = None)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Sincronizar inputs declarados en manifests
    Sync {
        /// Deprecated compatibility flag; plans are indexed only from input manifests
        #[arg(long, default_value_t = false)]
        no_plans: bool,
        /// Optimizar el índice FTS5 después de sincronizar
        #[arg(long, default_value_t = false)]
        optimize: bool,
        /// Skip embedding generation during sync
        #[arg(long)]
        no_embeddings: bool,
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
        /// Filter by source type: session, plan, ke, decision, memory, rule, spec, backlog
        #[arg(long, default_value = "all")]
        source: String,
        /// Filter by indexed source path (exact path, SQL LIKE pattern, or * glob pattern)
        #[arg(long)]
        source_path: Option<String>,
        /// Solo resultados después de esta fecha (ISO 8601, ej: 2026-03-01)
        #[arg(long)]
        after: Option<String>,
        /// Solo resultados antes de esta fecha (ISO 8601, ej: 2026-03-09)
        #[arg(long)]
        before: Option<String>,
        /// Filtrar por rol exacto (human se conserva como alias de user)
        #[arg(long)]
        role: Option<String>,
        /// Máximo de resultados (default 20, 0 = sin límite)
        #[arg(long, default_value_t = 20)]
        limit: usize,
        /// Número de resultados a saltar
        #[arg(long, default_value_t = 0)]
        offset: usize,
        /// Filtrar por tipo de contenido exacto (por ejemplo: text, code, tool, rationale)
        #[arg(long)]
        content_type: Option<String>,
        /// Filtrar por tag de sesión (auto-asignado)
        #[arg(long)]
        tag: Option<String>,
        /// Use lexical search only (disable hybrid vector+BM25)
        #[arg(long)]
        lexical_only: bool,
        /// Minimum similarity threshold for vector results (0.0-1.0)
        #[arg(long, default_value = "0.3")]
        similarity_threshold: f32,
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
    /// Re-indexar todos los archivos declarados en manifests (fuerza re-procesamiento)
    Reindex {
        /// Deprecated compatibility flag; plans are indexed only from input manifests
        #[arg(long, default_value_t = false)]
        no_plans: bool,
        /// Skip embedding generation during sync
        #[arg(long)]
        no_embeddings: bool,
    },
    /// Eliminar datos antiguos del índice
    Purge {
        /// Eliminar datos anteriores a esta fecha (ISO 8601, ej: 2025-01-01)
        #[arg(long)]
        before: String,
    },
    /// Verificar integridad del índice
    Validate,
    /// Mostrar insights y analytics del corpus
    Insights {
        /// Filtrar por proyecto
        #[arg(short, long)]
        project: Option<String>,
        /// Insights de todos los proyectos
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Formato JSON lines
        #[arg(long, default_value_t = false)]
        json: bool,
        /// Formato compacto tab-separated
        #[arg(long, default_value_t = false)]
        robot: bool,
    },
    /// Exportar resultados de búsqueda a archivo
    Export {
        /// Consulta de búsqueda
        query: String,
        /// Formato de exportación: markdown o csv
        #[arg(long, default_value = "markdown")]
        format: String,
        /// Filtrar por proyecto
        #[arg(short, long)]
        project: Option<String>,
        /// Exportar de todos los proyectos
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Filtrar por fuente: sessions, plans, o all
        #[arg(long, default_value = "all")]
        source: String,
        /// Solo resultados después de esta fecha
        #[arg(long)]
        after: Option<String>,
        /// Solo resultados antes de esta fecha
        #[arg(long)]
        before: Option<String>,
        /// Filtrar por rol
        #[arg(long)]
        role: Option<String>,
        /// Máximo de resultados (default 20, 0 = sin límite)
        #[arg(long, default_value_t = 20)]
        limit: usize,
    },
    /// Tooling for generic input manifests
    Inputs {
        #[command(subcommand)]
        command: InputCommands,
    },
    /// Mostrar estado del índice
    Status,
}

#[derive(Subcommand)]
enum InputCommands {
    /// List discovered input manifests and inputs
    List {
        /// Emit machine-readable JSON
        #[arg(long, default_value_t = false)]
        json: bool,
    },
    /// Validate input manifests without syncing
    Validate {
        /// Emit machine-readable JSON
        #[arg(long, default_value_t = false)]
        json: bool,
    },
    /// Dry-run a manifest input against a sample file
    #[command(visible_alias = "dry-run")]
    Test {
        /// Input id from the manifest
        #[arg(long = "input")]
        input_id: String,
        /// Sample file to parse without writing SQLite
        #[arg(long)]
        file: PathBuf,
        /// Emit machine-readable JSON
        #[arg(long, default_value_t = false)]
        json: bool,
    },
}

fn create_engine(config: &Config) -> Result<Box<dyn SearchEngine>> {
    let db = Database::open(&config.database_path)?;
    db.setup_schema()?;
    Ok(Box::new(db))
}

fn sync_manifest_inputs(engine: &dyn SearchEngine, input_config: &InputConfig) -> Result<()> {
    let inputs = input_config.active_inputs();
    if inputs.is_empty() {
        tracing::debug!(
            resolution_source = "none",
            "No active O02 input manifests found; skipping input sync"
        );
        return Ok(());
    }

    let hashes = engine.get_file_hashes()?;
    let files = parse_input_definitions(&inputs, &hashes);
    if !files.is_empty() {
        engine.sync_files(files)?;
    }
    Ok(())
}

#[derive(Serialize)]
struct InputListEntry {
    manifest: String,
    id: String,
    source: String,
    active: bool,
    roots: Vec<String>,
    include: Vec<String>,
    exclude: Vec<String>,
    format: String,
}

#[derive(Serialize)]
struct InputListOutput {
    manifests: usize,
    inputs: Vec<InputListEntry>,
}

#[derive(Serialize)]
struct InputValidationOutput {
    valid: bool,
    manifest_count: usize,
    input_count: usize,
}

fn decode_format_name(format: &backscroll::input_config::DecodeFormat) -> &'static str {
    match format {
        backscroll::input_config::DecodeFormat::Jsonl => "jsonl",
        backscroll::input_config::DecodeFormat::Json => "json",
        backscroll::input_config::DecodeFormat::Markdown => "markdown",
        backscroll::input_config::DecodeFormat::MarkdownSections => "markdown_sections",
    }
}

fn list_input_entries(input_config: &InputConfig) -> Vec<InputListEntry> {
    input_config
        .manifests
        .iter()
        .flat_map(|loaded| {
            loaded.manifest.inputs.iter().map(|input| InputListEntry {
                manifest: loaded.path.to_string_lossy().into_owned(),
                id: input.id.clone(),
                source: input.source.clone(),
                active: input.active,
                roots: input.discover.roots.clone(),
                include: input.discover.include.clone(),
                exclude: input.discover.exclude.clone(),
                format: decode_format_name(&input.decode.format).to_string(),
            })
        })
        .collect()
}

fn print_input_list(input_config: &InputConfig, json: bool) -> Result<()> {
    let entries = list_input_entries(input_config);
    if json {
        let output = InputListOutput {
            manifests: input_config.manifests.len(),
            inputs: entries,
        };
        println!(
            "{}",
            serde_json::to_string(&output)
                .map_err(|err| miette::miette!("Failed to serialize input list: {}", err))?
        );
        return Ok(());
    }

    if entries.is_empty() {
        println!(
            "No input manifests found in <config_dir>/backscroll/inputs/*.inputs.toml (set BACKSCROLL_CONFIG_DIR to override)."
        );
        return Ok(());
    }

    for entry in entries {
        println!(
            "{}\t{}\t{}\tactive={}\tformat={}\troots={}",
            entry.manifest,
            entry.id,
            entry.source,
            entry.active,
            entry.format,
            entry.roots.join(",")
        );
    }
    Ok(())
}

fn print_input_validation(input_config: &InputConfig, json: bool) -> Result<()> {
    if json {
        let output = InputValidationOutput {
            valid: true,
            manifest_count: input_config.manifests.len(),
            input_count: input_config.inputs.len(),
        };
        println!(
            "{}",
            serde_json::to_string(&output)
                .map_err(|err| miette::miette!("Failed to serialize validation output: {}", err))?
        );
    } else {
        println!(
            "Input manifests valid ({} manifests, {} inputs).",
            input_config.manifests.len(),
            input_config.inputs.len()
        );
    }
    Ok(())
}

fn print_dry_run_messages(messages: &[ParsedMessage]) {
    for message in messages {
        println!(
            "{}\t{}\t{}\t{}",
            message.ordinal, message.role, message.content_type, message.text
        );
    }
}

fn handle_inputs_command(command: &InputCommands) -> Result<()> {
    match command {
        InputCommands::List { json } => {
            let input_config = InputConfig::load()?;
            print_input_list(&input_config, *json)
        }
        InputCommands::Validate { json } => match InputConfig::load() {
            Ok(input_config) => print_input_validation(&input_config, *json),
            Err(err) => {
                if *json {
                    println!(
                        "{}",
                        serde_json::json!({
                            "valid": false,
                            "error": err.to_string(),
                        })
                    );
                }
                Err(err)
            }
        },
        InputCommands::Test {
            input_id,
            file,
            json,
        } => {
            let input_config = InputConfig::load()?;
            let input = input_config
                .inputs
                .iter()
                .find(|candidate| candidate.id == *input_id)
                .ok_or_else(|| {
                    miette::miette!("No input manifest entry found with id '{}'", input_id)
                })?;
            let report = dry_run_input_definition(input, file)?;
            if *json {
                println!(
                    "{}",
                    serde_json::to_string(&report).map_err(|err| miette::miette!(
                        "Failed to serialize dry-run output: {}",
                        err
                    ))?
                );
            } else {
                println!(
                    "Input '{}' dry-run for {}: {} records read, {} emitted, {} records dropped, {} blocks dropped",
                    report.input_id,
                    report.file,
                    report.records_read,
                    report.records_emitted,
                    report.records_dropped,
                    report.blocks_dropped
                );
                for drop in &report.drop_reasons {
                    println!(
                        "drop\t{}\tordinal={:?}\tblock={:?}\t{}",
                        drop.scope, drop.ordinal, drop.block_index, drop.reason
                    );
                }
                print_dry_run_messages(&report.messages);
            }
            Ok(())
        }
    }
}

use tracing_subscriber::EnvFilter;

fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env())
        .init();

    let cli = Cli::parse();

    // Cargar configuración de aplicación e inputs por separado (no requiere DB)
    let config = Config::load().unwrap_or_else(|_| Config::default_with_paths());
    if let Commands::Inputs { command } = &cli.command {
        return handle_inputs_command(command);
    }
    let input_config = InputConfig::load()?;

    match &cli.command {
        Commands::Sync {
            no_plans: _,
            optimize,
            no_embeddings: _,
        } => {
            let engine = create_engine(&config)?;
            sync_manifest_inputs(engine.as_ref(), &input_config)?;
            if *optimize {
                println!("Optimizando índice FTS5...");
                engine.optimize_fts()?;
                println!("Optimización completa.");
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
            source_path,
            after,
            before,
            role,
            limit,
            offset,
            content_type,
            tag,
            lexical_only,
            similarity_threshold,
        } => {
            let engine = create_engine(&config)?;

            // Auto-sync: indexar inputs declarados antes de buscar (incremental, rápido)
            sync_manifest_inputs(engine.as_ref(), &input_config)?;

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
            let params = SearchParams {
                project: effective_project,
                source: source_filter,
                source_path: source_path.clone(),
                after: after.clone(),
                before: before.clone(),
                role: role.clone(),
                content_type: content_type.clone(),
                tag: tag.clone(),
                limit: *limit,
                offset: *offset,
                hybrid: !*lexical_only,
                similarity_threshold: *similarity_threshold,
                ..SearchParams::default()
            };
            let results = engine.search(query, &params)?;
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
        Commands::Resume {
            query,
            project,
            all_projects,
            robot,
            source,
        } => {
            let engine = create_engine(&config)?;

            // Auto-sync before resume search
            sync_manifest_inputs(engine.as_ref(), &input_config)?;

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
            let params = SearchParams {
                project: effective_project,
                source: source_filter,
                limit: 20,
                ..SearchParams::default()
            };
            let results = engine.search(query, &params)?;
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
            sync_manifest_inputs(engine.as_ref(), &input_config)?;

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
            sync_manifest_inputs(engine.as_ref(), &input_config)?;

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
        Commands::Purge { before } => {
            let engine = create_engine(&config)?;
            println!("Purging data before {}...", before);
            let stats = engine.purge(before)?;
            let before_mb = stats.size_before as f64 / 1_048_576.0;
            let after_mb = stats.size_after as f64 / 1_048_576.0;
            println!(
                "Purged {} items, {} orphaned files. DB size: {:.2} MB → {:.2} MB",
                stats.deleted_items, stats.deleted_files, before_mb, after_mb
            );
        }
        Commands::Reindex {
            no_plans: _,
            no_embeddings: _,
        } => {
            let engine = create_engine(&config)?;
            println!("Clearing index hashes for full re-sync...");
            engine.clear_hashes()?;
            sync_manifest_inputs(engine.as_ref(), &input_config)?;
            println!("Reindex complete.");
        }
        Commands::Validate => {
            let engine = create_engine(&config)?;
            let report = engine.validate()?;
            let total = report.total_issues();
            if total == 0 {
                println!("Index is healthy. 0 issues found.");
            } else {
                println!("Validation found {} issue(s):", total);
                if report.orphaned_items > 0 {
                    println!(
                        "  Orphaned source paths (file missing on disk): {}",
                        report.orphaned_items
                    );
                }
                if report.stale_files > 0 {
                    println!(
                        "  Stale indexed_files (no search_items):        {}",
                        report.stale_files
                    );
                }
                if report.fts_inconsistencies > 0 {
                    println!(
                        "  FTS inconsistencies (orphaned FTS rows):      {}",
                        report.fts_inconsistencies
                    );
                }
                std::process::exit(1);
            }
        }
        Commands::Insights {
            project,
            all_projects,
            json,
            robot,
        } => {
            let engine = create_engine(&config)?;

            // Auto-sync before insights
            sync_manifest_inputs(engine.as_ref(), &input_config)?;

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

            let data = engine.get_insights(effective_project.as_deref())?;

            if *json {
                println!("{}", serde_json::to_string(&data).unwrap_or_default());
            } else if *robot {
                println!("total_sessions\t{}", data.total_sessions);
                println!("total_messages\t{}", data.total_messages);
                for d in &data.daily_activity {
                    println!("daily\t{}\t{}\t{}", d.date, d.sessions, d.messages);
                }
                for t in &data.tag_distribution {
                    println!("tag\t{}\t{}", t.tag, t.count);
                }
            } else {
                println!("Session Insights");
                println!(
                    "  Total sessions: {}  Total messages: {}",
                    data.total_sessions, data.total_messages
                );
                if !data.daily_activity.is_empty() {
                    println!("\nDaily Activity (last 30 days):");
                    println!("  {:<12} {:>10} {:>10}", "DATE", "SESSIONS", "MESSAGES");
                    println!("  {}", "-".repeat(34));
                    for d in &data.daily_activity {
                        println!("  {:<12} {:>10} {:>10}", d.date, d.sessions, d.messages);
                    }
                }
                if !data.tag_distribution.is_empty() {
                    println!("\nSession Categories:");
                    println!("  {:<20} {:>10}", "TAG", "SESSIONS");
                    println!("  {}", "-".repeat(32));
                    for t in &data.tag_distribution {
                        println!("  {:<20} {:>10}", t.tag, t.count);
                    }
                }
            }
        }
        Commands::Export {
            query,
            format,
            project,
            all_projects,
            source,
            after,
            before,
            role,
            limit,
        } => {
            let engine = create_engine(&config)?;

            // Auto-sync
            sync_manifest_inputs(engine.as_ref(), &input_config)?;

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

            let source_filter = if source == "all" {
                None
            } else {
                Some(source.clone())
            };

            let params = SearchParams {
                project: effective_project,
                source: source_filter,
                after: after.clone(),
                before: before.clone(),
                role: role.clone(),
                limit: *limit,
                ..SearchParams::default()
            };
            let results = engine.search(query, &params)?;

            match format.as_str() {
                "csv" => {
                    println!("source_path,role,score,timestamp,snippet");
                    for r in &results {
                        let snippet = r
                            .match_snippet
                            .as_deref()
                            .unwrap_or(&r.text)
                            .replace('"', "\"\"")
                            .replace(">>>", "")
                            .replace("<<<", "");
                        let ts = r.timestamp.as_deref().unwrap_or("");
                        println!(
                            "\"{}\",\"{}\",{:.4},\"{}\",\"{}\"",
                            r.source_path.replace('"', "\"\""),
                            r.role,
                            r.score,
                            ts,
                            snippet
                        );
                    }
                }
                _ => {
                    println!("# Search Results: {}\n", query);
                    println!("**Results**: {}\n", results.len());
                    for (i, r) in results.iter().enumerate() {
                        let ts = r.timestamp.as_deref().unwrap_or("N/A");
                        println!("## {}. [{}] {}\n", i + 1, r.role, r.source_path);
                        println!("**Score**: {:.4} | **Timestamp**: {}\n", r.score, ts);
                        let snippet = r
                            .match_snippet
                            .as_deref()
                            .unwrap_or(&r.text)
                            .replace(">>>", "**")
                            .replace("<<<", "**");
                        println!("> {}\n", snippet);
                    }
                }
            }
        }
        Commands::Inputs { .. } => unreachable!("inputs commands are handled before DB setup"),
        Commands::Status => {
            println!("Backscroll v{}", env!("CARGO_PKG_VERSION"));
            println!("Base de datos: {}", config.database_path);
            println!(
                "Manifiestos de input activos: {}",
                input_config.active_inputs().len()
            );

            if let Ok(engine) = create_engine(&config) {
                // Auto-sync antes de mostrar stats
                let _ = sync_manifest_inputs(engine.as_ref(), &input_config);

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

                    if let Some(model) = &stats.embedding_model {
                        println!("Embedding model:  {model}");
                        println!("Embeddings:       {}", stats.embedding_count);
                    }
                    if !stats.source_breakdown.is_empty() {
                        println!("\nSources:");
                        for sc in &stats.source_breakdown {
                            println!("  {:<12} {} files", sc.source, sc.count);
                        }
                    }
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;

    fn test_input_config(root: &std::path::Path, active: bool) -> InputConfig {
        if active {
            fs::create_dir_all(root).unwrap();
        }
        InputConfig {
            manifests: Vec::new(),
            inputs: vec![backscroll::input_config::InputDefinition {
                id: "test".to_string(),
                source: "session".to_string(),
                active,
                discover: backscroll::input_config::DiscoverConfig {
                    roots: vec![root.to_string_lossy().into_owned()],
                    include: vec!["**/*.jsonl".to_string()],
                    exclude: vec!["**/subagents/**".to_string()],
                    follow_symlinks: false,
                },
                decode: backscroll::input_config::DecodeConfig {
                    format: backscroll::input_config::DecodeFormat::Jsonl,
                    encoding: "utf-8".to_string(),
                },
                record: backscroll::input_config::RecordConfig::default(),
                mapping: Some(backscroll::input_config::MapConfig {
                    role: "$.message.role".to_string(),
                    uuid: None,
                    timestamp: None,
                    session_id: None,
                    project: None,
                    role_aliases: std::collections::BTreeMap::new(),
                }),
                content: Some(backscroll::input_config::ContentConfig {
                    selector: "$.message.content".to_string(),
                    string: "$".to_string(),
                    blocks: None,
                    block_text: None,
                    content_type: None,
                    include_when: Vec::new(),
                    exclude_when: Vec::new(),
                    default_content_type: "text".to_string(),
                }),
                text: backscroll::input_config::TextConfig::default(),
            }],
        }
    }

    #[test]
    fn sync_manifest_inputs_indexes_active_inputs() -> miette::Result<()> {
        let dir = tempdir().unwrap();
        let data = dir.path().join("data");
        fs::create_dir(&data).unwrap();
        fs::write(
            data.join("session.jsonl"),
            r#"{"message":{"role":"user","content":"manifest sync signal"}}"#,
        )
        .unwrap();
        let input_config = test_input_config(&data, true);

        let db_dir = tempdir().unwrap();
        let db_path = db_dir.path().join("sync_manifest.db");
        let db = Database::open(db_path.to_str().unwrap())?;
        db.setup_schema()?;

        sync_manifest_inputs(&db, &input_config)?;

        let results = db.search("signal", &SearchParams::default())?;
        assert!(!results.is_empty());
        Ok(())
    }

    #[test]
    fn sync_manifest_inputs_does_not_use_implicit_inputs_when_empty() -> miette::Result<()> {
        let db_dir = tempdir().unwrap();
        let db_path = db_dir.path().join("sync_empty.db");
        let db = Database::open(db_path.to_str().unwrap())?;
        db.setup_schema()?;

        sync_manifest_inputs(&db, &InputConfig::default())?;

        assert_eq!(db.get_stats()?.file_count, 0);
        Ok(())
    }

    #[test]
    fn sync_manifest_inputs_ignores_inactive_inputs() -> miette::Result<()> {
        let dir = tempdir().unwrap();
        let data = dir.path().join("inactive");
        let input_config = test_input_config(&data, false);

        let db_dir = tempdir().unwrap();
        let db_path = db_dir.path().join("sync_inactive.db");
        let db = Database::open(db_path.to_str().unwrap())?;
        db.setup_schema()?;

        sync_manifest_inputs(&db, &input_config)?;

        assert_eq!(db.get_stats()?.file_count, 0);
        Ok(())
    }
}
