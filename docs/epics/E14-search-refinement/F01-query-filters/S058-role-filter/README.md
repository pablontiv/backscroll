# S058: Role Filter

**Feature**: [F01 Query Filters](../README.md)
**Capacidad**: Search filtra resultados por rol (human o assistant)
**Cubre**: P2 (role filtering)

## Antes / Despues

**Antes**: Search retorna mensajes de ambos roles mezclados. El skill no puede buscar solo en preguntas del usuario o solo en respuestas de Claude.

**Despues**: `backscroll search "query" --role human` retorna solo mensajes del usuario. `--role assistant` retorna solo respuestas de Claude. La columna `role` ya existe en search_items — solo falta el filtro.

## Criterios de Aceptacion (semanticos)

- [ ] --role human filtra a solo mensajes con role='user'
- [ ] --role assistant filtra a solo mensajes con role='assistant'
- [ ] Sin --role, retorna ambos (backward compatible)

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T095](T095-role-flag-where-clause.md) | Add --role flag and implement role WHERE clause |
| [T096](T096-role-filter-tests.md) | Integration tests for role filtering |

## Fuente de verdad

- `src/main.rs` — Search command struct
- `src/storage/sqlite.rs` — search() WHERE clause construction
