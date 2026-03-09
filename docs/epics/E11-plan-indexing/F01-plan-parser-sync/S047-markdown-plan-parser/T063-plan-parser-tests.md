---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T063: Plan parser tests

**Story**: [S047 Markdown plan parser](README.md)
**Contribuye a**: P1 (plans indexados), P2 (plans spliteados)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Verificar que el plan parser funciona correctamente con varios formatos de plan.

## Especificacion Tecnica

En `src/core/plans.rs` tests:

1. Test: plan con una sola seccion (sin ##) → 1 ParsedMessage
2. Test: plan con 3 secciones ## → 3 ParsedMessages
3. Test: plan con contenido antes del primer ## → seccion pre-header
4. Test: plan vacio → 0 ParsedMessages o error graceful
5. Snapshot test con insta para un plan representativo

## Alcance

**In**: Unit tests y snapshot test para parse_plan()
**Out**: No test de integracion con sync

## Criterios de Aceptacion

- 4+ unit tests
- 1 snapshot test
- `just test` pasa

## Fuente de verdad

- `src/core/plans.rs` — mod tests
