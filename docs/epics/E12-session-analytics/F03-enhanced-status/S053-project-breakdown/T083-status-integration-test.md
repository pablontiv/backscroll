---
estado: Completed
tipo: test
ejecutable_en: 1 sesion
---
# T083: Test enhanced status output

**Story**: [S053 Per-project breakdown](README.md)
**Contribuye a**: P3 (status incluye breakdown por proyecto)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Test that enhanced status shows project breakdown.

## Especificacion Tecnica

1. Sync multi-project fixtures
2. Run status command
3. Verify output includes "By Project" section
4. Verify per-project counts are correct

## Alcance

**In**: Integration test for enhanced status output
**Out**: No unit tests

## Criterios de Aceptacion

- Test passes
- Output contains "By Project" section
- Per-project counts are correct
- `just test` pasa

## Fuente de verdad

- `tests/` — integration test
