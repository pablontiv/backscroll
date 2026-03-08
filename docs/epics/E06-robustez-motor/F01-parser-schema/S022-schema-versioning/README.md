# S022: Schema versioning + content table

**Feature**: [F01 Parser y Schema](../README.md)
**Capacidad**: La base de datos tiene schema versionado con tabla de contenido separada y FTS5 external content con triggers.
**Cubre**: P2 del Epic (schema con content table)

## Antes / Despues

**Antes**: Schema usa inline FTS5 (`messages_fts USING fts5(path, role, content, project)`) que es denormalizado y no extensible. No hay mecanismo de migracion de schema. No se puede agregar columnas como timestamp, uuid, ordinal sin reescribir todo.

**Despues**: `search_items` table con columnas (id, source, source_path, ordinal, role, text, timestamp, uuid, project). `messages_fts` usa external content (`content=search_items`). Triggers AI/AD/AU mantienen el indice sincronizado. `schema_version` table permite migraciones futuras.

## Criterios de Aceptacion (semanticos)

- [x] Schema tiene tabla search_items con todas las columnas requeridas
- [x] FTS5 usa external content pattern con triggers
- [x] Existe mecanismo de migracion de schema

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T004](T004-schema-version-table.md) | Crear tabla schema_version + mecanismo de migracion |
| [T005](T005-search-items-table.md) | Crear tabla search_items con columnas completas |
| [T006](T006-fts5-external-content.md) | Crear messages_fts external content + triggers |
| [T007](T007-update-index-message.md) | Actualizar index_message para INSERT en search_items |

## Fuente de verdad

- `src/storage/sqlite.rs` — schema DDL y queries
