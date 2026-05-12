---
estado: Completed
tipo: task
---
# T006: Validar con just check y just test

**Outcome**: [Migrar setup_schema a refinery](README.md)

## Preserva

- Ningún test existente falla.
- No se introducen nuevos clippy warnings.

## Contexto

El proyecto tiene clippy nursery + pedantic activos con -D warnings. Es importante correr just check antes de just test para detectar issues de formato y linting que bloquearían CI.

## Alcance

**In**:
1. Correr just check y just test.
2. Corregir cualquier issue de formato, clippy o compilación que aparezca.

**Out**:
1. Cambios funcionales adicionales.
2. Actualizar snapshots de insta (a menos que sean necesarios).

## Estado inicial esperado

Implementación completa de T001-T005 sin validación final.

## Criterios de Aceptación

- just check sale con código 0.
- just test sale con código 0.
- No hay warnings de clippy ni errores de rustfmt.

## Fuente de verdad

- Justfile
- src/storage/migrations.rs
- src/storage/sqlite.rs
- Cargo.toml
