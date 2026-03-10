# S051: Topics subcommand & output

**Feature**: [F01 Topic Discovery](../README.md)
**Capacidad**: Subcomando `topics` en el CLI con flags --project, --all-projects, --limit, y formatos de output text/robot/json.
**Cubre**: P1 (topics retorna terminos rankeados)

## Antes / Despues

**Antes**: No existe subcomando topics. El skill hace multiples `backscroll search` con keywords hardcoded.

**Despues**: `backscroll topics` retorna los N terminos mas frecuentes del corpus. Soporta --project, --all-projects, --limit N, --robot, --json.

## Criterios de Aceptacion (semanticos)

- [ ] `backscroll topics` muestra terminos en formato texto legible
- [ ] `backscroll topics --robot` produce output tab-separated (term, sessions, mentions)
- [ ] `backscroll topics --json` produce JSON lines
- [ ] `--project` filtra por proyecto
- [ ] `--limit N` controla cantidad de resultados (default: 30)

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T074](T074-topics-clap-dispatch.md) | Add topics subcommand to CLI |
| [T075](T075-topics-output-format.md) | Output formatting for topics |
| [T076](T076-topics-integration-test.md) | Integration test for topics |

## Fuente de verdad

- `src/main.rs` — clap subcommand + dispatch
- `src/output.rs` — output formatting
