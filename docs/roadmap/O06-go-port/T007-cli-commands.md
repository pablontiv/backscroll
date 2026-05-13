---
estado: Completed
tipo: task
---
# T007: Comandos CLI

**Outcome**: [Port a Go](README.md)

## Contexto

Glue layer: los 13 comandos cobra que conectan los módulos internos con la interfaz de usuario. Equivalente a `main.rs` (3.285 líneas). El entrypoint sigue el patrón de roadmapctl: `run()` con inyección de stdout/stderr para testabilidad.

## Alcance

**In**:
1. `cmd/backscroll/main.go` — `run()` inyectable, cobra root command con version flag.
2. Un archivo por comando: `sync.go`, `search.go`, `read.go`, `resume.go`, `list.go`, `topics.go`, `insights.go`, `export.go`, `reindex.go`, `purge.go`, `validate.go`, `status.go`.
3. `decisions.go` — subcomandos: `query`, `context`, `extract`, `conflicts`, `replay`.
4. `projects.go` — subcomandos: `identify`, `list`.
5. Todos los flags actuales preservados con los mismos nombres y comportamiento.
6. `--help` de cada comando describe correctamente los flags.

**Out**:
1. Tests de integración E2E (van en T008).
2. Lógica de negocio (ya en T002–T006).

## Criterios de Aceptación

- `backscroll --help` lista los 13 comandos.
- `backscroll sync --help` y `backscroll search --help` muestran todos los flags actuales.
- `backscroll status` conecta a la DB existente y muestra output correcto.
- `backscroll search "test"` retorna resultados sin panic.
- `go build ./cmd/backscroll` compila sin errores ni warnings.
