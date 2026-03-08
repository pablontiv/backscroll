---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T027: Agregar flag --include-agents a sync

**Story**: [S031 Exclusion de subagent sessions](README.md)
**Contribuye a**: sync --include-agents SI indexa subagents

[[blocks:T026-filter-subagent-paths]]

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Agregar flag CLI `--include-agents` al comando sync que pasa `include_agents: true` a parse_sessions.

## Alcance

**In**:
1. Agregar `--include-agents` flag al comando Sync en clap derive
2. Pasar el valor a parse_sessions

**Out**: No agregar flag a otros comandos.

## Estado inicial esperado

- parse_sessions acepta parametro include_agents (T026)
- CLI no tiene el flag

## Criterios de Aceptacion

- `backscroll sync --help` muestra `--include-agents`
- `cargo test` pasa
- `just check` pasa

## Fuente de verdad

- `src/main.rs`
