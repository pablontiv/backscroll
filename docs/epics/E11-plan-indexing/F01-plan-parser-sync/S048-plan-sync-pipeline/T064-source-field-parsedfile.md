---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T064: Add source field to ParsedFile

**Story**: [S048 Plan sync pipeline](README.md)
**Contribuye a**: P1 — plans indexados con source='plan'

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

`ParsedFile` no tiene campo `source`. El INSERT en sqlite.rs usa DEFAULT 'session'. Para plans necesitamos especificar explicitamente el source.

## Especificacion Tecnica

1. En `src/core/mod.rs`, agregar a ParsedFile:
   ```rust
   pub source: String, // "session" | "plan"
   ```

2. En `src/core/sync.rs`, actualizar todas las construcciones de ParsedFile para incluir `source: "session".into()`

3. En `src/core/plans.rs`, parse_plan() ya construye con `source: "plan".into()`

4. En `src/storage/sqlite.rs`, actualizar INSERT para incluir source:
   ```sql
   INSERT OR IGNORE INTO search_items (source, source_path, ordinal, role, text, project, uuid, timestamp)
   VALUES (?, ?, ?, ?, ?, ?, ?, ?)
   ```

5. Actualizar tests existentes que construyen ParsedFile para incluir source

## Alcance

**In**: Campo en struct, INSERT actualizado, todos los call sites
**Out**: No agregar --source filter (T067)

## Criterios de Aceptacion

- `ParsedFile.source` existe
- INSERT incluye source column
- Tests existentes actualizados y pasan

## Fuente de verdad

- `src/core/mod.rs` — ParsedFile struct
- `src/storage/sqlite.rs` — sync_files() INSERT
