---
estado: Pending
tipo: docs
ejecutable_en: 1 sesion
---
# T044: Actualizar CLAUDE.md con arquitectura y flags

**Story**: [S039 Actualizacion de documentacion](README.md)
**Contribuye a**: CLAUDE.md documenta todos los modulos actuales

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`

## Contexto

CLAUDE.md refleja el estado v1 del proyecto. Despues de E06-E08, hay modulos nuevos (output.rs, reader.rs), flags nuevos (--json, --robot, --fields, --max-tokens, --include-agents), un comando nuevo (read), y cambios arquitecturales (external FTS5, noise filtering, dyn SearchEngine).

## Alcance

**In**:
1. Actualizar seccion Architecture: agregar output.rs, reader.rs
2. Actualizar seccion Commands: agregar read, flags nuevos
3. Actualizar Key Design Decisions: external FTS5, noise filtering, subagent exclusion
4. Actualizar seccion Architecture diagram

**Out**: No modificar README.md (T045).

## Estado inicial esperado

- CLAUDE.md desactualizado (refleja estado v1)
- Todos los cambios de E06-E08 implementados

## Criterios de Aceptacion

- CLAUDE.md menciona output.rs y reader.rs
- CLAUDE.md documenta --json, --robot, --read flags
- CLAUDE.md menciona external FTS5 y noise filtering
- `just check` pasa (no afecta codigo)

## Fuente de verdad

- `CLAUDE.md`
