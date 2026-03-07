---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T019: Agregar #[instrument] spans a sync y search

**Story**: [S027 Tracing](README.md)
**Contribuye a**: RUST_LOG=debug muestra spans en sync y search

[[blocks:T018-tracing-subscriber]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Con el subscriber configurado, agregar `#[instrument]` a las funciones clave para visibilidad de ejecucion: sync (cuantos archivos, cuanto tarda), search (query, resultados).

## Alcance

**In**:
1. `#[instrument]` en `parse_sessions` (sync.rs) — skip large params
2. `#[instrument]` en `search` impl de Database (sqlite.rs)
3. `tracing::info!` para conteos significativos (archivos procesados, resultados encontrados)
4. `tracing::warn!` para records que no se pueden parsear

**Out**: No instrumentar todas las funciones — solo las criticas.

## Estado inicial esperado

- tracing-subscriber inicializado (T018)
- Funciones sin instrumentacion

## Criterios de Aceptacion

- `RUST_LOG=debug backscroll sync --path /tmp/test` muestra span de sync con conteos
- `RUST_LOG=debug backscroll search "test"` muestra span de search
- `just check` pasa

## Fuente de verdad

- `src/core/sync.rs`
- `src/storage/sqlite.rs`
