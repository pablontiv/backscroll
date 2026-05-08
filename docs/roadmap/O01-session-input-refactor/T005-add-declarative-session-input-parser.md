---
estado: Completed
tipo: task
---
# T005: Implementar parser `source="session"` desde manifiestos

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Consumir rutas y banderas (`include_agents`) declarativamente desde inputs.

## Preserva

- INV1: Mantener identidad por `source_path` y hash para deduplicación incremental.
  - Verificar: índices existentes no se reinsertan innecesariamente.

## Contexto

`source="session"` debe convertirse en un parser declarativo que recorra rutas definidas por manifiesto, aplique `include_agents` y reutilice el parser `claude` nativo.

## Alcance

**In**:
1. Mapear cada entrada declarativa a una ejecución de parser `claude` con parámetros.
2. Resolver glob/paths de carpeta y archivos con filtro de extensión.
3. Exponer `active` para poder desactivar inputs sin editar configuración.

**Out**:
- Cambiar semántica de `source` para sesiones ya existentes.

## Estado inicial esperado

- El flujo de sesiones sólo acepta rutas directas por `session_dirs` o `--path`.

## Criterios de Aceptación

- Entrada inactiva no se procesa.
- `include_agents=true` incluye `/subagents/` y `false` lo filtra.
- Falta de ruta produce warning controlado y no aborta sync.

## Fuente de verdad

- `src/core/sync.rs`
- `src/core/session_inputs` (nueva capa)
