---
estado: Completed
tipo: task
---
# T018: Actualizar descripción general en CLAUDE.md

**Contribuye a**: CLAUDE.md refleja que backscroll indexa Claude Code, Pi y OpenCode

## Preserva

- INV1: `just check` pasa
  - Verificar: `just check`

## Contexto

`CLAUDE.md` línea 7 dice: "Backscroll is a Go CLI tool that indexes Claude Code sessions, plans, and external knowledge sources into SQLite for full-text search". Pi y OpenCode están omitidos en esta primera línea de descripción del proyecto, aunque más adelante (línea 64) el layout del módulo sí lista `JsonlReader, OpenCodeReader`.

Es un cambio de una línea que alinea la descripción del proyecto con la implementación real.

## Alcance

**In**:
1. Línea 7: `"indexes Claude Code sessions"` → `"indexes Claude Code, Pi, and OpenCode sessions"`

**Out**:
- No modificar el resto de CLAUDE.md
- No agregar secciones sobre readers

## Estado inicial esperado

- `grep "indexes Claude Code sessions" CLAUDE.md` encuentra match

## Criterios de Aceptación

- `grep "indexes Claude Code sessions" CLAUDE.md` retorna exit 1 (la frase vieja no existe)
- `grep "indexes Claude Code, Pi, and OpenCode sessions" CLAUDE.md` retorna exit 0
- `just check` pasa

## Fuente de verdad

- `CLAUDE.md` línea 7 — línea a modificar
