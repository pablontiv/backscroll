# F01: Filtrado de Contenido

**Epic**: [E07 Calidad de Corpus](../README.md)
**Objetivo**: Filtrar records por type y patrones de ruido para que solo contenido util llegue al indice.
**Satisface**: P1 (solo user/assistant indexados, ruido excluido)
**Milestone**: `backscroll search "<system-reminder>"` retorna 0 resultados en corpus real.

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales (heredado de E07)
- INV2: Sync incremental funciona (heredado de E07)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S029](S029-prefiltrado-record-type/) | Pre-filtrado por record type |
| [S030](S030-filtrado-ruido/) | Filtrado de patrones de ruido |
