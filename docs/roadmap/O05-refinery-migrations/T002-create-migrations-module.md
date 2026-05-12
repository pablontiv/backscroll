---
estado: Specified
tipo: task
---
# T002: Crear src/storage/migrations.rs con SQL_V1 y run()

**Outcome**: [Migrar setup_schema a refinery](README.md)

## Preserva

- El schema resultante es idéntico al actual v7 para DBs nuevas.
- DBs existentes no pierden datos — solo se elimina la tabla de tracking schema_version.

## Contexto

sqlite-vec está registrado como auto-extension antes de open(), así que CREATE VIRTUAL TABLE vec_embeddings funciona desde el módulo de migraciones. SQL_V1 debe usar IF NOT EXISTS en todas las sentencias para ser seguro en DBs existentes.

## Alcance

**In**:
1. src/storage/migrations.rs con SQL_V1, drop_legacy_tracking(), y run().
2. Declaración pub(crate) mod migrations en src/storage/sqlite.rs o src/storage/mod.rs.

**Out**:
1. Modificaciones a sqlite.rs más allá de la declaración del módulo.
2. Migraciones adicionales más allá de V1.

## Estado inicial esperado

No existe src/storage/migrations.rs. La lógica de migraciones está embebida en setup_schema().

## Criterios de Aceptación

- src/storage/migrations.rs existe y compila.
- run() llamado en DB nueva crea el schema v7 completo.
- run() llamado en DB con schema_version=7 elimina esa tabla y registra V1 en refinery_schema_history.
- run() llamado dos veces es idempotente.

## Fuente de verdad

- src/storage/migrations.rs
- src/storage/sqlite.rs
