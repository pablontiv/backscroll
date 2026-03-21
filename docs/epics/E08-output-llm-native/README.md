# E08: Output LLM-Native

**Objetivo**: Enriquecer output de busqueda con snippets, scores, fechas. Agregar modos de salida estructurada (JSON, robot) y flags de control. Implementar status con metricas reales.
**Timeline**: 2026-Q1 — hecho

## Postcondiciones

| # | Postcondicion | Features | Verificacion |
|---|---------------|----------|-------------|
| P1 | Output incluye snippet con highlight, score, fecha, slug | F01 | `backscroll search "test"` muestra formato enriquecido |
| P2 | `--json` produce output parseable | F02 | `backscroll search "test" --json \| jq .` no falla |
| P3 | `status` muestra metricas reales del indice | F03 | `backscroll status` muestra conteo de archivos y mensajes |

## Invariantes

- INV1: Busqueda sin flags produce output legible por humanos
- INV2: Performance < 1s en corpus de test

## Out of Scope

- `--topics` mode (diferido post-v1)
- Semantic search
- Cross-platform builds

## Features

| Feature | Descripcion |
|---------|-------------|
| [F01](F01-search-enriquecido/) | Search Enriquecido (E08-alpha: paralelo con E07) |
| [F02](F02-cli-avanzado/) | CLI Avanzado (E08-alpha: paralelo con E07) |
| [F03](F03-status-documentacion/) | Status y Documentacion (E08-beta: necesita E07) |

## Dependencias

F01 y F02 requieren E06. F03 requiere E07 completado.
