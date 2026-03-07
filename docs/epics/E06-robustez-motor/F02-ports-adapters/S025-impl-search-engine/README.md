# S025: Implementar SearchEngine para Database

**Feature**: [F02 Ports & Adapters](../README.md)
**Capacidad**: `Database` implementa `SearchEngine` trait sobre el nuevo schema. El trait deja de ser dead code.
**Cubre**: P3 del Epic (SearchEngine trait implementado) — implementacion concreta

[[blocks:S024-abstracciones-dominio]]

## Antes / Despues

**Antes**: `SearchEngine` trait definido pero sin `impl SearchEngine for Database`. Database tiene metodos propios (`index_message`, `search`) que no pasan por el trait. El trait es dead code con `#[allow(dead_code)]`.

**Despues**: `impl SearchEngine for Database` con los metodos del trait rediseñado. `SearchResult` incluye `match_snippet: Option<String>` para snippet extraction futura.

## Criterios de Aceptacion (semanticos)

- [ ] `impl SearchEngine for Database` existe y compila
- [ ] SearchResult incluye match_snippet field

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T013](T013-impl-search-engine.md) | Escribir impl SearchEngine for Database |
| [T014](T014-search-result-snippet.md) | Extender SearchResult con match_snippet |

## Fuente de verdad

- `src/storage/sqlite.rs` — impl del adapter
- `src/core/mod.rs` — trait definition
