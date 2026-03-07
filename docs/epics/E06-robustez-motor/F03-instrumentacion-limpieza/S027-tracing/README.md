# S027: Tracing (completa S020)

**Feature**: [F03 Instrumentacion y Limpieza](../README.md)
**Capacidad**: Tracing funcional con spans visibles. Completa S020 que agrego los crates pero no los uso.
**Cubre**: P5 del Epic (tracing instrumentado)

## Antes / Despues

**Antes**: Crates `tracing` y `tracing-subscriber` estan en Cargo.toml pero no se usan en el codigo. S020 fue marcada Completed prematuramente. Zero spans, zero log output estructurado.

**Despues**: `tracing-subscriber` configurado en main.rs con `env_filter` (controlable via `RUST_LOG`). Spans `#[instrument]` en `sync_sessions` y `search`. Logs visibles con `RUST_LOG=debug`.

## Criterios de Aceptacion (semanticos)

- [ ] `RUST_LOG=debug backscroll sync` muestra spans de tracing
- [ ] `RUST_LOG=debug backscroll search "test"` muestra spans de tracing

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T018](T018-tracing-subscriber.md) | Configurar tracing-subscriber en main.rs |
| [T019](T019-instrument-spans.md) | Agregar #[instrument] spans a sync y search |

## Fuente de verdad

- `src/main.rs` — subscriber setup
- `src/core/sync.rs` — spans de sync
- `src/storage/sqlite.rs` — spans de search
