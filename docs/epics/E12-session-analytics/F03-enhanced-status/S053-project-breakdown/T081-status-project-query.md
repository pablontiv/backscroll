---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T081: Add project breakdown query

**Story**: [S053 Per-project breakdown](README.md)
**Contribuye a**: P3 (status incluye breakdown por proyecto)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Add query to get session/message counts grouped by project.

## Especificacion Tecnica

Add `get_project_breakdown()` to Database impl:

```sql
SELECT project, COUNT(DISTINCT source_path) as sessions, COUNT(*) as messages
FROM search_items
GROUP BY project
ORDER BY sessions DESC
```

## Alcance

**In**: Add `get_project_breakdown()` to Database impl
**Out**: No output formatting (T082), no tests (T083)

## Criterios de Aceptacion

- Returns vector of (project, sessions, messages)
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
