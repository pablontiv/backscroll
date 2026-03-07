---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T010: Test de re-sync sin duplicados + test de metadata

**Story**: [S023 Sync correcto con metadata](README.md)
**Contribuye a**: Re-sync no produce duplicados; metadata correcta end-to-end

[[blocks:T009-extract-timestamp-ordinal]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Verificar el comportamiento completo de S023: re-sync no duplica y metadata (timestamp, ordinal) se persisten correctamente.

## Alcance

**In**:
1. Test `test_resync_no_duplicates`: sync archivo, modificar, re-sync → count no aumenta
2. Test `test_metadata_persisted`: sync → verificar timestamp y ordinal en search_items
3. Test `test_resync_updates_content`: sync, modificar contenido, re-sync → contenido actualizado

**Out**: No agregar tests de filtrado (E07).

## Estado inicial esperado

- DELETE before reinsert implementado (T008)
- Timestamp y ordinal se extraen y persisten (T009)

## Criterios de Aceptacion

- `cargo test test_resync_no_duplicates` pasa
- `cargo test test_metadata_persisted` pasa
- `cargo test test_resync_updates_content` pasa

## Fuente de verdad

- `tests/cli.rs` o tests unitarios en `src/core/sync.rs`
