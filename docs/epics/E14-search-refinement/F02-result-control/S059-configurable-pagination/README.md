# S059: Configurable Pagination

**Feature**: [F02 Result Control](../README.md)
**Capacidad**: Search acepta --limit y --offset para controlar cantidad y paginacion de resultados
**Cubre**: P3 (pagination)

## Antes / Despues

**Antes**: Search tiene `LIMIT 20` hard-coded en la query SQL (sqlite.rs:260). No se puede obtener mas de 20 resultados ni paginar. Para corpus grandes, los mejores matches pueden quedar fuera.

**Despues**: `--limit N` controla cuantos resultados (default 20 para backward compat). `--offset M` permite saltar los primeros M resultados. El SQL usa parametros en lugar de constantes.

## Criterios de Aceptacion (semanticos)

- [ ] --limit 50 retorna hasta 50 resultados
- [ ] --offset 20 salta los primeros 20 resultados
- [ ] Sin flags, LIMIT es 20 (backward compatible)
- [ ] --limit 0 retorna todos los resultados (sin limite)

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T097](T097-limit-offset-flags.md) | Replace LIMIT 20 with --limit/--offset flags |
| [T098](T098-pagination-tests.md) | Integration tests for pagination |

## Fuente de verdad

- `src/main.rs` — Search command struct
- `src/storage/sqlite.rs:260` — hard-coded LIMIT 20
