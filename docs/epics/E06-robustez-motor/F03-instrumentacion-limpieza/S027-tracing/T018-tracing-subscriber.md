---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T018: Configurar tracing-subscriber en main.rs

**Story**: [S027 Tracing](README.md)
**Contribuye a**: RUST_LOG=debug backscroll sync muestra spans de tracing

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Los crates `tracing` y `tracing-subscriber` estan en Cargo.toml pero no se inicializan. Se necesita configurar el subscriber en main() con `EnvFilter` para control via `RUST_LOG`.

## Alcance

**In**:
1. Inicializar `tracing_subscriber` con `fmt()` y `EnvFilter::from_default_env()` en main()
2. Default level: warn (silencioso por defecto)
3. Activable con `RUST_LOG=debug` o `RUST_LOG=backscroll=debug`

**Out**: No agregar spans (T019).

## Estado inicial esperado

- tracing y tracing-subscriber en Cargo.toml
- No hay inicializacion de subscriber en main.rs

## Criterios de Aceptacion

- `RUST_LOG=debug backscroll status` produce output de tracing en stderr
- Sin RUST_LOG, output es limpio (sin logs de tracing)
- `just check` pasa

## Fuente de verdad

- `src/main.rs`
- `Cargo.toml`
