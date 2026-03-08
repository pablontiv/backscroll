# S026: Refactorizar sync y main

**Feature**: [F02 Ports & Adapters](../README.md)
**Capacidad**: `sync.rs` es una funcion pura que retorna `Vec<ParsedFile>`. `main.rs` usa `dyn SearchEngine` factory. Zero `#[allow(dead_code)]`.
**Cubre**: P3 del Epic (main.rs usa dyn SearchEngine)

[[blocks:S025-impl-search-engine]]

## Antes / Despues

**Antes**: `sync.rs` importa `Database` directamente y ejecuta side effects (INSERT). `main.rs` construye `Database` directamente sin abstraccion. Hay `#[allow(dead_code)]` en el trait.

**Despues**: `sync.rs` retorna `Vec<ParsedFile>` (funcion pura). `main.rs` tiene factory `create_engine() -> Box<dyn SearchEngine>`. Zero `#[allow(dead_code)]` en el codebase.

## Criterios de Aceptacion (semanticos)

- [x] sync.rs no importa Database directamente
- [x] main.rs usa dyn SearchEngine
- [x] Zero `#[allow(dead_code)]` en src/

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T015](T015-sync-pure-function.md) | Refactorizar sync.rs a funcion pura |
| [T016](T016-main-dyn-dispatch.md) | Refactorizar main.rs con factory dyn SearchEngine |
| [T017](T017-remove-dead-code.md) | Remover #[allow(dead_code)], verificar clippy limpio |

## Fuente de verdad

- `src/core/sync.rs` — logica de sync
- `src/main.rs` — CLI entry point
