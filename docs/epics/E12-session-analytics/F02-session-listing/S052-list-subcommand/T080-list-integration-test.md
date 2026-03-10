---
ejecutable_en: 1 sesion
estado: Completed
tipo: code
---
# T080: Integration test for list

**Story**: [S052 List subcommand](README.md)
**Contribuye a**: P2 (list retorna sesiones con metadata)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

E2E test for list subcommand.

## Especificacion Tecnica

1. Sync fixtures with known session data
2. Run list subcommand
3. Verify output contains expected session paths and metadata

## Alcance

**In**: Integration test for list subcommand
**Out**: No unit tests

## Criterios de Aceptacion

- Test passes
- Output contains expected session paths
- `just test` pasa

## Fuente de verdad

- `tests/` — integration test
