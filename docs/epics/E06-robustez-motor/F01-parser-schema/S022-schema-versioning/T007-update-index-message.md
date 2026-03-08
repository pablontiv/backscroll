---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T007: Actualizar index_message para INSERT en search_items

**Story**: [S022 Schema versioning + content table](README.md)
**Contribuye a**: Schema tiene content table funcional end-to-end

[[blocks:T006-fts5-external-content]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

`index_message` actualmente hace INSERT directo en `messages_fts` (inline FTS5). Con el nuevo schema, debe insertar en `search_items` y los triggers de T006 popularan `messages_fts` automaticamente.

## Alcance

**In**:
1. Cambiar INSERT de messages_fts a search_items en index_message
2. Agregar parametros: source_path, ordinal, timestamp, uuid, project
3. Actualizar firma de index_message para aceptar los nuevos campos
4. Test que verifica INSERT end-to-end: search_items → messages_fts via trigger

**Out**: No modificar search queries (se adaptan en S025).

## Estado inicial esperado

- FTS5 external content con triggers existe (T006 completado)
- index_message inserta en messages_fts inline

## Criterios de Aceptacion

- index_message inserta en search_items (no directamente en messages_fts)
- `cargo test test_index_and_search` — insert + FTS5 search funciona end-to-end
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
