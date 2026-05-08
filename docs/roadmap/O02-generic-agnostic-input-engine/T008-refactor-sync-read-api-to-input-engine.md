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

El flujo canónico debe ser TOML-only. No hay usuarios legacy, así que se puede remover `--path`, `session_dirs`, `parse_sessions` como API canónica, fallback Claude y `read_session` Claude-only.

## Alcance

**In**:
1. Cambiar `sync` y autosync para cargar inputs manifiestos y parsearlos con el engine.
2. Cambiar `read` para usar el mismo input engine o resolver input por manifest.
3. Remover `resolve_session_inputs` que inyecta `parser = "claude"`.
4. Remover o reemplazar `parse_sessions(...)` Claude-only como API pública canónica.
5. Actualizar `tests/lib_api.rs` para usar manifests.
6. Eliminar CLI `--path` si contradice el modelo aprobado.

**Out**:
- Mantener compatibilidad legacy.
- Migrar document sources; va en T009.

## Estado inicial esperado

- `src/main.rs` tiene `--path` y `resolve_session_inputs`.
- `src/core/reader.rs` parsea Claude directamente.
- `tests/lib_api.rs` llama `parse_sessions(...)`.

## Criterios de Aceptación

- Ningún comando de ingesta asume Claude si no hay manifest.
- `read` no importa `SessionRecord` ni usa parser Claude directo.
- Tests de lib usan input manifests.
- `rg "parse_sessions\(|read_session|session_dirs|~/.claude/projects" src tests` no muestra caminos canónicos pendientes, salvo docs/migraciones explícitas.

## Fuente de verdad

- `src/main.rs`
- `src/core/sync.rs`
- `src/core/reader.rs`
- `tests/lib_api.rs`
- `tests/cli.rs`
