---
id: T048
tipo: task
estado: Completed
titulo: Tests de integración para nuevos subcomandos
outcome: O11
dependencias: [T045, T046, T047]
---

# T048 — Tests de integración para nuevos subcomandos

Tests de integración end-to-end para los subcomandos nuevos de O11 usando
el harness de tests existente en `cmd/backscroll/main_test.go`.

## Alcance

- `backscroll events query <id>` → verifica output JSONL con ≥1 evento
- `backscroll events query <id> --json` → JSON válido
- `backscroll events query <id> --role user` → solo mensajes de usuario
- `backscroll sessions list` → mismos resultados que `backscroll list`
- `backscroll sessions validate` → mismos resultados que `backscroll validate`
- `backscroll sessions query --after <date>` → subset correcto de sesiones
- `backscroll status --json` → presencia de campos `active_inputs`, `using_declarative_inputs`
- Backward compat: `backscroll list` y `backscroll validate` siguen funcionando

## Criterios de aceptación

- `go test -race ./cmd/backscroll/...` pasa sin data races
- `just coverage` ≥85% en `cmd/backscroll/`
- Tests usan DB temporal y fixtures en `tests/fixtures/` — no acceden a `~/.claude/`
- CI pasa (`just check && just test`)
