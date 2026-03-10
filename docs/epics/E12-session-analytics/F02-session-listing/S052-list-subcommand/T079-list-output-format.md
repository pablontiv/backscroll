---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T079: Output formatting for list

**Story**: [S052 List subcommand](README.md)
**Contribuye a**: P2 (list retorna sesiones con metadata)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV4: Output consistente con search/resume
  - Verificar: comparar formato con `backscroll search --robot`

## Contexto

Format list output in text/robot/json.

## Especificacion Tecnica

- **Text**: table with path, project, messages, started, ended
- **Robot**: tab-separated
- **JSON**: one object per line

## Alcance

**In**: Text, robot, and JSON output formatting for list subcommand
**Out**: No CLI wiring (T078), no tests (T080)

## Criterios de Aceptacion

- Formats consistent with topics and search
- `just check` pasa

## Fuente de verdad

- `src/output.rs`
