---
estado: Specified
tipo: task
---
# T002: Integrar carga y normalización de inputs en Config

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Mantener compatibilidad y orden de configuración al incorporar manifiestos.

## Preserva

- INV1: Mantener compatibilidad con `session_dir` y `session_dirs`.
  - Verificar: los tests existentes que usan variables env siguen pasando.

## Contexto

`Config::load` debe extenderse para leer manifiestos `backscroll.inputs.toml` y `backscroll.inputs.d/*.toml`, normalizar entradas y mantener defaults actuales cuando no existan.

## Alcance

**In**:
1. Agregar struct(s) y deserialización para config de inputs.
2. Orden de precedencia: config explícita > inputs declarativos > defaults.
3. Exponer configuración unificada para el resolver de rutas/sync.

**Out**:
- Cambios en esquema SQL o CLI.

## Estado inicial esperado

- `Config` actualmente sólo usa `session_dirs` y no acepta manifiestos de inputs.

## Criterios de Aceptación

- Config legacy sigue funcionando sin archivos de inputs.
- Se puede cargar `backscroll.inputs.toml` con rutas activas y desactivadas.
- `resolve_session_paths` consume la config resultante correctamente.

## Fuente de verdad

- `src/config.rs`
- `src/main.rs`
- `tests/cli.rs`
