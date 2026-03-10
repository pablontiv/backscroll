---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T074: Add topics subcommand to CLI

**Story**: [S051 Topics subcommand & output](README.md)
**Contribuye a**: P1 (topics retorna terminos rankeados)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Add `topics` as a new subcommand in the clap CLI definition and dispatch logic.

## Especificacion Tecnica

1. Add Topics variant to Commands enum in main.rs
2. Add flags: --project, --all-projects, --limit (default 30), --json, --robot
3. Wire dispatch to call get_topics()

## Alcance

**In**: Add Topics variant to Commands enum in main.rs. Add flags: --project, --all-projects, --limit (default 30), --json, --robot. Wire dispatch to call get_topics().
**Out**: No output formatting (T075), no tests (T076)

## Criterios de Aceptacion

- `backscroll topics --help` shows correct flags
- `backscroll topics` executes without error
- `just check` pasa

## Fuente de verdad

- `src/main.rs` (Commands enum ~line 30, match dispatch ~line 150)
