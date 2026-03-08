---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T026: Filtrar paths con /subagents/ en walkdir

**Story**: [S031 Exclusion de subagent sessions](README.md)
**Contribuye a**: Sync default no indexa archivos en paths con /subagents/

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

El 74.9% del corpus real son subagent sessions ubicadas en `<session-uuid>/subagents/agent-*.jsonl`. Por defecto deben excluirse del sync. La deteccion es por path: si contiene `/subagents/`, se excluye.

IMPORTANTE: NO es por prefix `agent-*` en la raiz. Es por la presencia de `/subagents/` en el path completo.

## Alcance

**In**:
1. En walkdir de parse_sessions, skip archivos cuyo path contiene `/subagents/`
2. Parametro `include_agents: bool` (default false) para override
3. Log con tracing::debug! los archivos skipped

**Out**: No agregar flag CLI (T027).

## Estado inicial esperado

- parse_sessions procesa todos los .jsonl encontrados

## Criterios de Aceptacion

- `cargo test test_subagent_excluded` — archivos en /subagents/ no procesados
- `cargo test test_subagent_included` — con include_agents=true SI procesados
- `just check` pasa

## Fuente de verdad

- `src/core/sync.rs`
