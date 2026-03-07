# S028: Limpieza tecnica

**Feature**: [F03 Instrumentacion y Limpieza](../README.md)
**Capacidad**: Codebase limpio sin legacy artifacts. rust-version en Cargo.toml. Licencia MIT.
**Cubre**: P5 del Epic (zero dead code)

## Antes / Despues

**Antes**: Puede haber snapshots duplicados legacy, falta `rust-version` en Cargo.toml, no hay archivo LICENSE.

**Despues**: Sin legacy artifacts. `rust-version` declarado en Cargo.toml. Archivo LICENSE (MIT) presente.

## Criterios de Aceptacion (semanticos)

- [ ] rust-version esta en Cargo.toml
- [ ] LICENSE file existe en la raiz del repo

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T020](T020-cleanup-legacy.md) | Eliminar legacy artifacts, agregar rust-version |
| [T021](T021-license-mit.md) | Crear archivo LICENSE (MIT) |

## Fuente de verdad

- `Cargo.toml` — metadata del proyecto
- Raiz del repositorio — LICENSE file
