---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T056: Tests discovery

**Story**: [S044 Directory discovery](README.md)
**Contribuye a**: P1 — sync descubre directorios legacy + actual

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Verificar que discovery funciona con diferentes layouts de directorio.

## Especificacion Tecnica

1. Unit test: tempdir simulando `~/.claude/projects/foo/` con .jsonl → descubierto
2. Unit test: tempdir sin dirs de sesion → vec vacio
3. Integration test: sync con dirs descubiertos produce resultados
4. Test: --path explicito overridea discovery

## Alcance

**In**: Tests para discover_session_dirs() y integracion con sync
**Out**: No test de plan discovery (E11)

## Criterios de Aceptacion

- 3+ tests
- `just test` pasa

## Fuente de verdad

- `src/config.rs` — mod tests
- `tests/cli.rs`
