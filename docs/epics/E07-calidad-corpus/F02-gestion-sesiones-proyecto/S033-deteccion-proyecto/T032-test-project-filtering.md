---
estado: Completed
tipo: test
ejecutable_en: 1 sesion
---
# T032: Test end-to-end de --project filtering

**Story**: [S033 Deteccion automatica de proyecto](README.md)
**Contribuye a**: --project filter funciona en search

[[blocks:T031-project-fallback]]

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Integration test que verifica el flujo completo: sync con deteccion de proyecto → search con filtro --project retorna solo resultados del proyecto solicitado.

## Alcance

**In**:
1. Crear fixtures con 2 proyectos distintos (distintos directorios)
2. Sync ambos
3. Search sin --project → resultados de ambos
4. Search con --project "project-a" → solo resultados de project-a
5. Verificar que SELECT count(*) WHERE project IS NULL = 0

**Out**: No agregar funcionalidad nueva.

## Estado inicial esperado

- Deteccion de proyecto implementada (T030, T031)
- --project flag ya existe en search

## Criterios de Aceptacion

- `cargo test test_project_filtering_e2e` pasa
- Test verifica filtrado correcto por proyecto
- Zero registros con project NULL

## Fuente de verdad

- `tests/cli.rs`
