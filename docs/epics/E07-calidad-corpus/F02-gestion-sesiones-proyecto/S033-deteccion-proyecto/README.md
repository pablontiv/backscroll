# S033: Deteccion automatica de proyecto

**Feature**: [F02 Gestion de Sesiones y Proyecto](../README.md)
**Capacidad**: Cada mensaje indexado tiene un proyecto asociado non-NULL, derivado de sessions-index.json o del path del directorio.
**Cubre**: P3 del Epic (project non-NULL)

## Antes / Despues

**Antes**: `index_message` recibe `project: None` siempre (sync.rs linea 45). La columna project en la DB es siempre NULL. No se puede filtrar busquedas por proyecto.

**Despues**: Parser lee `sessions-index.json` cuando esta disponible para lookup de `projectPath`. Fallback: deriva slug del path del directorio. Columna project siempre non-NULL.

## Criterios de Aceptacion (semanticos)

- [x] `SELECT count(*) FROM search_items WHERE project IS NULL` = 0
- [x] `--project` filter funciona en search

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T030](T030-parse-sessions-index.md) | Parser sessions-index.json para lookup projectPath |
| [T031](T031-project-fallback.md) | Fallback: derivar slug del path del directorio |
| [T032](T032-test-project-filtering.md) | Test end-to-end de --project filtering |

## Fuente de verdad

- `src/core/sync.rs` — logica de deteccion de proyecto
- `src/config.rs` — path a sessions-index.json
- `~/.claude/projects/*/sessions-index.json` — formato del archivo
