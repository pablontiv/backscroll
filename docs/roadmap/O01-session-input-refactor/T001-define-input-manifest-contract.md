---
estado: Completed
tipo: task
---
# T001: Definir contrato TOML para inputs externos

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Base estable y reusable para agregar nuevos sources sin recompilar.

## Preserva

- INV1: No introducir reglas de parsing acopladas a CLI específicas en esta capa.
  - Verificar: el parser de sesiones sigue recibiendo estructura interna `ParsedFile`.

## Contexto

Se requiere un contrato de configuración para `backscroll.inputs.toml` y `backscroll.inputs.d/*.toml` con campos mínimos para declarar entradas de sesión, incluyendo `source`, `parser`, `paths`, `include_agents`, `active`, `glob` o filtros declarativos.

## Alcance

**In**:
1. Definir estructura TOML tipada en código y validación.
2. Soportar herencia/merge de múltiples archivos `inputs.d`.
3. Definir defaults de compatibilidad para sesiones y fallback.

**Out**:
- Ejecutar parsers de sesiones fuera de Rust.

## Estado inicial esperado

- No existen tipos ni carga de inputs para `source` declarativos.

## Criterios de Aceptación

- Existe documentación mínima del esquema en ejemplo de configuración.
- Se detecta archivo inválido con error de parse controlado.
- El contrato soporta declarar múltiples entradas activas y orden de evaluación.

## Fuente de verdad

- `src/config.rs`
- `backscroll.toml.example`
- Nuevos tests de configuración
