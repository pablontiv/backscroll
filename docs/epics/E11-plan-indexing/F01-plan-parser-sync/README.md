# F01: Plan Parser & Sync

**Epic**: [E11 Plan Indexing](../README.md)
**Objetivo**: Parsear `~/.claude/plans/*.md`, splitear contenido por `##` headers, e insertar en `search_items` con `source='plan'`.
**Satisface**: P1 (plans indexados), P2 (plans spliteados por headers)
**Milestone**: `backscroll sync` indexa plans y sessions. Secciones de plans aparecen en `search_items`.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E11)
- INV2: `just check` pasa (heredado de E11)
- INV4: Sync incremental funciona para plans (heredado de E11)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S047](S047-markdown-plan-parser/) | Markdown plan parser |
| [S048](S048-plan-sync-pipeline/) | Plan sync pipeline |
