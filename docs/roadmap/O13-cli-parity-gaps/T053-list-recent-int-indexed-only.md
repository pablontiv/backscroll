---
id: T053
tipo: task
estado: Pending
titulo: list --recent N (int) + --indexed-only en list y status
outcome: O13
dependencias: []
---

# T053 — `list --recent N` semántica correcta + `--indexed-only`

Corregir la semántica de `--recent` en `backscroll list` de bool a int (número de sesiones a mostrar), y añadir `--indexed-only` a `list` y `status`.

## Alcance

En `cmd/backscroll/list.go`:
- Cambiar `recent bool` → `recent int` (default: 0 = sin límite, o 20 como default)
- Actualizar `runList` signature: `recent int`
- Actualizar `ListSessions` en storage para aceptar `recent int` (si >0 → `ORDER BY timestamp DESC LIMIT recent`)
- Añadir `--indexed-only bool` — si true, usar `storage.OpenReadOnly` en lugar de `storage.Open`

En `cmd/backscroll/sessions.go` — actualizar `newSessionsListCmd` para pasar `recent int`.

En `cmd/backscroll/status.go`:
- Añadir `--indexed-only bool` — si true, usar `storage.OpenReadOnly`

En `internal/storage/queries.go`:
- Actualizar `ListSessions(project string, recent int)` — si recent>0: `ORDER BY timestamp DESC LIMIT ?`

## Criterios de aceptación

- `backscroll list --recent 20` muestra exactamente 20 sesiones más recientes
- `backscroll list --recent 0` o sin flag muestra todas
- `backscroll list --indexed-only` abre DB read-only sin auto-sync
- `backscroll status --indexed-only` no hace auto-sync
- Tests actualizados para `recent int`
- Coverage ≥85% mantenido
