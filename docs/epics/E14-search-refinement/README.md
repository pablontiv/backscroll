# E14: Search Refinement

**Metrica de exito**: Search queries filtran por fecha, rol y paginan resultados — reduciendo tokens consumidos por skills en >50%
**Timeline**: 2026-Q1 — hecho

## Intencion

El skill `/backscroll` hace queries frecuentes como "que discutimos la semana pasada sobre X" o "que me respondio Claude sobre Y". Hoy no hay filtros temporales ni de rol — se busca en todo el corpus y se trunca a 20 resultados. Este epic agrega filtros que reducen el volumen de resultados irrelevantes.

## Postcondiciones

- P1: `backscroll search "query" --after 2026-03-01 --before 2026-03-09` filtra por rango temporal
- P2: `backscroll search "query" --role human` filtra por rol (human|assistant)
- P3: `backscroll search "query" --limit 50 --offset 20` soporta paginacion configurable
- P4: SearchResult incluye timestamp y role en todos los formatos de output (text, json, robot)

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa
- INV3: Comportamiento default sin flags nuevos es identico al actual (backward compatible)
- INV4: `--limit` default = 20 (preserva comportamiento actual)

## Out of Scope

- Busqueda semantica / embeddings
- Sort order configurable (siempre BM25 rank)
- Cursor-based pagination

## Features

| ID | Nombre | Descripcion |
|----|--------|-------------|
| F01 | [Query Filters](F01-query-filters/) | Filtrado por fecha y rol en search |
| F02 | [Result Control](F02-result-control/) | Paginacion configurable y resultados enriquecidos |

## Orden de Ejecucion

| Feature | Depende de | Razon |
|---------|-----------|-------|
| F01 | — | Filters son independientes y mas impactantes |
| F02 | F01 | Pagination y result enrichment complementan filters |

## Decision Log

| Fecha | Decision | Razon |
|-------|----------|-------|
| 2026-03-20 | Todas las postcondiciones verificadas — epic completado | --after, --before, --role, --limit, --offset implementados y testeados |

## Gaps Activos

- Ninguno
