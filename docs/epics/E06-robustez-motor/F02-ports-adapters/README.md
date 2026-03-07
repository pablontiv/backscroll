# F02: Ports & Adapters

**Epic**: [E06 Robustez del Motor](../README.md)
**Objetivo**: Implementar el patron ports & adapters completo: SearchEngine trait funcional, Database como adapter, main.rs usa dyn dispatch.
**Satisface**: P3 (SearchEngine trait implementado)
**Milestone**: `grep -r "dyn SearchEngine" src/main.rs` encuentra uso; trait no es dead code.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E06)
- INV2: `just check` pasa (heredado de E06)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S024](S024-abstracciones-dominio/) | Abstracciones de dominio |
| [S025](S025-impl-search-engine/) | Implementar SearchEngine para Database |
| [S026](S026-refactor-sync-main/) | Refactorizar sync y main |
