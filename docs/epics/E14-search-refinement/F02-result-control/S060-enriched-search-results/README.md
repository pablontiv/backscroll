# S060: Enriched Search Results

**Feature**: [F02 Result Control](../README.md)
**Capacidad**: SearchResult incluye timestamp y role, visibles en todos los formatos de output
**Cubre**: P4 (enriched results)

## Antes / Despues

**Antes**: SearchResult tiene source_path, text, match_snippet, score. No incluye timestamp ni role, aunque ambas columnas existen en search_items. El skill no puede ver cuando ni quien escribio un resultado.

**Despues**: SearchResult incluye timestamp (Option<String>) y role (String). Output text muestra role y timestamp. JSON y robot incluyen los campos. Snapshot tests actualizados.

## Criterios de Aceptacion (semanticos)

- [ ] SearchResult struct tiene campos timestamp y role
- [ ] --json output incluye timestamp y role en cada resultado
- [ ] --robot output incluye timestamp y role como columnas adicionales
- [ ] Text output muestra role y timestamp de forma legible
- [ ] Snapshot tests reflejan los nuevos campos

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T099](T099-searchresult-fields.md) | Add timestamp and role to SearchResult struct and formatters |
| [T100](T100-snapshot-updates.md) | Update snapshot tests for new result fields |

## Fuente de verdad

- `src/core/mod.rs:10-15` — SearchResult struct
- `src/output.rs` — format_results()
- `src/storage/sqlite.rs:225-280` — search() SELECT columns
