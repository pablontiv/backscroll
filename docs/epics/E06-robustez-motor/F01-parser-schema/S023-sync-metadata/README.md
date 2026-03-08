# S023: Sync correcto con metadata

**Feature**: [F01 Parser y Schema](../README.md)
**Capacidad**: Re-sync no produce duplicados y los metadatos (timestamp, ordinal) se persisten correctamente.
**Cubre**: P4 del Epic (re-sync sin duplicados)

## Antes / Despues

**Antes**: No hay DELETE antes de reinsert — re-sincronizar un archivo duplica todos sus mensajes. Timestamp y ordinal no se extraen del JSONL.

**Despues**: `DELETE FROM search_items WHERE source_path = ?` antes de reinsertar. Timestamp se extrae del SessionRecord. Ordinal se calcula por posicion dentro del archivo.

## Criterios de Aceptacion (semanticos)

- [x] Re-sync del mismo archivo no produce duplicados
- [x] Cada mensaje tiene timestamp y ordinal correctos

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T008](T008-delete-before-reinsert.md) | Implementar DELETE antes de reinsert |
| [T009](T009-extract-timestamp-ordinal.md) | Extraer timestamp y calcular ordinal |
| [T010](T010-test-resync-metadata.md) | Test de re-sync sin duplicados + test de metadata |

## Fuente de verdad

- `src/core/sync.rs` — logica de sync
- `src/storage/sqlite.rs` — queries INSERT/DELETE
