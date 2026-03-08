---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T008: Implementar DELETE antes de reinsert

**Story**: [S023 Sync correcto con metadata](README.md)
**Contribuye a**: Re-sync del mismo archivo no produce duplicados

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Cuando un archivo JSONL se re-sincroniza (hash cambio), los mensajes anteriores no se borran antes de reinsertar. Esto produce duplicados. Se necesita `DELETE FROM search_items WHERE source_path = ?` antes de cada reinsert.

## Alcance

**In**:
1. Agregar `DELETE FROM search_items WHERE source_path = ?` antes de INSERT loop en sync
2. Ejecutar dentro de una transaccion (DELETE + INSERTs atomicos)

**Out**: No extraer timestamp/ordinal (T009).

## Estado inicial esperado

- search_items table con triggers FTS5 (T006/T007 completados)
- No hay DELETE antes de reinsert

## Criterios de Aceptacion

- Re-sincronizar un archivo no aumenta el count de mensajes
- `cargo test test_resync_no_duplicates` pasa
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs` o `src/core/sync.rs` (donde se ejecute el sync loop)
