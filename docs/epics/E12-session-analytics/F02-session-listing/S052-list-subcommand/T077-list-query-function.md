---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T077: Session listing query function

**Story**: [S052 List subcommand](README.md)
**Contribuye a**: P2 (list retorna sesiones con metadata)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Need a function that lists sessions with metadata from search_items.

## Especificacion Tecnica

Add `list_sessions()` to Database impl:

```sql
SELECT source_path, project, COUNT(*) as messages,
       MIN(timestamp) as started, MAX(timestamp) as ended
FROM search_items
WHERE source = 'session'
GROUP BY source_path
ORDER BY MAX(timestamp) DESC
LIMIT ?
```

Add project filter and --all-projects support.

## Alcance

**In**: Add `list_sessions()` to Database impl. Query with project filter and limit.
**Out**: No CLI subcommand (T078), no output formatting (T079)

## Criterios de Aceptacion

- `list_sessions(None, 10)` returns 10 most recent sessions
- `list_sessions(Some("proj"), 5)` filters by project
- Results include path, project, message count, timestamps
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`, `src/core/mod.rs` (SearchEngine trait)
