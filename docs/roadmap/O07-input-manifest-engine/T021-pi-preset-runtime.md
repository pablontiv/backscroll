---
id: T021
tipo: task
estado: Completed
titulo: Pi preset funcional en runtime
outcome: O07
dependencias: [T016, T017, T018, T019]
---

# T021 — Pi preset funcional en runtime

Verificar y ajustar `tests/fixtures/pi.inputs.toml` como preset de Pi para
el motor declarativo. Pi usa el mismo formato JSONL que Claude pero con campos
ligeramente diferentes.

## Alcance

- Mover/copiar `tests/fixtures/pi.inputs.toml` a `inputs/pi.inputs.toml`
- Ajustar el preset para mapear correctamente los campos Pi:
  - `role` en Pi puede ser `human`/`assistant` (Claude usa `user`/`assistant`)
  - `uuid` y `session_id` pueden tener paths diferentes
  - Ajustar predicados para excluir ruido específico de Pi
- Test con fixture JSONL de Pi: `backscroll sync` indexa sesiones Pi correctamente

## Criterios de aceptación

- `backscroll sync` con `pi.inputs.toml` activo indexa sesiones Pi desde `~/.pi/` (o dir configurado)
- Los mensajes de Pi aparecen con `source = "session"` en la DB
- Roles normalizados a `user`/`assistant` independientemente del valor en el JSONL
- `go test ./...` pasa

## Referencias

- `tests/fixtures/pi.inputs.toml` — preset existente como base
- `src/input_config.rs` — parser Pi en v0 (referencia de campos)
