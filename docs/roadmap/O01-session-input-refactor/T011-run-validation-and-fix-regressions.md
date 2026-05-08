---
estado: Specified
tipo: task
---
# T011: Ejecutar verificación y cerrar cambios

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Entregar resultado estable y sin regresiones.

## Preserva

- INV1: Los comandos CLI existentes conservan sintaxis y salidas esperadas fuera de cambios de feature.
  - Verificar: regresión en `tests/cli.rs` y smoke de comandos clave.

## Contexto

Al finalizar implementar plan, correr verificación completa y registrar estado antes de merge/transferencia a implementación.

## Alcance

**In**:
1. Ejecutar `just check` y `just test`.
2. Corregir errores y volver a ejecutar.
3. Preparar checklist final para implementación.

**Out**:
- Merge sin pasar validaciones.

## Estado inicial esperado

- No se han ejecutado verificaciones de CI-locales.

## Criterios de Aceptación

- `just check` y `just test` exitosos en entorno local.
- Se documenta cualquier fallo residual de forma explícita.
- Estado final de archivos de roadmap preparado para commit.

## Fuente de verdad

- `Justfile`
- `tests/**`
- `src/**`
