---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T014: Extender SearchResult con match_snippet

**Story**: [S025 Implementar SearchEngine para Database](README.md)
**Contribuye a**: SearchResult incluye match_snippet field

[[blocks:T013-impl-search-engine]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

El campo `match_snippet: Option<String>` fue agregado al trait en T012 pero el impl lo devuelve como `None` temporalmente. Este task asegura que la estructura completa de SearchResult funciona end-to-end, preparando para E08 donde el snippet se populara con FTS5 `snippet()`.

## Alcance

**In**:
1. Verificar que SearchResult.match_snippet se propaga correctamente en la impl
2. Agregar campo `source_path` al output de search (reemplaza `path`)
3. Test que verifica SearchResult tiene todos los campos esperados

**Out**: Popular snippet con FTS5 snippet() (E08/S034). Solo preparar la estructura.

## Estado inicial esperado

- impl SearchEngine for Database existe (T013)
- SearchResult tiene match_snippet: Option<String> (T012)

## Criterios de Aceptacion

- SearchResult en tests tiene campos: source_path, text, match_snippet, score
- `cargo test` pasa
- `just check` pasa

## Fuente de verdad

- `src/core/mod.rs` — SearchResult struct
- `src/storage/sqlite.rs` — impl
