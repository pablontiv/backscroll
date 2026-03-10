# E12: Session Analytics

**Objetivo**: Dar a backscroll la capacidad de responder queries analiticas sobre el corpus de sesiones — frecuencia de terminos (topics), listado de sesiones, y desglose por proyecto — usando datos que ya existen en el indice FTS5.

## Postcondiciones

| # | Postcondicion | Features | Verificacion |
|---|---------------|----------|-------------|
| P1 | `backscroll topics` retorna terminos rankeados por frecuencia desde fts5vocab | F01 | `backscroll topics --robot` retorna lineas tab-separated con term, sessions, mentions |
| P2 | `backscroll list` retorna sesiones con metadata ordenadas por recencia | F02 | `backscroll list --recent 5 --robot` retorna 5 sesiones con path, project, messages, timestamps |
| P3 | `backscroll status` incluye desglose de sesiones por proyecto | F03 | `backscroll status` muestra tabla con project, sessions, messages |

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa (clippy nursery+pedantic, -D warnings)
- INV3: Zero dependencias nuevas — todo es SQL sobre SQLite/FTS5 existente
- INV4: Output `--robot`/`--json` consistente con subcomandos existentes (search, resume)
- INV5: Auto-sync antes de cada operacion analitica (igual que search)

## Out of Scope

- Busqueda semantica / embeddings
- Topic clustering con ML (LDA, BERTopic)
- TUI / dashboard interactivo
- Analytics temporales (heatmaps, tendencias)

## Features

| Feature | Descripcion |
|---------|-------------|
| [F01](F01-topic-discovery/) | Topic Discovery via fts5vocab |
| [F02](F02-session-listing/) | Session Listing |
| [F03](F03-enhanced-status/) | Enhanced Status |

## Dependencias

Soft: despues de E11 (plan indexing enriquece el corpus disponible para analytics).
