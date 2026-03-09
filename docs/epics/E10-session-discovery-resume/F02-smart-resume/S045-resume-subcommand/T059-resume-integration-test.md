---
estado: Completed
tipo: test
ejecutable_en: 1 sesion
---
# T059: Resume integration test

**Story**: [S045 Resume subcommand](README.md)
**Contribuye a**: P3 (resume produce session ID), P4 (pipe-friendly)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Integration test end-to-end: sync fixture, resume query, verificar output.

## Especificacion Tecnica

En `tests/cli.rs`:

1. Test: sync fixture JSONL, `backscroll resume "keyword"`, verificar output contiene source path
2. Test: `backscroll resume "keyword" --robot` produce single line tab-separated
3. Test: `backscroll resume "nonexistent"` produce exit code 1

## Alcance

**In**: Integration tests en cli.rs
**Out**: No unit tests de resume logic

## Criterios de Aceptacion

- 3 integration tests
- `just test` pasa

## Fuente de verdad

- `tests/cli.rs`
