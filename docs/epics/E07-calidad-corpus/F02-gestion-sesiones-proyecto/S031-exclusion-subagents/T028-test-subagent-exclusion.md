---
estado: Completed
tipo: test
ejecutable_en: 1 sesion
---
# T028: Test de exclusion/inclusion de subagents

**Story**: [S031 Exclusion de subagent sessions](README.md)
**Contribuye a**: Sync default no indexa subagents; --include-agents si

[[blocks:T027-include-agents-flag]]

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Integration test end-to-end que verifica el comportamiento de exclusion/inclusion de subagent sessions.

## Alcance

**In**:
1. Crear fixture con estructura: `session/main.jsonl` + `session/subagents/agent-1.jsonl`
2. Test default sync: solo main.jsonl indexado
3. Test con --include-agents: ambos indexados
4. Test CLI integration si posible

**Out**: No agregar funcionalidad nueva.

## Estado inicial esperado

- Filtrado de subagents implementado (T026)
- Flag CLI implementado (T027)

## Criterios de Aceptacion

- `cargo test test_subagent_integration` pasa
- Test verifica conteo de mensajes con y sin --include-agents

## Fuente de verdad

- `tests/cli.rs`
