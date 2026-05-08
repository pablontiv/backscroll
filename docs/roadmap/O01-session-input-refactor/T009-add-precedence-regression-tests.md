---
estado: Completed
tipo: task
---
# T009: Añadir tests de precedencia y compatibilidad

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Asegurar reglas de precedencia y no regresión del comportamiento.

## Preserva

- INV1: Mantener rutas de `--path` con máxima prioridad.
  - Verificar: tests automatizados.

## Contexto

Agregar pruebas en `core/sync` y CLI que cubran explícitamente:
- `--path` sobre configuración e inputs
- config sobre inputs y fallback
- comportamiento con inputs inválidos.

## Alcance

**In**:
1. Tests unitarios de resolución de rutas.
2. Tests CLI de sincronización usando distintos manifiestos.
3. Snapshots/regresiones para parsing de ruido y mensajes textuales.

**Out**:
- Cambios de cobertura fuera del alcance de sesiones.

## Estado inicial esperado

- No hay tests explícitos de precedencia ni de inputs.

## Criterios de Aceptación

- Suite completa pasa en condiciones de CI.
- Al fallar un archivo individual no aborta todo el sync.
- Resultados de búsqueda siguen incluyendo sesiones existentes.

## Fuente de verdad

- `tests/cli.rs`
- `src/core/sync.rs`
