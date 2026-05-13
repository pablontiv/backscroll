---
estado: Completed
tipo: task
---
# T003: Ingestión de sesiones

**Outcome**: [Port a Go](README.md)

## Contexto

El módulo de sync es el más crítico funcionalmente: recorre directorios, parsea JSONL defensivamente, deduplica por SHA-256, filtra ruido, clasifica content-type y produce los registros que van a SQLite. Equivalente a `core/sync.rs` (2.045 líneas) + `core/models.rs`.

## Alcance

**In**:
1. `internal/sync` — WalkDir con `fs.WalkDir`, JSONL parsing defensivo (untagged enum equivalente con `json.RawMessage`), SHA-256 dedup, filtro de ruido (system-reminder, task-notification, subagents), clasificación text/code/tool, auto-tagging (delegar a internal/tagging).
2. `internal/models` — tipos de dominio: `SessionRecord`, `MessageContent`, `ParsedFile`, `SearchResult`, `Stats`.
3. Tests con los fixtures existentes en `tests/fixtures/` (claude-preset, pi-session.jsonl, claude-tool-events.jsonl).

**Out**:
1. Escritura a SQLite (va en T005).
2. Parsers de sources externos (van en T004).

## Criterios de Aceptación

- `ParsedFile` producido desde `tests/fixtures/claude-preset/projects/project-a/session-main.jsonl` coincide con el output actual de Rust en campos clave (role, content, timestamp).
- El filtro de ruido excluye correctamente `system-reminder` y sesiones de subagentes.
- SHA-256 de un mismo archivo produce el mismo hash en dos llamadas consecutivas.
- `go test ./internal/sync/... ./internal/models/...` pasa.
