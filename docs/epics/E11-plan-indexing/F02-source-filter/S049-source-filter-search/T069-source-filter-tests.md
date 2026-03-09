---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T069: Source filter tests

**Story**: [S049 Source filter in search](README.md)
**Contribuye a**: P3 (source filter), P4 (default preserva comportamiento)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Verificar que el filtro --source funciona correctamente.

## Especificacion Tecnica

1. Unit test: sync sessions + plans, search con source=plans → solo plans
2. Unit test: search con source=sessions → solo sessions
3. Unit test: search con source=all → ambos
4. Integration test via CLI: `backscroll search "test" --source plans`

## Alcance

**In**: Unit tests y integration test para source filter
**Out**: No test de resume con source (T070)

## Criterios de Aceptacion

- 4 tests
- `just test` pasa

## Fuente de verdad

- `src/storage/sqlite.rs` — mod tests
- `tests/cli.rs`
