---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T031: Fallback: derivar slug del path del directorio

**Story**: [S033 Deteccion automatica de proyecto](README.md)
**Contribuye a**: SELECT count(*) FROM search_items WHERE project IS NULL = 0

[[blocks:T030-parse-sessions-index]]

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Cuando sessions-index.json no esta disponible o no tiene la sesion, derivar el proyecto del path del directorio. Claude Code organiza sesiones por proyecto en `~/.claude/projects/<project-hash>/`. El path del directorio contiene informacion del proyecto.

## Alcance

**In**:
1. Si sessions-index lookup retorna None → derivar slug del parent directory name
2. Integrar lookup en parse_sessions: para cada archivo, resolver proyecto
3. Pasar proyecto a ParsedFile.project
4. Garantizar project nunca es None (usar "unknown" como ultimo fallback)

**Out**: No agregar configuracion de paths adicional.

## Estado inicial esperado

- load_session_index funciona (T030)
- project siempre None en ParsedFile

## Criterios de Aceptacion

- `cargo test test_project_from_index` — proyecto viene del index
- `cargo test test_project_fallback` — proyecto viene del path si no hay index
- ParsedFile.project nunca es None
- `just check` pasa

## Fuente de verdad

- `src/core/sync.rs`
