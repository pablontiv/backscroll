---
estado: Pending
tipo: chore
ejecutable_en: 1 sesion
---
# T020: Eliminar legacy artifacts, agregar rust-version

**Story**: [S028 Limpieza tecnica](README.md)
**Contribuye a**: rust-version esta en Cargo.toml

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Limpieza general del proyecto: eliminar snapshots duplicados legacy si existen, agregar `rust-version` a Cargo.toml para documentar MSRV.

## Alcance

**In**:
1. Agregar `rust-version = "1.85"` (o version correcta para edition 2024) a [package] en Cargo.toml
2. Eliminar archivos legacy si existen (snapshots duplicados, archivos huerfanos)
3. Verificar que no hay archivos sin usar en src/

**Out**: No agregar funcionalidad. No modificar CI.

## Estado inicial esperado

- Cargo.toml sin campo rust-version

## Criterios de Aceptacion

- `grep "rust-version" Cargo.toml` encuentra la version
- `just check` pasa
- No hay archivos sin usar en src/

## Fuente de verdad

- `Cargo.toml`
