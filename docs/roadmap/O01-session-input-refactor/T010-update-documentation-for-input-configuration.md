---
estado: Completed
tipo: task
---
# T010: Actualizar documentación y ejemplos de configuración

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Alinear usuario y operador con el nuevo flujo de inputs.

## Preserva

- INV1: No modificar la interpretación actual de documentos de sync para usuarios sin inputs.
  - Verificar: docs de compatibilidad y defaults explícitos.

## Contexto

Actualizar docs para incluir archivos `backscroll.inputs.toml` y carpeta `backscroll.inputs.d`, ejemplo de `claude`/`pi`, y explicación de precedencia.

## Alcance

**In**:
1. `README.md`: sección de compatibilidad y ejemplo de inputs.
2. `docs/configuration.md`: sección de resolución extendida.
3. `docs/sync.md`: flujo de discovery y precedencia.
4. `backscroll.toml.example`: nuevos keys de ejemplo.

**Out**:
- Reescritura de toda la documentación existente.

## Estado inicial esperado

- La configuración solo documenta `session_dir` y `session_dirs`.

## Criterios de Aceptación

- Documentación incluye `backscroll.inputs.toml` y `backscroll.inputs.d`.
- Se describen reglas de precedencia `--path` y fallback.
- `backscroll.toml.example` refleja compatibilidad de entrada y ruta legacy.

## Fuente de verdad

- `README.md`
- `docs/configuration.md`
- `docs/sync.md`
- `backscroll.toml.example`
