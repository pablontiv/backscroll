# S053: Per-project breakdown

**Feature**: [F03 Enhanced Status](../README.md)
**Capacidad**: El subcomando `status` existente incluye una seccion adicional mostrando desglose de sesiones y mensajes por proyecto.
**Cubre**: P3 (status incluye breakdown por proyecto)

## Antes / Despues

**Antes**: `backscroll status` muestra totales globales (files indexed, messages, projects). No hay desglose por proyecto.

**Despues**: `backscroll status` incluye tabla con project name, session count, y message count por proyecto.

## Criterios de Aceptacion (semanticos)

- [ ] `backscroll status` muestra seccion "By Project" con tabla
- [ ] Tabla incluye project, sessions, messages por proyecto
- [ ] Proyectos ordenados por session count descendente
- [ ] Totales globales siguen mostrando igual que antes

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T081](T081-status-project-query.md) | Add project breakdown query |
| [T082](T082-status-output-update.md) | Update status output format |
| [T083](T083-status-integration-test.md) | Test enhanced status output |

## Fuente de verdad

- `src/storage/sqlite.rs` — query function
- `src/main.rs` — status command formatting
