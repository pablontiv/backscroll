---
estado: Specified
tipo: task
---
# T005: Agregar tests: fresh DB, legacy DB v7, idempotencia

**Outcome**: [Migrar setup_schema a refinery](README.md)

## Preserva

- Tests existentes no se modifican.
- La suite de tests sigue pasando completamente.

## Contexto

El caso legacy (test 2) es el más crítico: valida que la transición de DBs existentes es transparente. Usar tempfile para crear DBs temporales en tests. Para el test 2, crear la tabla schema_version con version=7, insertar datos en search_items, correr migrations::run(), y verificar el estado final.

## Alcance

**In**:
1. Test test_fresh_db_migration: DB nueva, V1 aplicada, schema correcto.
2. Test test_legacy_db_v7_migration: DB con schema_version=7, transición correcta, datos intactos.
3. Test test_migration_idempotent: run() dos veces, sin error, 1 row en refinery_schema_history.

**Out**:
1. Tests de comportamiento funcional de la DB (búsqueda, sync, etc.).
2. Tests de performance.

## Estado inicial esperado

No existen tests específicos para el módulo de migraciones ni para la transición legacy.

## Criterios de Aceptación

- Los 3 tests pasan con cargo test.
- test_legacy_db_v7_migration verifica explícitamente que schema_version no existe post-migración.
- test_legacy_db_v7_migration verifica que datos pre-existentes en search_items sobreviven.

## Fuente de verdad

- src/storage/migrations.rs
- src/storage/sqlite.rs
