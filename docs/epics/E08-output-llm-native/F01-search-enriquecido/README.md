# F01: Search Enriquecido

**Epic**: [E08 Output LLM-Native](../README.md)
**Objetivo**: Extraer snippets FTS5 con highlight y popular SearchResult con metadata completa.
**Satisface**: P1 (output con snippet, highlight, score, fecha, slug)
**Milestone**: `backscroll search "test"` muestra formato enriquecido con snippets.
**Fase**: E08-alpha (puede ejecutarse en paralelo con E07, solo necesita E06).

## Invariantes

- INV1: Busqueda sin flags produce output legible (heredado de E08)
- INV2: Performance < 1s en corpus de test (heredado de E08)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S034](S034-fts5-snippet/) | FTS5 snippet extraction |
| [S035](S035-output-format/) | Output format enriquecido |
