# F02: FTS5 Optimization

**Epic**: [E16 Search Quality](../README.md)
**Objetivo**: Proveer mecanismo para defragmentar el indice FTS5 y recuperar performance de queries
**Satisface**: P2 (sync --optimize)
**Milestone**: `backscroll sync --optimize` ejecuta FTS5 OPTIMIZE y reporta tiempo transcurrido

## Invariantes

- INV1: OPTIMIZE es opcional — sync normal no lo ejecuta
- INV2: Warning al usuario si DB > 100MB (OPTIMIZE puede tardar)
- INV3: DB queda en estado consistente si se interrumpe

## Stories

| Story | Descripcion |
|-------|-------------|
| S066 | Flag --optimize en sync: ejecutar `INSERT INTO messages_fts(messages_fts) VALUES('optimize')` |
| S067 | Tests: verificar que OPTIMIZE no corrompe indice |
