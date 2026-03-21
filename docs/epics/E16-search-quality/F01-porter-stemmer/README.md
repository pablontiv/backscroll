# F01: Porter Stemmer

**Epic**: [E16 Search Quality](../README.md)
**Objetivo**: Activar el tokenizer Porter en la tabla FTS5 para que busquedas por raiz morfologica funcionen automaticamente
**Satisface**: P1 (busqueda por variaciones morfologicas)
**Milestone**: `backscroll search "error"` retorna resultados con "errors", "errored", "erroring"

## Invariantes

- INV1: Schema migration v3 → v4 es automatica al abrir DB
- INV2: Reindex automatico post-migration (cambio de tokenizer invalida indice existente)
- INV3: Precision de busqueda no degrada significativamente (monitor over-matching)

## Stories

| Story | Descripcion |
|-------|-------------|
| S064 | Schema migration: agregar version 4 con tokenizer Porter |
| S065 | Auto-reindex: detectar cambio de tokenizer y forzar rebuild |
