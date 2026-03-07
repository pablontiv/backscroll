---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T023: Test con fixture JSONL multi-type

**Story**: [S029 Pre-filtrado por record type](README.md)
**Contribuye a**: Records con isMeta=true no se indexan

[[blocks:T022-filter-record-type]]

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Crear una fixture JSONL que contenga records de multiples tipos (user, assistant, progress, system) y verificar que solo user/assistant son procesados.

## Alcance

**In**:
1. Crear fixture con records: 2 user, 2 assistant, 3 progress, 1 system, 1 isMeta=true
2. Test que sync produce exactamente 4 mensajes (2 user + 2 assistant)
3. Verificar que conteo de skipped se loguea correctamente

**Out**: No agregar filtros de contenido.

## Estado inicial esperado

- Filtrado por tipo implementado (T022)

## Criterios de Aceptacion

- `cargo test test_multitype_fixture` pasa
- Fixture tiene >= 5 tipos distintos de records
- Solo 4 mensajes se indexan

## Fuente de verdad

- `tests/cli.rs` o tests unitarios en `src/core/sync.rs`
