---
estado: Completed
tipo: task
---
# T001: Plumb session cwd into project identification

**Outcome**: [Workspace bucketing por cwd](README.md)

## Contexto

`cmd/backscroll/sync_helpers.go` llama `projects.Identify(ref, registry)` con `ref` = el path del archivo de sesión (de `reader.Discover`), no el cwd. `Identify`/`LoadLocalHint` (`internal/projects/projects.go`) están diseñados para un directorio de trabajo, así que nunca matchean los roots de `projects.toml` (paths de repo reales) ni los hints `.backscroll/project.toml`. Además `ParseDeclarative` (`internal/input_config/pipeline.go`) extrae role/uuid/timestamp/session_id pero NO `def.Map.Project` (aunque `MapConfig.Project` existe en `internal/input_config/types.go`), y `models.ParsedFile` no tiene campo para el cwd. Por eso toda sesión file-based resuelve a `unknown` y los buckets quedan vacíos.

## Alcance

**In**:
1. `internal/models/models.go` — agregar `Cwd string` a `ParsedFile`.
2. `internal/input_config/pipeline.go` — en `ParseDeclarative`, extraer `def.Map.Project` por record (`SelectString`) y exponer el primer valor no-vacío como cwd de la sesión.
3. `internal/readers/jsonl_reader.go` — poblar `ParsedFile.Cwd` con ese valor.
4. `cmd/backscroll/sync_helpers.go` — usar el cwd de la sesión: `projects.Identify(cwd, registry)`, con fallback al path (`ref`) solo si el cwd está vacío.

**Out**:
1. Cambiar el preset Claude (va en T002).
2. Migrar el path legacy `sync.ParseSessions` (sin Map) ni el reader de OpenCode.

## Criterios de Aceptación

- `ParseDeclarative` surfacea el valor de `def.Map.Project` (`$.cwd`) en `ParsedFile.Cwd` (primer record no-vacío).
- `sync_helpers` indexa con `Project` derivado del cwd de la sesión; usa el path solo cuando el cwd está vacío.
- Una sesión cuyo cwd cae bajo un root de `projects.toml` (o un hint `.backscroll/project.toml`) se indexa con `Project` = ese id, no `unknown` (test con fixture).
- `just check fmt test` verde; cobertura ≥85% mantenida.

## Fuente de verdad

- `internal/models/models.go`
- `internal/input_config/pipeline.go`
- `internal/readers/jsonl_reader.go`
- `cmd/backscroll/sync_helpers.go`
- `internal/projects/projects.go` (consumidor; no se modifica)
