# E07: Calidad de Corpus

**Objetivo**: Filtrar ruido, excluir subagent sessions, y asociar cada mensaje con su proyecto. Que el indice contenga solo contenido util para busqueda.

## Postcondiciones

| # | Postcondicion | Features | Verificacion |
|---|---------------|----------|-------------|
| P1 | Solo mensajes user/assistant indexados, ruido excluido | F01 | Search por `<system-reminder>` retorna 0 |
| P2 | Subagent sessions excluidas por defecto | F02 | Sync default no indexa archivos en `/subagents/` |
| P3 | Cada mensaje tiene proyecto asociado non-NULL | F02 | `SELECT count(*) FROM search_items WHERE project IS NULL` = 0 |

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales
- INV2: Sync incremental funciona (archivos sin cambios no se re-procesan)

## Out of Scope

- Semantic search / Tantivy migration
- Output formatting (E08)

## Features

| Feature | Descripcion |
|---------|-------------|
| [F01](F01-filtrado-contenido/) | Filtrado de Contenido |
| [F02](F02-gestion-sesiones-proyecto/) | Gestion de Sesiones y Proyecto |

## Dependencias

Requiere E06 completado. E08-beta depende de E07.
