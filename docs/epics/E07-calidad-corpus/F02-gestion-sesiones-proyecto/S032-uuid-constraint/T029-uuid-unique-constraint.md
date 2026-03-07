---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T029: UNIQUE constraint en uuid + INSERT OR IGNORE

**Story**: [S032 UUID constraint defensivo](README.md)
**Contribuye a**: UNIQUE constraint en uuid de search_items

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Aunque los datos reales muestran 0 duplicados en 244K UUIDs, un UNIQUE constraint en la columna uuid de search_items es una defensa a nivel de DB. INSERT OR IGNORE previene errores si un duplicado aparece.

## Alcance

**In**:
1. Agregar `UNIQUE` constraint en columna uuid de search_items (en migracion)
2. Cambiar INSERT a `INSERT OR IGNORE` para records con uuid
3. Test que intenta insertar uuid duplicado → no falla, no duplica

**Out**: No cambiar logica de sync.

## Estado inicial esperado

- search_items tiene columna uuid sin constraint unique

## Criterios de Aceptacion

- `sqlite3 test.db ".schema search_items"` muestra UNIQUE en uuid
- INSERT de uuid duplicado no produce error (INSERT OR IGNORE)
- `cargo test test_uuid_dedup` pasa
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
