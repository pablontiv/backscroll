# S031: Exclusion de subagent sessions

**Feature**: [F02 Gestion de Sesiones y Proyecto](../README.md)
**Capacidad**: Subagent sessions se excluyen del sync por defecto, con flag para incluirlas.
**Cubre**: P2 del Epic (subagents excluidas por defecto)

## Antes / Despues

**Antes**: Sync indexa todas las sesiones incluyendo subagents (74.9% del corpus real). Subagent sessions estan en paths como `<session-uuid>/subagents/agent-*.jsonl` — NO como archivos con prefix `agent-*` en la raiz.

**Despues**: Sync excluye paths que contienen `/subagents/` en el walkdir por defecto. Flag `--include-agents` permite incluirlas cuando se desee.

## Criterios de Aceptacion (semanticos)

- [ ] Sync default no indexa archivos en paths con `/subagents/`
- [ ] `sync --include-agents` SI los indexa

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T026](T026-filter-subagent-paths.md) | Filtrar paths con /subagents/ en walkdir |
| [T027](T027-include-agents-flag.md) | Agregar flag --include-agents a sync |
| [T028](T028-test-subagent-exclusion.md) | Test de exclusion/inclusion de subagents |

## Fuente de verdad

- `src/core/sync.rs` — walkdir y filtrado de paths
- `src/main.rs` — CLI flags
