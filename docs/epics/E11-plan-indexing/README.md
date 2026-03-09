# E11: Plan Indexing

**Objetivo**: Indexar `~/.claude/plans/*.md` en la tabla FTS5 existente con `source='plan'`, spliteando por `##` headers, y agregar filtro `--source sessions|plans|all` a la busqueda.

## Postcondiciones

| # | Postcondicion | Features | Verificacion |
|---|---------------|----------|-------------|
| P1 | Plans indexados en `search_items` con `source='plan'` | F01 | `SELECT count(*) FROM search_items WHERE source = 'plan'` > 0 despues de sync |
| P2 | Plans spliteados por `##` headers en secciones individuales | F01 | count(plan rows) > count(plan files) |
| P3 | `--source plans` filtra busqueda a solo contenido de plans | F02 | `backscroll search "test" --source plans` retorna solo plan results |
| P4 | `--source sessions` preserva comportamiento actual (default) | F02 | `backscroll search "test"` se comporta igual que pre-E11 |

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa (clippy nursery+pedantic, -D warnings)
- INV3: Busqueda de sessions sin cambios cuando `--source` no se especifica
- INV4: Sync incremental funciona para plans (plans sin cambios se omiten)

## Out of Scope

- Asociacion plan-a-proyecto (M7). Plans son globales, no project-scoped. La columna `project` sera NULL para plans.
- Busqueda semantica sobre plans
- Extraccion de metadata de plans (frontmatter, status fields)

## Features

| Feature | Descripcion |
|---------|-------------|
| [F01](F01-plan-parser-sync/) | Plan Parser & Sync |
| [F02](F02-source-filter/) | Source Filter |

## Dependencias

Soft: despues de E10 (auto-discovery mejora sync general).
