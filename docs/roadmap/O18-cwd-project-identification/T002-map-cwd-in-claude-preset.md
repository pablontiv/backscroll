---
estado: Completed
tipo: task
---
# T002: Map cwd in the shipped Claude input preset

**Outcome**: [Workspace bucketing por cwd](README.md)

[[blocked_by:./T001-plumb-session-cwd-to-identify.md]]

## Contexto

El preset Pi ya mapea `project = "$.cwd"`, pero el preset Claude shipped (`inputs/claude.inputs.toml`) no, así que las sesiones de Claude no aportan cwd al pipeline. Una vez que T001 hace que `ParseDeclarative` lea `Map.Project`, este mapeo empieza a tener efecto y las sesiones de Claude se agrupan por cwd igual que las de Pi.

## Alcance

**In**:
1. `inputs/claude.inputs.toml` — agregar `project = "$.cwd"` a `[inputs.map]`.
2. `docs/sync.md` — documentar que las sesiones Claude se agrupan por cwd (igual que Pi).

**Out**:
1. Actualizar el preset instalado del usuario (`~/.config/backscroll/inputs/...`) y rebuild/reinstall del binario — es paso de despliegue, no del repo.

## Criterios de Aceptación

- `inputs/claude.inputs.toml` mapea `project = "$.cwd"` en `[inputs.map]`.
- `docs/sync.md` menciona el bucketing por cwd para las sesiones Claude.
- `just check fmt test` verde.

## Fuente de verdad

- `inputs/claude.inputs.toml`
- `docs/sync.md`
