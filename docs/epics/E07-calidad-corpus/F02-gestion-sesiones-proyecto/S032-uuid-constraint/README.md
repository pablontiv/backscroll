# S032: UUID constraint defensivo

**Feature**: [F02 Gestion de Sesiones y Proyecto](../README.md)
**Capacidad**: UNIQUE constraint en uuid de search_items previene duplicados a nivel de base de datos.
**Cubre**: Complementa P4 de E06 (defensa adicional contra duplicados)

## Antes / Despues

**Antes**: No hay constraint de unicidad en uuid. Aunque los datos reales muestran 0 duplicados en 244K UUIDs, no hay proteccion a nivel de DB.

**Despues**: UNIQUE constraint en columna uuid de search_items. INSERT OR IGNORE para records con uuid duplicado.

## Criterios de Aceptacion (semanticos)

- [ ] `sqlite3 db ".schema search_items"` muestra UNIQUE en uuid
- [ ] INSERT de uuid duplicado no falla (INSERT OR IGNORE)

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T029](T029-uuid-unique-constraint.md) | UNIQUE constraint en uuid + INSERT OR IGNORE |

## Fuente de verdad

- `src/storage/sqlite.rs` — schema DDL
