---
id: T046
tipo: task
estado: Pending
titulo: sessions query/list/validate subcommands bajo namespace sessions
outcome: O11
---

# T046 — `sessions` namespace con query/list/validate

Implementar el subcomando `backscroll sessions` con sub-subcomandos para inspección
y validación de sesiones indexadas. El Go port tiene `list` y `validate` top-level;
esta task los agrupa bajo el namespace `sessions` y agrega `sessions query`.

## Alcance

En `cmd/backscroll/sessions.go`:

```
backscroll sessions list
  --recent N          últimas N sesiones (default: 10)
  --project <name>    filtrar por proyecto
  --all-projects      todas las sesiones
  --json              output JSON
  --robot             output para LLM

backscroll sessions query <expr>
  --project <name>
  --all-projects
  --after <date>
  --before <date>
  --source <source>
  --json

backscroll sessions validate
  --project <name>
  --all-projects
```

`sessions list` y `sessions validate` pueden delegarse a la lógica existente
en `list.go` y `validate.go` respectivamente — es solo un alias bajo el nuevo namespace.

`sessions query` agrega filtros combinados que `search` no ofrece directamente
(e.g., buscar sesiones (no mensajes) por metadatos).

## Criterios de aceptación

- `backscroll sessions list` produce el mismo output que `backscroll list`
- `backscroll sessions validate` produce el mismo output que `backscroll validate`
- `backscroll sessions query --after 2024-01-01 --project myproject` filtra sesiones
- `--json` en todos los sub-subcomandos produce JSON válido
- Backward compat: `backscroll list` y `backscroll validate` siguen funcionando
- `go test ./cmd/backscroll/...` pasa
