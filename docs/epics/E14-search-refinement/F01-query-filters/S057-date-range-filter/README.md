# S057: Date Range Filter

**Feature**: [F01 Query Filters](../README.md)
**Capacidad**: Search filtra resultados por rango de fecha usando columna timestamp existente
**Cubre**: P1 (date range filtering)

## Antes / Despues

**Antes**: `backscroll search "query"` busca en todo el corpus historico. No hay forma de limitar a un rango temporal. El skill `/backscroll` retorna resultados de hace meses mezclados con los recientes.

**Despues**: `backscroll search "query" --after 2026-03-01 --before 2026-03-09` filtra por timestamp. Solo retorna resultados dentro del rango. Reduccion significativa de tokens irrelevantes para queries recientes.

## Criterios de Aceptacion (semanticos)

- [ ] --after filtra resultados con timestamp >= valor
- [ ] --before filtra resultados con timestamp < valor
- [ ] Ambos flags pueden usarse juntos o por separado
- [ ] Sin flags temporales, comportamiento es identico al actual

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T092](T092-date-flags-trait.md) | Add --after/--before CLI flags and extend SearchEngine trait |
| [T093](T093-timestamp-where-clause.md) | Implement timestamp WHERE clause in SQLite search() |
| [T094](T094-date-filter-tests.md) | Integration tests for date range filtering |

## Fuente de verdad

- `src/main.rs` — Search command struct (lines 40-65)
- `src/core/mod.rs` — SearchEngine trait (lines 65-83)
- `src/storage/sqlite.rs` — search() implementation (lines 225-280)
