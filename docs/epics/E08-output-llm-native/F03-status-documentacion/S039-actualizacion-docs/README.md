# S039: Actualizacion de documentacion

**Feature**: [F03 Status y Documentacion](../README.md)
**Capacidad**: CLAUDE.md y README.md reflejan la arquitectura y comandos actualizados post-E06/E07/E08.
**Cubre**: Documentacion actualizada para el estado final del sistema

## Antes / Despues

**Antes**: CLAUDE.md y README.md reflejan el estado v1 (3 comandos, output basico, sin flags avanzados). No documentan search_items, noise filtering, --json, --read, etc.

**Despues**: CLAUDE.md actualizado con arquitectura (nuevos modulos: output.rs, reader.rs), comandos (read), flags (--json, --robot, --fields, --max-tokens, --include-agents), design decisions (external FTS5, noise filtering). README.md con usage actualizado y quick start.

## Criterios de Aceptacion (semanticos)

- [ ] CLAUDE.md documenta todos los modulos actuales
- [ ] README.md tiene usage con nuevos flags

## Invariantes

- INV1: Busqueda sin flags produce output legible
  - Verificar: N/A (docs only)

## Tasks

| Task | Descripcion |
|------|-------------|
| [T044](T044-update-claude-md.md) | Actualizar CLAUDE.md con arquitectura y flags |
| [T045](T045-update-readme.md) | Actualizar README.md con usage y quick start |

## Fuente de verdad

- `CLAUDE.md` — instrucciones del proyecto
- `README.md` — documentacion publica
