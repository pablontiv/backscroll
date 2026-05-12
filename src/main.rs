#![deny(unsafe_code)]

mod output;

use crate::output::{OutputFormat, OutputOptions, format_results};
use backscroll::config::Config;
use backscroll::core::projects::{
    IdentificationConfidence, ProjectConfig, identify, load_global_registry,
};
use backscroll::core::sync::{dry_run_input_definition, parse_input_definitions};
use backscroll::core::{
    IndexedRecordQuery, ParsedMessage, ProjectBreakdown, SearchEngine, SearchParams, SourceCount,
    Stats,
};
use backscroll::input_config::InputConfig;
use backscroll::storage::sqlite::Database;
use clap::{Parser, Subcommand};
use miette::Result;
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};

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
        /// Read only the existing SQLite index without auto-syncing inputs
        #[arg(long, default_value_t = false)]
        indexed_only: bool,
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
    /// Query normalized session events without a search term
    Events {
        #[command(subcommand)]
        command: EventCommands,
    },
    /// Query indexed session records without a search term
    Sessions {
        #[command(subcommand)]
        command: SessionCommands,
    },
    /// Tooling for generic input manifests
    Inputs {
        #[command(subcommand)]
        command: InputCommands,
    },
    /// Manage and query the project identity registry
    Projects {
        #[command(subcommand)]
        command: ProjectsCommands,
    },
    /// Query and analyze decision records
    Decisions {
        #[command(subcommand)]
        command: DecisionsCommands,
    },
    /// Mostrar estado del índice
    Status {
        /// Emit machine-readable JSON
        #[arg(long, default_value_t = false)]
        json: bool,
        /// Read only the existing SQLite index without auto-syncing inputs
        #[arg(long, default_value_t = false)]
        indexed_only: bool,
    },
}

#[derive(Subcommand)]
enum EventCommands {
    /// Stream normalized session events in deterministic order
    Query {
        /// Emit JSON Lines (the stable output format for this command)
        #[arg(long, default_value_t = false)]
        jsonl: bool,
        /// Filter by project (default: derived from current directory)
        #[arg(short, long)]
        project: Option<String>,
        /// Query all projects instead of deriving the current project
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Filter by source/input type: session, plan, ke, decision, memory, rule, spec, backlog, or all
        #[arg(long, default_value = "session")]
        source: String,
        /// Filter by indexed source path (exact path, SQL LIKE pattern, or * glob pattern)
        #[arg(long)]
        source_path: Option<String>,
        /// Filter by normalized event type: message, tool_call, tool_result, command, error, metadata, other
        #[arg(long)]
        event_type: Option<String>,
        /// Only events at or after this timestamp/date
        #[arg(long)]
        after: Option<String>,
        /// Only events before this timestamp/date
        #[arg(long)]
        before: Option<String>,
        /// Maximum events to stream (default 100, 0 = no limit)
        #[arg(long, default_value_t = 100)]
        limit: usize,
        /// Read only the existing SQLite index without auto-syncing inputs
        #[arg(long, default_value_t = false)]
        indexed_only: bool,
    },
}

#[derive(Subcommand)]
enum SessionCommands {
    /// Stream indexed records in deterministic order
    Query {
        /// Emit JSON Lines (the stable output format for this command)
        #[arg(long, default_value_t = false)]
        jsonl: bool,
        /// Filter by project (default: derived from current directory)
        #[arg(short, long)]
        project: Option<String>,
        /// Query all projects instead of deriving the current project
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Filter by source/input type: session, plan, ke, decision, memory, rule, spec, backlog, or all
        #[arg(long, default_value = "session")]
        source: String,
        /// Filter by indexed source path (exact path, SQL LIKE pattern, or * glob pattern)
        #[arg(long)]
        source_path: Option<String>,
        /// Only records at or after this timestamp/date
        #[arg(long)]
        after: Option<String>,
        /// Only records before this timestamp/date
        #[arg(long)]
        before: Option<String>,
        /// Maximum records to stream (default 100, 0 = no limit)
        #[arg(long, default_value_t = 100)]
        limit: usize,
        /// Maximum text characters per record (default 2000, 0 = no text)
        #[arg(long, default_value_t = 2000)]
        max_chars: usize,
        /// Read only the existing SQLite index without auto-syncing inputs
        #[arg(long, default_value_t = false)]
        indexed_only: bool,
    },
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

#[derive(Subcommand)]
enum ProjectsCommands {
    /// Identify the canonical project for a directory path
    Identify {
        /// Directory path to identify (default: current directory)
        #[arg(long)]
        cwd: Option<PathBuf>,
        /// Emit machine-readable JSON
        #[arg(long, default_value_t = false)]
        json: bool,
    },
    /// List all projects in the registry
    List {
        /// Emit machine-readable JSON
        #[arg(long, default_value_t = false)]
        json: bool,
    },
    /// Show aliases for a project
    Aliases {
        /// Project ID to look up
        #[arg(long = "project-id")]
        project_id: String,
        /// Emit machine-readable JSON
        #[arg(long, default_value_t = false)]
        json: bool,
    },
}

#[derive(Subcommand)]
enum DecisionsCommands {
    /// Query indexed decision records
    Query {
        /// Filter by project (default: derived from current directory)
        #[arg(short, long)]
        project: Option<String>,
        /// Query all projects instead of deriving the current project
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Filter by decision status: accepted, proposed, candidate, rejected, superseded, conflicted, obsolete
        #[arg(long)]
        status: Option<String>,
        /// Filter by decision scope: technical, organizational, architectural, operational
        #[arg(long)]
        scope: Option<String>,
        /// Emit JSON Lines (the stable output format for this command)
        #[arg(long, default_value_t = false)]
        json: bool,
        /// Maximum records to stream (default 100, 0 = no limit)
        #[arg(long, default_value_t = 100)]
        limit: usize,
        /// Read only the existing SQLite index without auto-syncing inputs
        #[arg(long, default_value_t = false)]
        indexed_only: bool,
    },
    /// Get bounded decision context for LLM injection
    Context {
        /// Filter by project (default: derived from current directory)
        #[arg(short, long)]
        project: Option<String>,
        /// Query all projects instead of deriving the current project
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Maximum tokens (approximate) for context (default: 8000)
        #[arg(long, default_value = "8000")]
        max_tokens: usize,
        /// Emit JSON output
        #[arg(long, default_value_t = false)]
        json: bool,
        /// Read only the existing SQLite index without auto-syncing inputs
        #[arg(long, default_value_t = false)]
        indexed_only: bool,
    },
    /// Extract decision candidates from indexed session records using heuristics
    Extract {
        /// Filter by project (default: derived from current directory)
        #[arg(short, long)]
        project: Option<String>,
        /// Query all projects instead of deriving the current project
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Only sessions since this date (ISO 8601, e.g. 2026-01-01)
        #[arg(long)]
        since: Option<String>,
        /// Maximum candidates to emit (default 0 = no limit)
        #[arg(long, default_value_t = 0)]
        limit: usize,
        /// Emit JSON Lines output
        #[arg(long, default_value_t = false)]
        jsonl: bool,
        /// Read only the existing SQLite index without auto-syncing inputs
        #[arg(long, default_value_t = false)]
        indexed_only: bool,
    },
    /// Detect conflicts between a proposed decision and the indexed corpus
    Conflicts {
        /// JSON string with proposal to check (or read from stdin if omitted)
        #[arg(long)]
        proposal_json: Option<String>,
        /// Filter by project (default: derived from current directory)
        #[arg(short, long)]
        project: Option<String>,
        /// Query all projects
        #[arg(long, default_value_t = false)]
        all_projects: bool,
        /// Emit JSON output
        #[arg(long, default_value_t = false)]
        json: bool,
        /// Read only the existing SQLite index without auto-syncing inputs
        #[arg(long, default_value_t = false)]
        indexed_only: bool,
    },
}

#[derive(Serialize)]
struct CandidateEvidence {
    source_path: String,
    snippet: String,
    timestamp: Option<String>,
}

#[derive(Serialize)]
struct DecisionCandidate {
    statement: String,
    status: String,
    confidence: f64,
    cluster_id: String,
    evidence: Vec<CandidateEvidence>,
}

#[derive(Serialize)]
struct ProjectIdentifyOutput {
    project_id: String,
    confidence: String,
    cwd: String,
}

#[derive(Serialize)]
struct ProjectListOutput {
    count: usize,
    projects: Vec<ProjectConfig>,
}

#[derive(Serialize)]
struct ProjectAliasesOutput {
    project_id: String,
    aliases: Vec<String>,
}

#[derive(Serialize, Deserialize)]
#[serde(crate = "serde")]
struct ProposalInput {
    id: Option<String>,
    statement: String,
    status: Option<String>,
    scope: Option<String>,
}

#[derive(Serialize)]
struct ConflictHint {
    conflict_type: String,
    existing_decision_id: Option<String>,
    existing_statement: String,
    existing_status: String,
    source_path: String,
    explanation: String,
}

#[derive(Serialize)]
struct DecisionRecord {
    id: Option<String>,
    title: String,
    status: String,
    scope: Option<String>,
    source_path: String,
    provenance: serde_json::Value,
    is_accepted: bool,
    excerpt: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    freshness: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    last_seen: Option<String>,
}

#[derive(Serialize)]
struct DecisionContext {
    project: Option<String>,
    decisions: Vec<DecisionRecord>,
    total_tokens: usize,
}

fn extract_frontmatter(text: &str) -> (Option<serde_json::Value>, String) {
    use backscroll::core::sources::parse_frontmatter;

    let (fm_opt, rest) = parse_frontmatter(text);
    let fm_value = fm_opt.and_then(|fm_str| serde_json::from_str(&fm_str).ok());
    (fm_value, rest.to_string())
}

fn get_decision_metadata(
    text: &str,
    source_path: &str,
) -> (Option<String>, String, String, Option<String>, bool) {
    let (fm_opt, rest) = extract_frontmatter(text);

    // Extract ID from frontmatter
    let id = fm_opt
        .as_ref()
        .and_then(|fm| fm.get("id"))
        .and_then(|v| v.as_str())
        .map(|s| s.to_string());

    // Extract status from frontmatter (default: "proposed")
    let status = fm_opt
        .as_ref()
        .and_then(|fm| fm.get("status"))
        .and_then(|v| v.as_str())
        .unwrap_or("proposed")
        .to_string();

    // Extract scope from frontmatter
    let scope = fm_opt
        .as_ref()
        .and_then(|fm| fm.get("scope"))
        .and_then(|v| v.as_str())
        .map(|s| s.to_string());

    // Accepted if status is "accepted" or "proposed" (pending acceptance)
    let is_accepted = status.to_lowercase() == "accepted";

    // Extract title from first heading or fallback to path
    let title = rest
        .lines()
        .find(|line| line.trim().starts_with('#'))
        .map(|line| line.trim().trim_start_matches('#').trim().to_string())
        .unwrap_or_else(|| {
            std::path::Path::new(source_path)
                .file_stem()
                .and_then(|stem| stem.to_str())
                .unwrap_or("Unknown")
                .to_string()
        });

    (id, title, status, scope, is_accepted)
}

fn normalize_statement(text: &str) -> String {
    text.to_lowercase()
        .chars()
        .map(|c| {
            if c.is_alphanumeric() || c.is_whitespace() {
                c
            } else {
                ' '
            }
        })
        .collect::<String>()
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
}

fn compute_freshness(last_seen: Option<&str>) -> String {
    match last_seen {
        None => "unknown".to_string(),
        Some(ts) => {
            if ts.starts_with("2026") {
                "active".to_string()
            } else if ts.starts_with("2025") || ts.starts_with("202") {
                "stale".to_string()
            } else {
                "unknown".to_string()
            }
        }
    }
}

fn compute_cluster_id(statement: &str) -> String {
    let normalized = normalize_statement(statement);
    normalized.chars().take(40).collect()
}

fn extract_significant_words(text: &str) -> Vec<String> {
    normalize_statement(text)
        .split_whitespace()
        .filter(|w| w.len() > 4)
        .map(|w| w.to_string())
        .collect()
}

fn count_keyword_overlap(statement1: &str, statement2: &str) -> usize {
    let words1: Vec<String> = extract_significant_words(statement1);
    let words2: Vec<String> = extract_significant_words(statement2);

    words1.iter().filter(|w| words2.contains(w)).count()
}

fn detect_conflicts(
    proposal: &ProposalInput,
    records: &[(String, String, String, Option<String>, String)],
) -> Vec<ConflictHint> {
    let mut hints = Vec::new();

    let proposal_statement_normalized = normalize_statement(&proposal.statement);
    let proposal_statement_prefix = proposal_statement_normalized
        .chars()
        .take(60)
        .collect::<String>();

    for (existing_id, existing_statement, existing_status, existing_scope, source_path) in records {
        let existing_statement_normalized = normalize_statement(existing_statement);
        let existing_statement_prefix = existing_statement_normalized
            .chars()
            .take(60)
            .collect::<String>();

        // Check for superseded: existing decision is superseded AND its id matches proposal.id
        if existing_status.to_lowercase() == "superseded" {
            if let Some(proposal_id) = &proposal.id {
                if proposal_id == existing_id {
                    hints.push(ConflictHint {
                        conflict_type: "superseded".to_string(),
                        existing_decision_id: Some(existing_id.clone()),
                        existing_statement: existing_statement.clone(),
                        existing_status: existing_status.clone(),
                        source_path: source_path.clone(),
                        explanation: format!(
                            "proposal is replacing a known superseded record with id {}",
                            proposal_id
                        ),
                    });
                    continue;
                }
            }
        }

        // Check for duplicate: normalized statement prefixes match (first 60 chars)
        if proposal_statement_prefix == existing_statement_prefix {
            hints.push(ConflictHint {
                conflict_type: "duplicate".to_string(),
                existing_decision_id: Some(existing_id.clone()),
                existing_statement: existing_statement.clone(),
                existing_status: existing_status.clone(),
                source_path: source_path.clone(),
                explanation: "proposal statement matches an existing decision (normalized prefix)"
                    .to_string(),
            });
            continue;
        }

        // Check for potential conflict: accepted AND same scope AND keyword overlap
        // Only if NOT a duplicate
        if existing_status.to_lowercase() == "accepted" {
            // Check if scopes match (if provided)
            let scopes_match = match (&proposal.scope, existing_scope) {
                (Some(p_scope), Some(e_scope)) => p_scope.to_lowercase() == e_scope.to_lowercase(),
                (None, None) => true, // Both unspecified
                _ => false,
            };

            if scopes_match {
                let keyword_overlap =
                    count_keyword_overlap(&proposal.statement, existing_statement);
                if keyword_overlap >= 2 {
                    hints.push(ConflictHint {
                        conflict_type: "potential_conflict".to_string(),
                        existing_decision_id: Some(existing_id.clone()),
                        existing_statement: existing_statement.clone(),
                        existing_status: existing_status.clone(),
                        source_path: source_path.clone(),
                        explanation: format!(
                            "accepted decision with same scope may conflict ({} keywords overlap)",
                            keyword_overlap
                        ),
                    });
                }
            }
        }
    }

    hints
}

fn extract_statement_from_snippet(snippet: &str) -> String {
    let lower = snippet.to_lowercase();

    let prefixes = vec![
        "we decided to ",
        "decision: ",
        "decided: ",
        "we have decided to ",
        "the decision is ",
        "we will use ",
        "we are using ",
        "we should ",
        "we need to ",
        "going forward ",
        "we will not ",
        "we must not ",
    ];

    for prefix in prefixes {
        if lower.starts_with(prefix) {
            let rest = &snippet[prefix.len()..];
            return rest.trim().to_string();
        }
    }

    snippet.trim().to_string()
}

fn get_confidence_for_snippet(snippet: &str) -> f64 {
    let lower = snippet.to_lowercase();

    // 0.90: exact phrase match
    if lower.starts_with("we decided to ")
        || lower.starts_with("decision: ")
        || lower.starts_with("decided: ")
        || lower.starts_with("we will not ")
        || lower.starts_with("we must not ")
    {
        0.90
    }
    // 0.75: second tier patterns
    else if lower.starts_with("we will use ")
        || lower.starts_with("we are using ")
        || lower.starts_with("we have decided ")
        || lower.starts_with("the decision is ")
    {
        0.75
    }
    // 0.60: third tier patterns
    else if (lower.starts_with("we should ") && !lower.starts_with("we should not "))
        || lower.starts_with("we need to ")
        || lower.starts_with("going forward ")
    {
        0.60
    } else {
        0.0
    }
}

fn matches_decision_pattern(line: &str) -> bool {
    let lower = line.to_lowercase();
    lower.contains("we decided to ")
        || lower.contains("decision: ")
        || lower.contains("decided: ")
        || lower.contains("we have decided ")
        || lower.contains("the decision is ")
        || lower.contains("we will use ")
        || lower.contains("we are using ")
        || lower.contains("we should ")
        || lower.contains("we need to ")
        || lower.contains("going forward ")
        || lower.contains("we will not ")
        || lower.contains("we must not ")
}

fn extract_decision_candidates(
    records: Vec<backscroll::core::IndexedRecord>,
    since_filter: &Option<String>,
) -> Vec<DecisionCandidate> {
    use std::collections::BTreeMap;

    let mut candidates_by_cluster: BTreeMap<String, (f64, Vec<CandidateEvidence>)> =
        BTreeMap::new();

    for record in records {
        // Apply since filter if provided
        if let Some(since) = since_filter {
            if let Some(ref timestamp) = record.timestamp {
                if timestamp < since {
                    continue;
                }
            } else {
                // Skip records without timestamp if since filter is active
                continue;
            }
        }

        // Process each line in the record text
        for line in record.text.lines() {
            let trimmed = line.trim();

            // Skip lines that are too short or too long
            if trimmed.len() < 20 || trimmed.len() > 500 {
                continue;
            }

            // Check if line matches decision patterns
            if !matches_decision_pattern(trimmed) {
                continue;
            }

            let confidence = get_confidence_for_snippet(trimmed);
            if confidence <= 0.0 {
                continue;
            }

            let statement = extract_statement_from_snippet(trimmed);
            let cluster_id = compute_cluster_id(&statement);

            let evidence = CandidateEvidence {
                source_path: record.source_path.clone(),
                snippet: trimmed.to_string(),
                timestamp: record.timestamp.clone(),
            };

            candidates_by_cluster
                .entry(cluster_id.clone())
                .and_modify(|(max_conf, _)| {
                    if confidence > *max_conf {
                        *max_conf = confidence;
                    }
                })
                .or_insert((confidence, Vec::new()));

            candidates_by_cluster
                .get_mut(&cluster_id)
                .unwrap()
                .1
                .push(evidence);
        }
    }

    // Convert to DecisionCandidate vec, one per cluster
    candidates_by_cluster
        .into_iter()
        .map(|(cluster_id, (confidence, evidence))| {
            let statement = evidence
                .first()
                .map(|e| extract_statement_from_snippet(&e.snippet))
                .unwrap_or_default();

            DecisionCandidate {
                statement,
                status: "candidate".to_string(),
                confidence,
                cluster_id,
                evidence,
            }
        })
        .collect()
}

fn handle_decisions_command(command: &DecisionsCommands, config: &Config) -> Result<()> {
    match command {
        DecisionsCommands::Conflicts {
            proposal_json,
            project,
            all_projects,
            json: emit_json,
            indexed_only,
        } => {
            // Parse proposal from input or stdin
            let proposal_str = if let Some(json_str) = proposal_json {
                json_str.clone()
            } else {
                use std::io::Read;
                let mut buffer = String::new();
                std::io::stdin()
                    .read_to_string(&mut buffer)
                    .map_err(|err| miette::miette!("Failed to read from stdin: {}", err))?;
                buffer
            };

            let proposal: ProposalInput = serde_json::from_str(&proposal_str)
                .map_err(|err| miette::miette!("Failed to parse proposal JSON: {}", err))?;

            let engine = if *indexed_only {
                create_indexed_only_engine(config)?
            } else {
                create_engine(config)?
            };

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

            // Query all decision records for the project
            let query = backscroll::core::IndexedRecordQuery {
                project: effective_project,
                source: Some("decision".to_string()),
                source_path: None,
                after: None,
                before: None,
                limit: 0,
            };

            let records = engine.query_indexed_records(&query)?;
            let mut existing_decisions = Vec::new();

            for record in records {
                let (id, _title, rec_status, rec_scope, _is_accepted) =
                    get_decision_metadata(&record.text, &record.source_path);
                existing_decisions.push((
                    id.unwrap_or_default(),
                    record.text.clone(),
                    rec_status,
                    rec_scope,
                    record.source_path.clone(),
                ));
            }

            let hints = detect_conflicts(&proposal, &existing_decisions);

            if *emit_json {
                println!(
                    "{}",
                    serde_json::to_string(&hints).map_err(|err| {
                        miette::miette!("Failed to serialize conflict hints: {}", err)
                    })?
                );
            } else {
                if hints.is_empty() {
                    println!("No conflicts found.");
                } else {
                    println!("Conflict Analysis:");
                    println!(
                        "Proposal: {} (scope: {})\n",
                        proposal.statement,
                        proposal.scope.as_deref().unwrap_or("unspecified")
                    );
                    for (i, hint) in hints.iter().enumerate() {
                        println!(
                            "{}. [{}] {} at {}",
                            i + 1,
                            hint.conflict_type.to_uppercase(),
                            hint.existing_decision_id.as_deref().unwrap_or("(no id)"),
                            hint.source_path
                        );
                        println!("   Statement: {}", hint.existing_statement);
                        println!("   Status: {}", hint.existing_status);
                        println!("   Note: {}\n", hint.explanation);
                    }
                }
            }
        }
        DecisionsCommands::Extract {
            project,
            all_projects,
            since,
            limit,
            jsonl: _,
            indexed_only,
        } => {
            let engine = if *indexed_only {
                create_indexed_only_engine(config)?
            } else {
                create_engine(config)?
            };

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

            // Query all session records (not decision records)
            let query = backscroll::core::IndexedRecordQuery {
                project: effective_project,
                source: Some("session".to_string()),
                source_path: None,
                after: None,
                before: None,
                limit: 0,
            };

            let records = engine.query_indexed_records(&query)?;
            let mut candidates = extract_decision_candidates(records, since);

            // Sort by confidence descending, then by cluster_id
            candidates.sort_by(|a, b| {
                b.confidence
                    .partial_cmp(&a.confidence)
                    .unwrap_or(std::cmp::Ordering::Equal)
                    .then_with(|| a.cluster_id.cmp(&b.cluster_id))
            });

            // Apply limit if specified
            if *limit > 0 {
                candidates.truncate(*limit);
            }

            // Output as JSONL
            for candidate in candidates {
                println!(
                    "{}",
                    serde_json::to_string(&candidate).map_err(|err| {
                        miette::miette!("Failed to serialize decision candidate: {}", err)
                    })?
                );
            }
        }
        DecisionsCommands::Query {
            project,
            all_projects,
            status,
            scope,
            json,
            limit,
            indexed_only,
        } => {
            let engine = if *indexed_only {
                create_indexed_only_engine(config)?
            } else {
                create_engine(config)?
            };

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

            let query = backscroll::core::IndexedRecordQuery {
                project: effective_project,
                source: Some("decision".to_string()),
                source_path: None,
                after: None,
                before: None,
                limit: *limit,
            };

            let records = engine.query_indexed_records(&query)?;
            let mut decision_count = 0;

            for record in records {
                let (id, title, rec_status, rec_scope, is_accepted) =
                    get_decision_metadata(&record.text, &record.source_path);

                // Apply status filter if provided
                if let Some(status_filter) = status {
                    if rec_status.to_lowercase() != status_filter.to_lowercase() {
                        continue;
                    }
                }

                // Apply scope filter if provided
                if let Some(scope_filter) = scope {
                    if let Some(rec_scope_val) = &rec_scope {
                        if rec_scope_val.to_lowercase() != scope_filter.to_lowercase() {
                            continue;
                        }
                    } else {
                        continue;
                    }
                }

                // Truncate excerpt to first 200 chars
                let excerpt = record.text.chars().take(200).collect::<String>();

                let freshness = compute_freshness(record.timestamp.as_deref());

                let decision = DecisionRecord {
                    id: id.clone(),
                    title: title.clone(),
                    status: rec_status.clone(),
                    scope: rec_scope.clone(),
                    source_path: record.source_path.clone(),
                    provenance: serde_json::json!({
                        "source": record.source,
                        "timestamp": record.timestamp,
                        "ordinal": record.ordinal,
                    }),
                    is_accepted,
                    excerpt,
                    freshness: Some(freshness.clone()),
                    last_seen: record.timestamp.clone(),
                };

                if *json {
                    println!(
                        "{}",
                        serde_json::to_string(&decision).map_err(|err| {
                            miette::miette!("Failed to serialize decision record: {}", err)
                        })?
                    );
                } else {
                    let status_indicator = if is_accepted { "[✓]" } else { "[ ]" };
                    println!(
                        "{} {}\n   Status: {} | Scope: {} | Freshness: {} | {}\n",
                        status_indicator,
                        title,
                        rec_status,
                        rec_scope.as_deref().unwrap_or("unspecified"),
                        freshness,
                        record.source_path
                    );
                }

                decision_count += 1;
            }

            if !*json && decision_count == 0 {
                println!("No decisions found.");
            }
        }
        DecisionsCommands::Context {
            project,
            all_projects,
            max_tokens,
            json,
            indexed_only,
        } => {
            let engine = if *indexed_only {
                create_indexed_only_engine(config)?
            } else {
                create_engine(config)?
            };

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

            // Fetch all decisions for the project
            let query = backscroll::core::IndexedRecordQuery {
                project: effective_project.clone(),
                source: Some("decision".to_string()),
                source_path: None,
                after: None,
                before: None,
                limit: 0,
            };

            let records = engine.query_indexed_records(&query)?;
            let mut decisions = Vec::new();
            let mut total_tokens = 0;

            for record in records {
                let (id, title, rec_status, rec_scope, is_accepted) =
                    get_decision_metadata(&record.text, &record.source_path);

                // Create a bounded excerpt suitable for context
                let max_excerpt_chars = (*max_tokens as f64 * 4.0) as usize;
                let excerpt = record
                    .text
                    .chars()
                    .take(max_excerpt_chars)
                    .collect::<String>();

                let token_estimate = excerpt.split_whitespace().count() + 10;
                if total_tokens + token_estimate > *max_tokens {
                    break;
                }

                let freshness = compute_freshness(record.timestamp.as_deref());

                let decision = DecisionRecord {
                    id,
                    title,
                    status: rec_status,
                    scope: rec_scope,
                    source_path: record.source_path.clone(),
                    provenance: serde_json::json!({
                        "source": record.source,
                        "timestamp": record.timestamp,
                        "ordinal": record.ordinal,
                    }),
                    is_accepted,
                    excerpt,
                    freshness: Some(freshness),
                    last_seen: record.timestamp.clone(),
                };

                total_tokens += token_estimate;
                decisions.push(decision);
            }

            let context = DecisionContext {
                project: effective_project,
                decisions,
                total_tokens,
            };

            if *json {
                println!(
                    "{}",
                    serde_json::to_string(&context).map_err(|err| {
                        miette::miette!("Failed to serialize decision context: {}", err)
                    })?
                );
            } else {
                println!("Decision Context (approx {} tokens)", context.total_tokens);
                println!("Project: {}", context.project.as_deref().unwrap_or("(all)"));
                println!("Decisions: {}\n", context.decisions.len());

                for decision in &context.decisions {
                    let status_indicator = if decision.is_accepted { "[✓]" } else { "[ ]" };
                    println!(
                        "{} {}\n   Status: {} | Scope: {}\n",
                        status_indicator,
                        decision.title,
                        decision.status,
                        decision.scope.as_deref().unwrap_or("unspecified")
                    );
                }
            }
        }
    }
    Ok(())
}

fn handle_projects_command(command: &ProjectsCommands) -> Result<()> {
    let registry = load_global_registry();
    match command {
        ProjectsCommands::Identify { cwd, json } => {
            let effective_cwd = match cwd {
                Some(p) => p.clone(),
                None => std::env::current_dir()
                    .map_err(|e| miette::miette!("Cannot determine current directory: {}", e))?,
            };
            let result = identify(&effective_cwd, &registry);
            let confidence_str = match result.confidence {
                IdentificationConfidence::Exact => "exact",
                IdentificationConfidence::Pattern => "pattern",
                IdentificationConfidence::Hint => "hint",
                IdentificationConfidence::Truncated => "truncated",
                IdentificationConfidence::Unknown => "unknown",
            };
            if *json {
                let out = ProjectIdentifyOutput {
                    project_id: result.project_id,
                    confidence: confidence_str.to_string(),
                    cwd: effective_cwd.to_string_lossy().into_owned(),
                };
                println!("{}", serde_json::to_string(&out).unwrap());
            } else {
                println!(
                    "project: {} (confidence: {})",
                    result.project_id, confidence_str
                );
            }
        }
        ProjectsCommands::List { json } => {
            if *json {
                let out = ProjectListOutput {
                    count: registry.projects.len(),
                    projects: registry.projects.clone(),
                };
                println!("{}", serde_json::to_string(&out).unwrap());
            } else {
                for p in &registry.projects {
                    println!(
                        "{} — roots: {:?}  patterns: {:?}  aliases: {:?}",
                        p.id, p.roots, p.worktree_patterns, p.aliases
                    );
                }
            }
        }
        ProjectsCommands::Aliases { project_id, json } => {
            let aliases: Vec<String> = registry
                .projects
                .iter()
                .find(|p| &p.id == project_id)
                .map(|p| p.aliases.clone())
                .unwrap_or_default();
            if *json {
                let out = ProjectAliasesOutput {
                    project_id: project_id.clone(),
                    aliases,
                };
                println!("{}", serde_json::to_string(&out).unwrap());
            } else {
                for alias in &aliases {
                    println!("{}", alias);
                }
            }
        }
    }
    Ok(())
}

fn create_engine(config: &Config) -> Result<Box<dyn SearchEngine>> {
    let db = Database::open(&config.database_path)?;
    db.setup_schema()?;
    Ok(Box::new(db))
}

fn create_indexed_only_engine(config: &Config) -> Result<Box<dyn SearchEngine>> {
    let database_path = Path::new(&config.database_path);
    if !database_path.exists() {
        return Err(miette::miette!(
            "indexed-only requires an existing backscroll database: {}; run `backscroll sync` first",
            database_path.display()
        ));
    }

    let db = Database::open_readonly(database_path)?;
    db.ensure_usable_index()?;
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
struct StatusInputEntry {
    id: String,
    source: String,
    active: bool,
}

#[derive(Serialize)]
struct StatusInputs {
    active_count: usize,
    inputs: Vec<StatusInputEntry>,
}

#[derive(Serialize)]
struct StatusDatabase {
    path: String,
    exists: bool,
}

#[derive(Serialize)]
struct StatusIndex {
    usable: bool,
    files: i64,
    messages: i64,
    projects: i64,
    database_size_bytes: i64,
    last_sync: Option<String>,
    embedding_count: i64,
    embedding_model: Option<String>,
    sources: Vec<SourceCount>,
}

#[derive(Serialize)]
struct StatusJson {
    version: u8,
    database: StatusDatabase,
    inputs: StatusInputs,
    index: StatusIndex,
    projects: Vec<ProjectBreakdown>,
    diagnostics: Vec<String>,
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

fn status_inputs(input_config: &InputConfig) -> StatusInputs {
    let inputs: Vec<StatusInputEntry> = input_config
        .inputs
        .iter()
        .map(|input| StatusInputEntry {
            id: input.id.clone(),
            source: input.source.clone(),
            active: input.active,
        })
        .collect();
    let active_count = inputs.iter().filter(|input| input.active).count();

    StatusInputs {
        active_count,
        inputs,
    }
}

fn status_index_from_stats(stats: Stats) -> StatusIndex {
    StatusIndex {
        usable: true,
        files: stats.file_count,
        messages: stats.message_count,
        projects: stats.project_count,
        database_size_bytes: stats.db_size_bytes,
        last_sync: stats.last_sync,
        embedding_count: stats.embedding_count,
        embedding_model: stats.embedding_model,
        sources: stats.source_breakdown,
    }
}

fn empty_status_index() -> StatusIndex {
    StatusIndex {
        usable: false,
        files: 0,
        messages: 0,
        projects: 0,
        database_size_bytes: 0,
        last_sync: None,
        embedding_count: 0,
        embedding_model: None,
        sources: Vec::new(),
    }
}

fn build_status_json(
    config: &Config,
    input_config: &InputConfig,
    indexed_only: bool,
) -> Result<StatusJson> {
    let database_path = Path::new(&config.database_path);
    let mut diagnostics = Vec::new();
    let mut index = empty_status_index();
    let mut projects = Vec::new();

    if indexed_only && !database_path.exists() {
        diagnostics.push(format!(
            "indexed-only requires an existing backscroll database: {}; run `backscroll sync` first",
            database_path.display()
        ));
    } else {
        let engine = if indexed_only {
            match create_indexed_only_engine(config) {
                Ok(engine) => Some(engine),
                Err(err) => {
                    diagnostics.push(err.to_string());
                    None
                }
            }
        } else {
            let engine = create_engine(config)?;
            sync_manifest_inputs(engine.as_ref(), input_config)?;
            Some(engine)
        };

        if let Some(engine) = engine {
            match engine.get_stats() {
                Ok(stats) => index = status_index_from_stats(stats),
                Err(err) => diagnostics.push(format!("failed to read index stats: {err}")),
            }
            match engine.get_project_breakdown() {
                Ok(breakdown) => projects = breakdown,
                Err(err) => diagnostics.push(format!("failed to read project breakdown: {err}")),
            }
        }
    }

    Ok(StatusJson {
        version: 1,
        database: StatusDatabase {
            path: config.database_path.clone(),
            exists: database_path.exists(),
        },
        inputs: status_inputs(input_config),
        index,
        projects,
        diagnostics,
    })
}

fn print_status_json(
    config: &Config,
    input_config: &InputConfig,
    indexed_only: bool,
) -> Result<()> {
    let status = build_status_json(config, input_config, indexed_only)?;
    println!(
        "{}",
        serde_json::to_string_pretty(&status)
            .map_err(|err| miette::miette!("Failed to serialize status JSON: {}", err))?
    );
    Ok(())
}

fn bounded_text(text: &str, max_chars: usize) -> String {
    text.chars().take(max_chars).collect()
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

    if let Commands::Projects { command } = &cli.command {
        return handle_projects_command(command);
    }

    if let Commands::Decisions { command } = &cli.command {
        let config = Config::load().unwrap_or_else(|_| Config::default_with_paths());
        return handle_decisions_command(command, &config);
    }
    let input_config = if matches!(
        &cli.command,
        Commands::List {
            indexed_only: true,
            ..
        }
    ) {
        InputConfig::default()
    } else {
        InputConfig::load()?
    };

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
            indexed_only,
        } => {
            let engine = if *indexed_only {
                create_indexed_only_engine(&config)?
            } else {
                let engine = create_engine(&config)?;
                // Auto-sync before list
                sync_manifest_inputs(engine.as_ref(), &input_config)?;
                engine
            };

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
        Commands::Events {
            command:
                EventCommands::Query {
                    jsonl,
                    project,
                    all_projects,
                    source,
                    source_path,
                    event_type,
                    after,
                    before,
                    limit,
                    indexed_only,
                },
        } => {
            if !jsonl {
                return Err(miette::miette!(
                    "events query currently supports JSON Lines output; pass --jsonl"
                ));
            }

            let engine = if *indexed_only {
                create_indexed_only_engine(&config)?
            } else {
                let engine = create_engine(&config)?;
                sync_manifest_inputs(engine.as_ref(), &input_config)?;
                engine
            };

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
            let query = backscroll::core::SessionEventQuery {
                project: effective_project,
                source: source_filter,
                source_path: source_path.clone(),
                event_type: event_type.clone(),
                after: after.clone(),
                before: before.clone(),
                limit: *limit,
            };

            for event in engine.query_session_events(&query)? {
                println!(
                    "{}",
                    serde_json::to_string(&event).map_err(|err| {
                        miette::miette!("Failed to serialize session event: {}", err)
                    })?
                );
            }
        }
        Commands::Sessions {
            command:
                SessionCommands::Query {
                    jsonl,
                    project,
                    all_projects,
                    source,
                    source_path,
                    after,
                    before,
                    limit,
                    max_chars,
                    indexed_only,
                },
        } => {
            if !jsonl {
                return Err(miette::miette!(
                    "sessions query currently supports JSON Lines output; pass --jsonl"
                ));
            }

            let engine = if *indexed_only {
                create_indexed_only_engine(&config)?
            } else {
                let engine = create_engine(&config)?;
                sync_manifest_inputs(engine.as_ref(), &input_config)?;
                engine
            };

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
            let query = IndexedRecordQuery {
                project: effective_project,
                source: source_filter,
                source_path: source_path.clone(),
                after: after.clone(),
                before: before.clone(),
                limit: *limit,
            };

            for mut record in engine.query_indexed_records(&query)? {
                record.text = bounded_text(&record.text, *max_chars);
                println!(
                    "{}",
                    serde_json::to_string(&record).map_err(|err| {
                        miette::miette!("Failed to serialize indexed record: {}", err)
                    })?
                );
            }
        }
        Commands::Inputs { .. } => unreachable!("inputs commands are handled before DB setup"),
        Commands::Projects { .. } => unreachable!("projects commands are handled before DB setup"),
        Commands::Decisions { command } => {
            return handle_decisions_command(command, &config);
        }
        Commands::Status { json, indexed_only } => {
            if *json {
                print_status_json(&config, &input_config, *indexed_only)?;
                return Ok(());
            }

            println!("Backscroll v{}", env!("CARGO_PKG_VERSION"));
            println!("Base de datos: {}", config.database_path);
            println!(
                "Manifiestos de input activos: {}",
                input_config.active_inputs().len()
            );

            let engine = if *indexed_only {
                create_indexed_only_engine(&config)
            } else {
                let engine = create_engine(&config);
                if let Ok(engine) = &engine {
                    // Auto-sync antes de mostrar stats
                    let _ = sync_manifest_inputs(engine.as_ref(), &input_config);
                }
                engine
            };

            if let Ok(engine) = engine {
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

    #[test]
    fn extract_candidates_finds_decided_statement() {
        let records = vec![backscroll::core::IndexedRecord {
            schema_version: 1,
            source: "session".to_string(),
            source_path: "session-abc.jsonl".to_string(),
            ordinal: 1,
            role: "assistant".to_string(),
            text: "we decided to use ESM modules for all TypeScript code".to_string(),
            project: None,
            uuid: None,
            timestamp: Some("2026-05-01T10:00:00Z".to_string()),
            content_type: "text".to_string(),
        }];

        let candidates = extract_decision_candidates(records, &None);
        assert_eq!(candidates.len(), 1);
        let cand = &candidates[0];
        assert!(cand.confidence >= 0.9);
        assert_eq!(cand.status, "candidate");
        assert_eq!(cand.evidence.len(), 1);
    }

    #[test]
    fn extract_candidates_deduplicates_same_statement() {
        let records = vec![
            backscroll::core::IndexedRecord {
                schema_version: 1,
                source: "session".to_string(),
                source_path: "session-abc.jsonl".to_string(),
                ordinal: 1,
                role: "assistant".to_string(),
                text: "we decided to use ESM modules for all TypeScript code".to_string(),
                project: None,
                uuid: None,
                timestamp: Some("2026-05-01T10:00:00Z".to_string()),
                content_type: "text".to_string(),
            },
            backscroll::core::IndexedRecord {
                schema_version: 1,
                source: "session".to_string(),
                source_path: "session-def.jsonl".to_string(),
                ordinal: 2,
                role: "assistant".to_string(),
                text: "we decided to use ESM modules for all TypeScript code".to_string(),
                project: None,
                uuid: None,
                timestamp: Some("2026-05-02T11:00:00Z".to_string()),
                content_type: "text".to_string(),
            },
        ];

        let candidates = extract_decision_candidates(records, &None);
        assert_eq!(candidates.len(), 1);
        assert_eq!(candidates[0].evidence.len(), 2);
    }

    #[test]
    fn extract_candidates_skips_short_lines() {
        let records = vec![backscroll::core::IndexedRecord {
            schema_version: 1,
            source: "session".to_string(),
            source_path: "session-abc.jsonl".to_string(),
            ordinal: 1,
            role: "assistant".to_string(),
            text: "we decided".to_string(),
            project: None,
            uuid: None,
            timestamp: Some("2026-05-01T10:00:00Z".to_string()),
            content_type: "text".to_string(),
        }];

        let candidates = extract_decision_candidates(records, &None);
        assert_eq!(candidates.len(), 0);
    }

    #[test]
    fn extract_candidates_proposed_status_in_output() {
        let records = vec![backscroll::core::IndexedRecord {
            schema_version: 1,
            source: "session".to_string(),
            source_path: "session-abc.jsonl".to_string(),
            ordinal: 1,
            role: "assistant".to_string(),
            text: "we decided to use ESM modules for all TypeScript code".to_string(),
            project: None,
            uuid: None,
            timestamp: Some("2026-05-01T10:00:00Z".to_string()),
            content_type: "text".to_string(),
        }];

        let candidates = extract_decision_candidates(records, &None);
        assert_eq!(candidates[0].status, "candidate");
    }

    #[test]
    fn extract_candidates_since_filter_excludes_old_records() {
        let records = vec![
            backscroll::core::IndexedRecord {
                schema_version: 1,
                source: "session".to_string(),
                source_path: "session-old.jsonl".to_string(),
                ordinal: 1,
                role: "assistant".to_string(),
                text: "we decided to use old approach long ago".to_string(),
                project: None,
                uuid: None,
                timestamp: Some("2026-04-01T10:00:00Z".to_string()),
                content_type: "text".to_string(),
            },
            backscroll::core::IndexedRecord {
                schema_version: 1,
                source: "session".to_string(),
                source_path: "session-new.jsonl".to_string(),
                ordinal: 2,
                role: "assistant".to_string(),
                text: "we decided to use new approach recently".to_string(),
                project: None,
                uuid: None,
                timestamp: Some("2026-05-01T10:00:00Z".to_string()),
                content_type: "text".to_string(),
            },
        ];

        let since_filter = Some("2026-05-01".to_string());
        let candidates = extract_decision_candidates(records, &since_filter);
        assert_eq!(candidates.len(), 1);
        assert_eq!(candidates[0].evidence[0].source_path, "session-new.jsonl");
    }

    #[test]
    fn conflicts_superseded_detected() {
        let proposal = ProposalInput {
            id: Some("DEC-001".to_string()),
            statement: "use node for all scripts".to_string(),
            status: Some("proposed".to_string()),
            scope: None,
        };

        let existing_records = vec![(
            "DEC-001".to_string(),
            "use node for all scripts".to_string(),
            "superseded".to_string(),
            None,
            "docs/adr/0001.md".to_string(),
        )];

        let hints = detect_conflicts(&proposal, &existing_records);
        assert_eq!(hints.len(), 1);
        assert_eq!(hints[0].conflict_type, "superseded");
        assert_eq!(hints[0].existing_status, "superseded");
    }

    #[test]
    fn conflicts_duplicate_detected() {
        let proposal = ProposalInput {
            id: None,
            statement: "use typescript for frontend development".to_string(),
            status: Some("proposed".to_string()),
            scope: None,
        };

        let existing_records = vec![(
            "DEC-002".to_string(),
            "use typescript for frontend development".to_string(),
            "accepted".to_string(),
            None,
            "docs/adr/0002.md".to_string(),
        )];

        let hints = detect_conflicts(&proposal, &existing_records);
        assert_eq!(hints.len(), 1);
        assert_eq!(hints[0].conflict_type, "duplicate");
    }

    #[test]
    fn conflicts_potential_conflict_detected() {
        let proposal = ProposalInput {
            id: None,
            statement: "use pnpm package manager for builds".to_string(),
            status: Some("proposed".to_string()),
            scope: Some("technical".to_string()),
        };

        let existing_records = vec![(
            "DEC-003".to_string(),
            "npm package manager is our standard".to_string(),
            "accepted".to_string(),
            Some("technical".to_string()),
            "docs/adr/0003.md".to_string(),
        )];

        let hints = detect_conflicts(&proposal, &existing_records);
        assert_eq!(hints.len(), 1);
        assert_eq!(hints[0].conflict_type, "potential_conflict");
    }

    #[test]
    fn conflicts_no_conflict_returns_empty() {
        let proposal = ProposalInput {
            id: None,
            statement: "use bun for testing".to_string(),
            status: Some("proposed".to_string()),
            scope: Some("technical".to_string()),
        };

        let existing_records = vec![(
            "DEC-004".to_string(),
            "postgres is our database".to_string(),
            "accepted".to_string(),
            Some("technical".to_string()),
            "docs/adr/0004.md".to_string(),
        )];

        let hints = detect_conflicts(&proposal, &existing_records);
        assert_eq!(hints.len(), 0);
    }

    #[test]
    fn freshness_active_for_2026() {
        let result = compute_freshness(Some("2026-05-11T10:00:00Z"));
        assert_eq!(result, "active");
    }

    #[test]
    fn freshness_stale_for_2025() {
        let result = compute_freshness(Some("2025-01-01T10:00:00Z"));
        assert_eq!(result, "stale");
    }

    #[test]
    fn freshness_unknown_when_none() {
        let result = compute_freshness(None);
        assert_eq!(result, "unknown");
    }
}
