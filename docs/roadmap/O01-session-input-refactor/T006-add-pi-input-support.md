---
estado: Specified
tipo: task
---
# T006: Añadir soporte inicial de input `pi`

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Validar extensibilidad del modelo más allá de Claude.

## Preserva

- INV1: Mantener parser de fuentes externas actuales independiente de este cambio.
  - Verificar: `--source` para fuentes externas sigue operativo.

## Contexto

Agregar un nuevo input nativo para `pi` con configuración y parser mínimo, sin usar integración ejecutable externa.

## Alcance

**In**:
1. Definir estructura esperada de `backscroll.inputs.toml` para `source = "pi"`.
2. Implementar parser nativo para estructura de logs/formatos de pi.
3. Añadir fixture mínimo/documentación.

**Out**:
- Implementación completa de transformaciones avanzadas propias de PI.

## Estado inicial esperado

- No hay soporte para otro source session además de la ruta directa de Claude.

## Criterios de Aceptación

- Un `backscroll.inputs.toml` con `source = "pi"` indexa al menos un fixture.
- El registro resultante tiene `source="session"` como salida interna.
- No rompe ruta de fallback y sync legacy.

## Fuente de verdad

- `src/config.rs`
- Nuevos fixtures en `tests/fixtures` o dir de tests
