# F01: Query Filters

**Epic**: [E14 Search Refinement](../README.md)
**Objetivo**: Search acepta filtros por fecha y rol que reducen resultados irrelevantes
**Satisface**: P1 (date filter), P2 (role filter)
**Milestone**: `backscroll search "query" --after 2026-03-01 --role human` retorna solo mensajes humanos posteriores a la fecha

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E14)
- INV2: `just check` pasa (heredado de E14)
- INV3: Backward compatible (heredado de E14)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S057](S057-date-range-filter/) | Date Range Filter |
| [S058](S058-role-filter/) | Role Filter |
