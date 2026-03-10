---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T103: Add purge subcommand with --before date filter

**Story**: [S062 Purge Old Data](README.md)
**Contribuye a**: `backscroll purge --before DATE` existe como subcomando

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Se necesita un nuevo subcomando `purge` con flag `--before` que acepta fecha ISO 8601. El subcomando invoca un metodo del SearchEngine trait que ejecuta DELETE FROM search_items WHERE timestamp < ?.

## Alcance

**In**:
1. Agregar `Purge { before: String }` al enum Commands en main.rs
2. Agregar `purge(&self, before: &str) -> miette::Result<PurgeStats>` al SearchEngine trait
3. Agregar struct PurgeStats { deleted_items: i64, deleted_files: i64 } a core/mod.rs
4. Implementar purge basico en sqlite.rs (DELETE + conteo)

**Out**: VACUUM y orphan cleanup (T104), tests (T105)

## Estado inicial esperado

- main.rs con enum Commands
- SearchEngine trait

## Criterios de Aceptacion

- `backscroll purge --before 2025-01-01 --help` muestra flag sin error
- Trait compila con nuevo metodo
- `cargo test --all-features` pasa

## Fuente de verdad

- `src/main.rs` — Commands enum
- `src/core/mod.rs` — SearchEngine trait
- `src/storage/sqlite.rs` — purge implementation
