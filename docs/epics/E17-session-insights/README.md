# E17: Session Insights

**Metrica de exito**: Usuarios pueden responder "que tipo de trabajo hice esta semana?" y "cuales fueron mis sesiones de debugging?" sin revisar sesiones individualmente
**Timeline**: 2026-Q2 — planificado

## Intencion

Backscroll tiene datos ricos (timestamps, proyectos, contenido) pero solo expone busqueda y listado. Este epic agrega capacidades analiticas: auto-etiquetado de sesiones por tipo de trabajo (debug, refactor, feature) y visualizacion de patrones temporales (actividad por dia, distribucion de topics).

Contexto competitivo: Anthropic lanzo `/insights` (Feb 2026) con analisis HTML interactivo. Backscroll puede ofrecer insights CLI-native con output estructurado (JSON/robot) consumible por LLMs y scripts.

## Postcondiciones

- P1: `backscroll search --tag debugging` filtra a sesiones auto-etiquetadas como debugging
- P2: `backscroll insights` muestra actividad por dia y distribucion de categorias de trabajo
- P3: Tags heuristicos asignados automaticamente durante sync con >70% precision

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa
- INV3: Zero dependencias nuevas — heuristicas son regex sobre texto ya parseado
- INV4: Auto-tagging es aditivo — no modifica search_items existentes
- INV5: Output `--robot`/`--json` consistente con subcomandos existentes

## Out of Scope

- Tags manuales definidos por usuario (v1 solo auto-tags)
- Topic clustering con ML (LDA, BERTopic)
- Dashboard HTML interactivo (solo output texto/JSON)
- Heatmaps visuales

## Features

| ID | Nombre | Descripcion |
|----|--------|-------------|
| F01 | [Auto-Tagging](F01-auto-tagging/) | Categorizar sesiones por tipo de trabajo via heuristicas regex |
| F02 | [Time-Series Analytics](F02-time-series-analytics/) | Comando `insights` con agregaciones temporales |

## Orden de Ejecucion

| Feature | Depende de | Razon |
|---------|-----------|-------|
| F01 | — | Tags son prerequisito para analytics por categoria |
| F02 | F01 | Insights usa tags para distribucion de categorias |

## Decision Log

| Fecha | Decision | Razon |
|-------|----------|-------|
| 2026-03-20 | Heuristicas regex sobre ML | Zero deps, predecible, suficiente para categorias amplias (debug/refactor/feature/test) |
| 2026-03-20 | session_tags table separada | Evita contaminar search_items, permite re-tag sin reindex |

## Gaps Activos

- Definir taxonomia de tags: debug, refactor, feature, test, docs, config — cuales son suficientes?
- Evaluar si insights necesita formato HTML ademas de text/JSON
