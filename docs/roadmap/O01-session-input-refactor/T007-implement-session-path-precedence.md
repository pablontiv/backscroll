---
estado: Specified
tipo: task
---
# T007: Ajustar resolución de rutas con precedencia

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Garantizar que `--path` siga teniendo prioridad absoluta.

## Preserva

- INV1: Precedencia actual no debe cambiar: `--path` primero.
  - Verificar: test dedicado de resolución de paths.

## Contexto

Reorganizar la lógica de elección de rutas en `main.rs`/sync para evaluar secuencialmente:
1) `--path`, 2) config explícita, 3) inputs declarativos, 4) auto-discovery.

## Alcance

**In**:
1. Refactorizar función de resolución de sesiones.
2. Integrar rutas provenientes de inputs.
3. Añadir trazas útiles en logs para resolución.

**Out**:
- Cambiar semántica de deduplicación o hash.

## Estado inicial esperado

- La resolución está acoplada hoy a `session_dirs` y fallback.

## Criterios de Aceptación

- Caso `--path` + config + inputs elige `--path`.
- Caso sin `--path`, sin config explícita y con inputs usa inputs.
- Caso sin todo usa fallback de `~/.claude/projects`.

## Fuente de verdad

- `src/main.rs`
- `src/config.rs`
