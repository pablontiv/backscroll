# S030: Filtrado de patrones de ruido

**Feature**: [F01 Filtrado de Contenido](../README.md)
**Capacidad**: Contenido de ruido (system-reminder, task-notification, tool_use blocks, etc.) se filtra antes de indexar.
**Cubre**: P1 del Epic (ruido excluido)

[[blocks:S029-prefiltrado-record-type]]

## Antes / Despues

**Antes**: Todo el contenido textual de mensajes user/assistant se indexa sin filtrar. Busquedas retornan ruido: system-reminder tags, task-notification blocks, command XML, caveat blocks, tool_use/tool_result blocks.

**Despues**: 8+ filtros de ruido implementados: `<system-reminder>`, `<task-notification>`, `<caveat>`, command XML, `<local-command-caveat>`/stdout, "Request interrupted", "Base directory", y skip de tool_use/tool_result content blocks. Cada filtro tiene test unitario con fixture.

## Criterios de Aceptacion (semanticos)

- [x] Search por `<system-reminder>` retorna 0 resultados en corpus filtrado
- [x] Search por `<task-notification>` retorna 0 resultados
- [x] tool_use/tool_result blocks no se indexan como texto

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T024](T024-noise-filters.md) | Implementar 8+ filtros de ruido |
| [T025](T025-test-noise-patterns.md) | Tests unitarios para cada patron con fixtures |

## Fuente de verdad

- `src/core/sync.rs` — logica de filtrado
- Documentacion de patrones en `docs/research/backscroll-session-search-cli.md` seccion "Noise Patterns"
