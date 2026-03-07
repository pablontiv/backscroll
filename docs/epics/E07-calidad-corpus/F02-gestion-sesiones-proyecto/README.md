# F02: Gestion de Sesiones y Proyecto

**Epic**: [E07 Calidad de Corpus](../README.md)
**Objetivo**: Excluir subagent sessions por defecto, agregar UUID constraint defensivo, y detectar proyecto automaticamente.
**Satisface**: P2 (subagents excluidas), P3 (project non-NULL)
**Milestone**: Sync default no indexa `/subagents/`; `SELECT count(*) WHERE project IS NULL` = 0.

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales (heredado de E07)
- INV2: Sync incremental funciona (heredado de E07)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S031](S031-exclusion-subagents/) | Exclusion de subagent sessions |
| [S032](S032-uuid-constraint/) | UUID constraint defensivo |
| [S033](S033-deteccion-proyecto/) | Deteccion automatica de proyecto |
