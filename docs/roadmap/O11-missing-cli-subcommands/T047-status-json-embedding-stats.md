---
id: T047
tipo: task
estado: Completed
titulo: Mejora status --json con embedding stats e inputs activos
outcome: O11
dependencias: [T043, T019]
---

# T047 — Mejora `status --json` con embedding stats e inputs activos

Extender el output de `backscroll status --json` con los campos de embedding
(si O10 está disponible) y la lista de inputs activos (si O07 está disponible).

## Alcance

En `cmd/backscroll/status.go`:

Campos adicionales en el JSON de status:
```json
{
  "... campos existentes ...",
  "active_inputs": ["claude", "pi"],
  "chunks_count": 12340,
  "embeddings_count": 12340,
  "embedding_model": "all-MiniLM-L6-v2",
  "using_declarative_inputs": true
}
```

En output text:
```
inputs: 2 active (claude, pi)  [o "1 active (legacy: session_dirs)"]
chunks: 12340
embeddings: 12340 (model: all-MiniLM-L6-v2)
```

Los campos de embeddings se muestran solo si `embedding.enabled = true`.
Los campos de inputs se muestran siempre (al menos el modo: declarativo o legacy).

## Criterios de aceptación

- `backscroll status --json` tiene `active_inputs` (array de nombres)
- `backscroll status --json` tiene `using_declarative_inputs` (bool)
- Con embeddings deshabilitados, los campos de embedding son 0/omitidos
- Test de integración en `cmd/backscroll/main_test.go` verifica el JSON
- `go test ./...` pasa
