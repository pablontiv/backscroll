# S029: Pre-filtrado por record type

**Feature**: [F01 Filtrado de Contenido](../README.md)
**Capacidad**: Solo records de tipo user y assistant se parsean e indexan. Records de tipo progress, system, y isMeta=true se descartan.
**Cubre**: P1 del Epic (solo user/assistant indexados)

## Antes / Despues

**Antes**: El parser intenta parsear todos los records JSONL sin discriminar por `type`. Records de progreso (51.6% del corpus real), system, y meta se procesan innecesariamente.

**Despues**: `sync.rs` filtra por `type` field: solo `user` y `assistant` se procesan. Records con `isMeta: true` se descartan. Reduccion significativa de volumen procesado.

## Criterios de Aceptacion (semanticos)

- [ ] Solo records type=user y type=assistant se indexan
- [ ] Records con isMeta=true no se indexan

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T022](T022-filter-record-type.md) | Filtrar records por type en sync |
| [T023](T023-test-multitype-fixture.md) | Test con fixture JSONL multi-type |

## Fuente de verdad

- `src/core/sync.rs` — logica de filtrado
- `src/core/models.rs` — SessionRecord con campo type
