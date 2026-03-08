---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T009: Extraer timestamp y calcular ordinal

**Story**: [S023 Sync correcto con metadata](README.md)
**Contribuye a**: Cada mensaje tiene timestamp y ordinal correctos

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Cada `SessionRecord` tiene un campo `timestamp` opcional. El ordinal se calcula como la posicion del mensaje dentro del archivo JSONL (0-indexed). Ambos se persisten en search_items.

## Alcance

**In**:
1. Extraer `record.timestamp` del SessionRecord parseado
2. Calcular ordinal como contador incremental por archivo
3. Pasar timestamp y ordinal a index_message/sync_files

**Out**: No modificar schema (ya tiene las columnas desde T005).

## Estado inicial esperado

- SessionRecord tiene campo timestamp (T001)
- search_items tiene columnas timestamp y ordinal (T005)
- index_message acepta timestamp y ordinal (T007)

## Criterios de Aceptacion

- `SELECT timestamp, ordinal FROM search_items LIMIT 5` muestra valores non-NULL
- Ordinal es secuencial (0, 1, 2, ...) dentro de un archivo
- `just check` pasa

## Fuente de verdad

- `src/core/sync.rs`
