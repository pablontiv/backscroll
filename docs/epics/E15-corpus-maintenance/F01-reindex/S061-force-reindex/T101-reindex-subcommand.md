---
ejecutable_en: 1 sesion
estado: Completed # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T101: Add reindex subcommand with clear hashes and force re-sync

**Story**: [S061 Force Reindex](README.md)
**Contribuye a**: `backscroll reindex` re-procesa todos los archivos

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

El sync actual en main.rs llama `parse_sessions(dir, existing_hashes, include_agents)`. Cuando hashes match, los archivos se skipean. Para reindex: borrar todos los hashes en indexed_files (`DELETE FROM indexed_files`), luego ejecutar sync normal. Los triggers FTS5 se encargan de actualizar el indice automaticamente.

Se necesita agregar un nuevo variant `Reindex` al enum Commands en main.rs, y un metodo `clear_hashes()` al SearchEngine trait / Database impl.

## Alcance

**In**:
1. Agregar `Reindex` al enum Commands en main.rs
2. Agregar `clear_hashes(&self) -> miette::Result<()>` al SearchEngine trait
3. Implementar clear_hashes: `DELETE FROM indexed_files` en sqlite.rs
4. En el handler de Reindex: llamar clear_hashes() y luego ejecutar sync normal

**Out**: Tests (T102), opciones avanzadas (--path para reindex selectivo)

## Estado inicial esperado

- main.rs con enum Commands (7 variants)
- sqlite.rs con indexed_files table

## Criterios de Aceptacion

- `backscroll reindex` completa sin error
- Despues de reindex, `backscroll status` muestra mismos conteos que antes (datos re-procesados)
- `DELETE FROM indexed_files` ejecutado dentro de transaccion
- `cargo test --all-features` pasa

## Fuente de verdad

- `src/main.rs` — Commands enum, sync logic
- `src/core/mod.rs` — SearchEngine trait
- `src/storage/sqlite.rs` — indexed_files table
