# F01: Auto-Tagging

**Epic**: [E17 Session Insights](../README.md)
**Objetivo**: Categorizar sesiones automaticamente por tipo de trabajo usando heuristicas regex sobre contenido indexado
**Satisface**: P1 (--tag filter), P3 (>70% precision en tags)
**Milestone**: `backscroll search --tag debugging` retorna sesiones donde se discutieron bugs/errors

## Invariantes

- INV1: Tags se almacenan en tabla separada `session_tags` (no modifica search_items)
- INV2: Re-tagging es idempotente — sync recalcula tags sin duplicar
- INV3: Sesiones pueden tener multiples tags (e.g., "debugging" + "refactoring")

## Stories

| Story | Descripcion |
|-------|-------------|
| S072 | Tabla session_tags: schema, insert, query |
| S073 | Engine de heuristicas: regex patterns por categoria (debug, refactor, feature, test, docs) |
| S074 | Integracion con sync: auto-tag al indexar sesion |
| S075 | Flag --tag en search: filtro por tag en query |
| S076 | Tests: precision de heuristicas contra corpus real |
