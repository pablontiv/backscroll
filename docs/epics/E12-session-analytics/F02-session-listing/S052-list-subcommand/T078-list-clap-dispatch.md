---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T078: Add list subcommand to CLI

**Story**: [S052 List subcommand](README.md)
**Contribuye a**: P2 (list retorna sesiones con metadata)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Add `list` as new subcommand in clap CLI.

## Especificacion Tecnica

1. Add List variant to Commands enum
2. Flags: --project, --all-projects, --recent N (alias for --limit), --json, --robot
3. Wire dispatch to list_sessions()

## Alcance

**In**: Add List variant to Commands enum. Flags: --project, --all-projects, --recent N (alias for --limit), --json, --robot. Wire dispatch to list_sessions().
**Out**: No output formatting (T079), no tests (T080)

## Criterios de Aceptacion

- `backscroll list --help` shows correct flags
- `backscroll list --recent 5` executes
- `just check` pasa

## Fuente de verdad

- `src/main.rs`
