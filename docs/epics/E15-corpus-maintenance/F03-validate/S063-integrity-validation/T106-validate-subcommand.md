---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T106: Add validate subcommand with integrity queries

**Story**: [S063 Integrity Validation](README.md)
**Contribuye a**: `backscroll validate` reporta problemas de integridad

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV4: validate es read-only
  - Verificar: DB file modification time no cambia

## Contexto

El subcomando validate ejecuta queries de integridad sin modificar datos:
1. Orphaned search_items: source_path en search_items pero archivo no existe en disco
2. Stale indexed_files: path en indexed_files pero no hay search_items correspondientes
3. FTS5 consistency: rowids en messages_fts que no existen en search_items (raro pero posible)

Para verificar archivos en disco, se necesita iterar los source_paths distintos y verificar con std::path::Path::exists().

## Alcance

**In**:
1. Agregar `Validate` al enum Commands en main.rs
2. Agregar `validate(&self) -> miette::Result<ValidationReport>` al SearchEngine trait
3. Struct ValidationReport con conteos de orphans, stale files, FTS inconsistencies
4. Implementar queries en sqlite.rs
5. Imprimir reporte legible

**Out**: Auto-repair, tests (T107)

## Estado inicial esperado

- main.rs con Commands enum
- search_items y indexed_files tables

## Criterios de Aceptacion

- `backscroll validate` completa sin error
- Reporta conteo de cada tipo de problema
- Si no hay problemas: "Index is healthy. 0 issues found."
- Si hay problemas: lista cada tipo con conteo
- DB no es modificada (read-only)

## Fuente de verdad

- `src/main.rs` — Commands enum
- `src/core/mod.rs` — SearchEngine trait
- `src/storage/sqlite.rs` — integrity queries
