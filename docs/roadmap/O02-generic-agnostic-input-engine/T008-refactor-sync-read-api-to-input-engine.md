---
estado: Specified
tipo: task
---
# T008: Refactor sync/read/library API to use input engine only

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE1, CE3, CE4

[[blocked_by:./T006-create-claude-input-preset.md]]
[[blocked_by:./T007-create-pi-input-preset.md]]

## Preserva

- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: DB sync sigue consumiendo esos tipos.
- INV3: No se introducen plugins/scripts ejecutables para el MVP.
  - Verificar: API usa input engine in-process.

## Contexto

O01 permitió un periodo transicional con `--path`, `session_dirs`, `parse_sessions` Claude-only y `read_session` Claude-only. O02 reemplaza ese flujo principal por ingesta TOML-only: sync, autosync, read y API pública deben cargar manifests activos y ejecutar el engine genérico.

La ausencia de manifest activo no debe activar un fallback Claude implícito. Si se conserva una API legacy por compatibilidad temporal, debe quedar fuera del camino principal, marcada como legacy y cubierta por tests que prueben que no se usa en flujos canónicos.

## Alcance

**In**:
1. Cambiar `sync` y autosync para cargar manifests `*.inputs.toml`/`backscroll.inputs.d/*.toml` y parsearlos con el input engine.
2. Cambiar `read` para usar el mismo input engine o resolver explícitamente el input aplicable por manifest.
3. Remover/refactorizar `resolve_session_inputs` y cualquier código que inyecte `parser = "claude"`.
4. Remover o reemplazar `parse_sessions(...)` Claude-only como API pública canónica.
5. Remover o aislar `read_session` Claude-only para que no sea usado por el flujo principal.
6. Actualizar `tests/lib_api.rs` y tests CLI para usar manifests.
7. Eliminar `--path` y `session_dirs` del flujo canónico si siguen presentes; cualquier compatibilidad temporal debe ser explícita y no silenciosa.

**Out**:
- Mantener compatibilidad legacy como comportamiento principal.
- Fallback Claude implícito.
- Migrar document sources; va en T009.

## Estado inicial esperado

- `src/main.rs` tiene `--path` y resolución de rutas/session inputs heredada.
- `src/core/reader.rs` parsea Claude directamente.
- `tests/lib_api.rs` llama `parse_sessions(...)`.

## Criterios de Aceptación

- Ningún comando de ingesta asume Claude si no hay manifest activo válido.
- `sync`, autosync y `read` pasan por el input engine y no por parsers provider-specific como camino principal.
- `read` no importa `SessionRecord` ni usa parser Claude directo.
- Tests de lib/API usan manifests y fallan si se reintroduce parser Claude implícito.
- `rg "parse_sessions\(|read_session|session_dirs|--path|~/.claude/projects" src tests` no muestra caminos canónicos pendientes; cualquier match permitido está aislado como legacy/migración explícita o fixture del preset Claude.

## Fuente de verdad

- `docs/input-contract.md`
- `src/main.rs`
- `src/core/sync.rs`
- `src/core/reader.rs`
- `tests/lib_api.rs`
- `tests/cli.rs`
