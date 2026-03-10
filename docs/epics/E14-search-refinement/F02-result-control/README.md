# F02: Result Control

**Epic**: [E14 Search Refinement](../README.md)
**Objetivo**: Resultados de busqueda son paginables y contienen metadata enriquecida (timestamp, role)
**Satisface**: P3 (pagination), P4 (enriched results)
**Milestone**: `backscroll search "query" --limit 50 --offset 20 --json` retorna resultados paginados con timestamp y role

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E14)
- INV2: `just check` pasa (heredado de E14)
- INV4: --limit default = 20 (heredado de E14)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S059](S059-configurable-pagination/) | Configurable Pagination |
| [S060](S060-enriched-search-results/) | Enriched Search Results |

## Dependencias

- Soft: F01 (Query Filters) — filters primero para coherencia del trait refactor
