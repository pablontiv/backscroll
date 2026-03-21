# F03: Content-Type Filter

**Epic**: [E16 Search Quality](../README.md)
**Objetivo**: Clasificar mensajes por tipo de contenido (text, code, tool) durante sync y permitir filtrado en busqueda
**Satisface**: P3 (--content-type filter)
**Milestone**: `backscroll search "query" --content-type code` retorna solo mensajes con bloques de codigo

## Invariantes

- INV1: Mensajes con contenido mixto (text + code) se clasifican por contenido predominante
- INV2: Nueva columna `content_type` en search_items, parte de schema v4 migration
- INV3: Default sin flag es "all" (backward compatible)

## Stories

| Story | Descripcion |
|-------|-------------|
| S068 | Clasificador de contenido: detectar text/code/tool en MessageContent::Blocks |
| S069 | Columna content_type en search_items + schema migration |
| S070 | Flag --content-type en search con filtro WHERE |
| S071 | Tests: clasificacion correcta de contenido mixto |
