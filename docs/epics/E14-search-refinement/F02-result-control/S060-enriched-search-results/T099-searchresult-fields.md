---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T099: Add timestamp and role to SearchResult struct and formatters

**Story**: [S060 Enriched Search Results](README.md)
**Contribuye a**: SearchResult incluye timestamp y role visibles en todos los formatos

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

SearchResult en core/mod.rs:10-15 tiene: source_path, text, match_snippet, score. La query SQL en sqlite.rs ya accede a si.text y si.source_path via JOIN. Se necesita tambien SELECT si.timestamp y si.role, y propagarlos al struct.

Output formatters en output.rs renderizan SearchResult para text, json, robot. Necesitan manejar los nuevos campos.

## Alcance

**In**:
1. Agregar `timestamp: Option<String>` y `role: String` a SearchResult
2. Actualizar SELECT en sqlite.rs search() para incluir si.timestamp y si.role
3. Actualizar row mapping en sqlite.rs para poblar nuevos campos
4. Actualizar output.rs text formatter para mostrar [role] y timestamp
5. JSON y robot formatters incluyen los campos automaticamente via Serialize

**Out**: Snapshot test updates (T100)

## Estado inicial esperado

- SearchResult con 4 campos
- output.rs con format_results()

## Criterios de Aceptacion

- `backscroll search "test" --json | jq '.[0].role'` retorna "user" o "assistant"
- `backscroll search "test" --json | jq '.[0].timestamp'` retorna string o null
- Text output muestra role y timestamp de forma legible
- `cargo test --all-features` pasa

## Fuente de verdad

- `src/core/mod.rs:10-15` — SearchResult
- `src/storage/sqlite.rs:225-280` — search() query
- `src/output.rs` — formatters
