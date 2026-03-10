---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T076: Integration test for topics

**Story**: [S051 Topics subcommand & output](README.md)
**Contribuye a**: P1 (topics retorna terminos rankeados)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

End-to-end test: sync fixture data, run topics subcommand, verify output format.

## Especificacion Tecnica

1. Test with --robot flag, verify tab-separated format
2. Test with --json, verify JSON lines
3. Verify term counts are reasonable for fixture data

## Alcance

**In**: Integration test for topics subcommand with robot and JSON output
**Out**: No unit tests (covered by T073)

## Criterios de Aceptacion

- Integration test passes
- Output format matches spec
- `just test` pasa

## Fuente de verdad

- `tests/` — integration test
