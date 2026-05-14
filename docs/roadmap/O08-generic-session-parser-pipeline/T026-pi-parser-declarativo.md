---
id: T026
tipo: task
estado: Pending
titulo: Pi como primer parser declarativo nativo via manifest
outcome: O08
dependencias: [T024, T025, T021]
---

# T026 — Pi como primer parser declarativo nativo via manifest

Migrar el parsing de Pi de "funciona por compatibilidad de formato" a un parser
declarativo completo via su manifest TOML. Pi tiene campos ligeramente diferentes
a Claude (role `human` vs `user`, estructura del mensaje).

## Alcance

- Actualizar `inputs/pi.inputs.toml` con los `map` y `content` selectors correctos para Pi
- Implementar normalización de roles en `JsonlReader.Parse()`:
  `human` → `user`, mantener `assistant`
- Predicate para excluir mensajes de sistema Pi-específicos
- Verificar con fixture Pi real (en `tests/fixtures/`)

## Criterios de aceptación

- Pi JSONL indexado via pipeline genérico produce el mismo `ParsedFile` que hoy
- Rol `human` normalizado a `user` en los mensajes indexados
- Mensajes de sistema de Pi excluidos via predicado en el manifest (no hardcode)
- `go test ./...` pasa; fixture Pi en tests de regresión

## Referencias

- `tests/fixtures/pi.inputs.toml` — base del preset
- `internal/sync/sync.go:rawRecord` — campos que actualmente se leen de Pi JSONL
