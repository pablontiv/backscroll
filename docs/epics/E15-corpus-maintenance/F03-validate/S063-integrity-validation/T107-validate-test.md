---
ejecutable_en: 1 sesion
estado: Completed # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: test # [code, test, refactor, chore, docs]
---
# T107: Integration test for validate

**Story**: [S063 Integrity Validation](README.md)
**Contribuye a**: Test verifica validate end-to-end

[[blocks:T106-validate-subcommand]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Test que: 1) sync fixtures, 2) validate reports healthy, 3) manually delete source file, 4) validate reports orphan.

## Alcance

**In**:
1. Sync fixture, run validate → expect "0 issues"
2. Delete source JSONL file from disk
3. Run validate → expect orphan detected
4. Verify exit code (0 for healthy, non-zero for issues)

**Out**: Tests de otros subcommands

## Estado inicial esperado

- T106 completado
- tests/cli.rs

## Criterios de Aceptacion

- Test `test_validate_healthy` pasa (clean index)
- Test `test_validate_orphan` pasa (detects deleted source file)
- `cargo test test_validate` exit 0

## Fuente de verdad

- `tests/cli.rs`
