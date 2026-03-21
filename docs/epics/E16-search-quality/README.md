# E16: Search Quality

**Metrica de exito**: Busquedas con variaciones morfologicas (plurales, conjugaciones) retornan resultados relevantes sin necesidad de wildcards manuales
**Timeline**: 2026-Q2 — planificado

## Intencion

FTS5 con tokenizer default (`unicode61`) requiere match exacto de palabras. "error" no encuentra "errors", "run" no encuentra "running". Esto obliga a usuarios a usar wildcards (`error*`) o repetir variantes. Este epic mejora la calidad de busqueda con stemming, optimizacion del indice FTS5, y filtrado por tipo de contenido.

## Postcondiciones

- P1: Busqueda por "error" retorna resultados que contienen "errors", "errored", etc.
- P2: `backscroll sync --optimize` ejecuta FTS5 OPTIMIZE para defragmentar el indice
- P3: `backscroll search "query" --content-type code` filtra resultados a mensajes con bloques de codigo

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa (clippy nursery+pedantic, -D warnings)
- INV3: Migration de schema es automatica y transparente (v3 → v4)
- INV4: Reindex automatico despues de migration de tokenizer

## Out of Scope

- Busqueda semantica / embeddings (requiere investigacion separada)
- Stemming multi-idioma (solo Porter English en v1)
- Custom tokenizers definidos por usuario

## Features

| ID | Nombre | Descripcion |
|----|--------|-------------|
| F01 | [Porter Stemmer](F01-porter-stemmer/) | Activar tokenizer Porter en FTS5 para stemming automatico |
| F02 | [FTS5 Optimization](F02-fts5-optimization/) | Flag --optimize para defragmentar indice FTS5 |
| F03 | [Content-Type Filter](F03-content-type-filter/) | Clasificar e indexar tipo de contenido (text/code/tool), filtro --content-type |

## Orden de Ejecucion

| Feature | Depende de | Razon |
|---------|-----------|-------|
| F01 | — | Stemmer requiere schema migration que F03 tambien necesita — hacerlas juntas |
| F02 | — | Independiente, puro SQL |
| F03 | F01 | Comparte migration de schema |

## Decision Log

| Fecha | Decision | Razon |
|-------|----------|-------|
| 2026-03-20 | Porter stemmer sobre ICU tokenizer | Porter es built-in en SQLite FTS5, zero deps. ICU requiere libicu |
| 2026-03-20 | Content-type como columna indexada, no FTS field | Filtro discreto (code/text/tool), no necesita ranking por relevancia |

## Gaps Activos

- Evaluar impacto de stemming en precision (over-matching: "universe" ↔ "university")
- Decidir si --optimize va en sync o como subcommand separado
