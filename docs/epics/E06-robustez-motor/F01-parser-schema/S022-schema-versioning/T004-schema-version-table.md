---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T004: Crear tabla schema_version + mecanismo de migracion

**Story**: [S022 Schema versioning + content table](README.md)
**Contribuye a**: Existe mecanismo de migracion de schema

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

No existe mecanismo de migracion de schema. Si se cambia el DDL, las bases de datos existentes quedan incompatibles. Se necesita una tabla `schema_version` y un patron de migracion secuencial.

## Alcance

**In**:
1. Crear tabla `schema_version (version INTEGER NOT NULL)` en `ensure_table`
2. Insertar version 1 si la tabla esta vacia (migracion inicial)
3. Funcion `migrate()` que ejecuta migraciones secuenciales segun version actual
4. Migracion v1→v2: sera implementada en T005/T006 (este task solo crea la infraestructura)

**Out**: No crear las tablas nuevas (search_items, FTS5 external) — eso es T005/T006.

## Estado inicial esperado

- `src/storage/sqlite.rs` tiene `ensure_table()` que crea `indexed_files` y `messages_fts`
- No existe tabla schema_version

## Criterios de Aceptacion

- `cargo test test_schema_version` pasa — tabla creada con version inicial
- Migracion es idempotente (ejecutar 2 veces no falla)
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
