---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T005: Crear tabla search_items con columnas completas

**Story**: [S022 Schema versioning + content table](README.md)
**Contribuye a**: Schema tiene content table con source, timestamp, ordinal, uuid, project

[[blocks:T004-schema-version-table]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

El schema actual usa inline FTS5 que es denormalizado. Se necesita una content table `search_items` con todas las columnas necesarias para metadata y filtrado.

## Especificacion Tecnica

```sql
CREATE TABLE IF NOT EXISTS search_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL DEFAULT 'session',
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    role TEXT NOT NULL,
    text TEXT NOT NULL,
    timestamp TEXT,
    uuid TEXT,
    project TEXT
);
CREATE INDEX idx_search_items_source_path ON search_items(source_path);
CREATE INDEX idx_search_items_project ON search_items(project);
```

## Alcance

**In**:
1. Agregar DDL de `search_items` como migracion v1→v2 en el mecanismo de T004
2. Crear indices para source_path y project
3. Test que verifica las columnas existen

**Out**: No crear FTS5 external content (T006). No actualizar index_message (T007).

## Estado inicial esperado

- Mecanismo de migracion existe (T004 completado)
- Solo existe messages_fts inline

## Criterios de Aceptacion

- `sqlite3 test.db ".schema search_items"` muestra tabla con todas las columnas
- Indices idx_search_items_source_path e idx_search_items_project existen
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
