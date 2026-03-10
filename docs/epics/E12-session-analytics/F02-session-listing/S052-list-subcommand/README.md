# S052: List subcommand

**Feature**: [F02 Session Listing](../README.md)
**Capacidad**: Subcomando `list` que muestra sesiones indexadas con metadata (path, proyecto, message count, timestamps), ordenadas por recencia.
**Cubre**: P2 (list retorna sesiones con metadata)

## Antes / Despues

**Antes**: No hay forma de listar sesiones desde el CLI. El skill usa `ls -lt` sobre el filesystem como workaround.

**Despues**: `backscroll list` retorna sesiones desde el indice con metadata. Soporta --recent N, --project, --all-projects, --robot, --json.

## Criterios de Aceptacion (semanticos)

- [ ] `backscroll list` muestra sesiones del proyecto actual
- [ ] `backscroll list --recent 5` limita a 5 sesiones
- [ ] `backscroll list --all-projects` muestra de todos los proyectos
- [ ] Output incluye path, project, message count, started, ended
- [ ] `--robot` y `--json` funcionan

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV4: Output consistente con search/resume
  - Verificar: comparar formato con `backscroll search --robot`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T077](T077-list-query-function.md) | Session listing query function |
| [T078](T078-list-clap-dispatch.md) | Add list subcommand to CLI |
| [T079](T079-list-output-format.md) | Output formatting for list |
| [T080](T080-list-integration-test.md) | Integration test for list |

## Fuente de verdad

- `src/storage/sqlite.rs` — query function
- `src/main.rs` — clap subcommand + dispatch
- `src/output.rs` — output formatting
