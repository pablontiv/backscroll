# F02: Source Filter

**Epic**: [E11 Plan Indexing](../README.md)
**Objetivo**: Agregar flag `--source` al comando search para filtrar por fuente de contenido (sessions, plans, o all).
**Satisface**: P3 (--source plans filtra solo plans), P4 (default preserva comportamiento actual)
**Milestone**: `backscroll search "test" --source plans` retorna solo resultados de plans.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E11)
- INV2: `just check` pasa (heredado de E11)
- INV3: Busqueda de sessions sin cambios sin --source (heredado de E11)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S049](S049-source-filter-search/) | Source filter in search |
