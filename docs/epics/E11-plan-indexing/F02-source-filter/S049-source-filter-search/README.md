# S049: Source filter in search

**Feature**: [F02 Source Filter](../README.md)
**Capacidad**: `--source sessions|plans|all` filtra resultados de busqueda por la columna `source`. Default es `all`.
**Cubre**: P3 (--source plans retorna solo plans), P4 (default preserva comportamiento)

[[blocks:S048-plan-sync-pipeline]]

## Antes / Despues

**Antes**: Search no tiene filtro de source. Todos los resultados son sessions.

**Despues**: `--source sessions` retorna solo sessions, `--source plans` retorna solo plans, `--source all` (default) retorna ambos. Resume command tambien gana soporte `--source`.

## Criterios de Aceptacion (semanticos)

- [ ] `--source plans` retorna solo plans
- [ ] `--source sessions` retorna solo sessions
- [ ] `--source all` (default) retorna ambos
- [ ] Resume command soporta `--source`

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Busqueda sin --source incluye todo
  - Verificar: `backscroll search "test"` retorna sessions y plans

## Tasks

| Task | Descripcion |
|------|-------------|
| [T067](T067-source-flag-search.md) | Add --source flag to Search |
| [T068](T068-source-filter-sql.md) | Source filter in SQL |
| [T069](T069-source-filter-tests.md) | Source filter tests |
| [T070](T070-resume-source-support.md) | Resume source support |

## Fuente de verdad

- `src/main.rs` — Search subcommand args
- `src/core/mod.rs` — SearchEngine::search() signature
- `src/storage/sqlite.rs` — SQL WHERE clause
