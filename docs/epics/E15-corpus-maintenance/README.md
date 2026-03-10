# E15: Corpus Maintenance

**Metrica de exito**: Operadores pueden reconstruir, podar y validar el indice de backscroll sin intervencion manual en la base de datos
**Timeline**: 2026-Q2 — planificado

## Intencion

A medida que el corpus crece (1000+ sesiones), se necesitan herramientas de mantenimiento: reconstruir el indice cuando cambian los filtros de ruido, purgar sesiones antiguas, y validar la integridad de los datos indexados vs los archivos fuente.

## Postcondiciones

- P1: `backscroll reindex` reconstruye el indice FTS5 desde archivos fuente
- P2: `backscroll purge --before 2025-01-01` elimina entries anteriores a la fecha
- P3: `backscroll validate` reporta problemas de integridad (orphans, inconsistencias FTS5)

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa
- INV3: Operaciones de purge son transaccionales (all-or-nothing)
- INV4: validate es read-only (no modifica datos)

## Out of Scope

- Auto-repair (validate solo reporta, no corrige)
- Scheduled maintenance / cron
- Purge by project (solo por fecha en v1)

## Features

| ID | Nombre | Descripcion |
|----|--------|-------------|
| F01 | [Reindex](F01-reindex/) | Reconstruccion forzada del indice FTS5 |
| F02 | [Purge](F02-purge/) | Eliminacion de datos antiguos con VACUUM |
| F03 | [Validate](F03-validate/) | Validacion de integridad del indice |

## Orden de Ejecucion

| Feature | Depende de | Razon |
|---------|-----------|-------|
| F01 | — | Reindex es independiente y mas urgente |
| F02 | — | Purge es independiente |
| F03 | — | Validate es independiente |

## Decision Log

| Fecha | Decision | Razon |
|-------|----------|-------|

## Gaps Activos

- Ninguno identificado
