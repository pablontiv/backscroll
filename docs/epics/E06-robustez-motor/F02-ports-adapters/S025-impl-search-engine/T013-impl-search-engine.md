---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T013: Escribir impl SearchEngine for Database

**Story**: [S025 Implementar SearchEngine para Database](README.md)
**Contribuye a**: impl SearchEngine for Database existe y compila

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

El trait `SearchEngine` esta definido pero no tiene implementacion concreta. `Database` tiene metodos propios que hacen lo mismo pero no a traves del trait. Se debe implementar el trait para `Database` usando el nuevo schema (search_items + external FTS5).

## Alcance

**In**:
1. `impl SearchEngine for Database` en `src/storage/sqlite.rs`
2. `sync_files`: iterates ParsedFile → DELETE old + INSERT new en search_items
3. `search`: query messages_fts con BM25 ranking, JOIN search_items para metadata
4. `get_file_hashes`: SELECT from indexed_files

**Out**: No refactorizar main.rs para usar el trait (S026).

## Estado inicial esperado

- SearchEngine trait rediseñado (T012)
- search_items + FTS5 external content con triggers (T005-T007)
- Database tiene metodos legacy (index_message, search)

## Criterios de Aceptacion

- `grep "impl SearchEngine for Database" src/storage/sqlite.rs` encuentra la impl
- `cargo test` — sync y search funcionan via el trait
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
- `src/core/mod.rs`
