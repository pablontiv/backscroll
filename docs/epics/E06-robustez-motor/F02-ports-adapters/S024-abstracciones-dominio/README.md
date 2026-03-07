# S024: Abstracciones de dominio

**Feature**: [F02 Ports & Adapters](../README.md)
**Capacidad**: Existen structs intermedias (ParsedFile, ParsedMessage) y un SearchEngine trait rediseñado que desacopla parsing de storage.
**Cubre**: P3 del Epic (SearchEngine trait implementado) — define las abstracciones

## Antes / Despues

**Antes**: `sync.rs` importa `Database` directamente y llama `index_message` uno a uno. El trait `SearchEngine` existe pero con firma que no refleja el flujo real (no recibe archivos parseados). `SearchResult` tiene solo `{path, content, score}`.

**Despues**: `ParsedFile` y `ParsedMessage` structs en `core/mod.rs`. `SearchEngine` trait con `sync_files(Vec<ParsedFile>)`, `search()`, `get_file_hashes()`. `SearchResult` con `{source_path, text, match_snippet: Option<String>, score}`.

## Criterios de Aceptacion (semanticos)

- [ ] ParsedFile y ParsedMessage structs definidas y usadas
- [ ] SearchEngine trait refleja el flujo real del sistema

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T011](T011-parsed-structs.md) | Definir ParsedFile + ParsedMessage structs |
| [T012](T012-redesign-trait.md) | Rediseñar SearchEngine trait |

## Fuente de verdad

- `src/core/mod.rs` — trait y structs de dominio
