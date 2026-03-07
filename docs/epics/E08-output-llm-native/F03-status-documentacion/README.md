# F03: Status y Documentacion

**Epic**: [E08 Output LLM-Native](../README.md)
**Objetivo**: Status con metricas reales del indice y documentacion actualizada.
**Satisface**: P3 (status muestra metricas reales)
**Milestone**: `backscroll status` muestra conteo de archivos, mensajes, tamano de DB.
**Fase**: E08-beta (necesita E07 completado para metricas con noise filtering).

## Invariantes

- INV1: Busqueda sin flags produce output legible (heredado de E08)
- INV2: Performance < 1s en corpus de test (heredado de E08)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S038](S038-metricas-indice/) | Metricas reales del indice |
| [S039](S039-actualizacion-docs/) | Actualizacion de documentacion |
