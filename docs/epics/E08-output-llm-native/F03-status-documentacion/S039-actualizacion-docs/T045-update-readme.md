---
estado: Pending
tipo: docs
ejecutable_en: 1 sesion
---
# T045: Actualizar README.md con usage y quick start

**Story**: [S039 Actualizacion de documentacion](README.md)
**Contribuye a**: README.md tiene usage con nuevos flags

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`

## Contexto

README.md es la documentacion publica del proyecto. Debe reflejar todas las capacidades implementadas en E06-E08: nuevos comandos, flags, y ejemplos de uso.

## Alcance

**In**:
1. Actualizar seccion Usage/Quick Start con ejemplos de todos los comandos
2. Documentar: `sync [--path] [--include-agents]`, `search <query> [--project] [--json] [--robot] [--fields] [--max-tokens]`, `read <path>`, `status`
3. Agregar ejemplos de uso con LLMs (pipe a jq, --max-tokens)

**Out**: No modificar CLAUDE.md (T044).

## Estado inicial esperado

- README.md desactualizado
- Todos los cambios de E06-E08 implementados

## Criterios de Aceptacion

- README.md documenta todos los comandos: sync, search, read, status
- README.md tiene ejemplos de --json, --robot, --max-tokens
- `just check` pasa (no afecta codigo)

## Fuente de verdad

- `README.md`
