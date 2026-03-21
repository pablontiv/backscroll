# F01: Export Formats

**Epic**: [E18 Integration](../README.md)
**Objetivo**: Exportar resultados de busqueda a formatos consumibles por herramientas externas (Markdown, CSV)
**Satisface**: P1 (Markdown export), P2 (CSV export)
**Milestone**: `backscroll export --format markdown "query"` produce archivo .md con resultados formateados

## Invariantes

- INV1: Export reutiliza SearchEngine::search() — no duplica logica de busqueda
- INV2: Export respeta todos los filtros existentes (--project, --source, --after, --before, --role)
- INV3: CSV usa headers estandar, compatible con Excel/Sheets/pandas

## Stories

| Story | Descripcion |
|-------|-------------|
| S081 | Formatter Markdown: resultados como secciones con metadata |
| S082 | Formatter CSV: headers + rows, escape correcto de comillas |
| S083 | Subcomando `export` con --format flag |
| S084 | Tests: output correcto para ambos formatos |
