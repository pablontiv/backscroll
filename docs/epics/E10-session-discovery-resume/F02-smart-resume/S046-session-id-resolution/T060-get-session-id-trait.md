---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T060: Add get_session_id to SearchEngine

**Story**: [S046 Session ID resolution](README.md)
**Contribuye a**: P3 — resume produce session ID usable por claude --resume

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

SearchEngine trait necesita un metodo para resolver session ID dado un source_path.

## Especificacion Tecnica

1. En `src/core/mod.rs`, agregar a SearchEngine trait:
   ```rust
   fn get_session_id(&self, source_path: &str) -> miette::Result<Option<String>>;
   ```

2. En `src/storage/sqlite.rs`, implementar:
   ```sql
   SELECT uuid FROM search_items
   WHERE source_path = ? AND uuid IS NOT NULL
   ORDER BY ordinal LIMIT 1
   ```

3. Si no hay UUID, retornar el file stem como fallback

## Alcance

**In**: Trait method, SQLite implementation
**Out**: No cambiar el dispatch de resume (T058)

## Criterios de Aceptacion

- Trait method definido
- SQLite impl retorna UUID del primer record
- Fallback a file stem si no hay UUID

## Fuente de verdad

- `src/core/mod.rs` — SearchEngine trait
- `src/storage/sqlite.rs` — Database impl
