---
estado: Specified
tipo: task
---
# T002: Skip missing discovery roots

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE4

[[blocked_by:./T001-implement-os-aware-global-input-loader.md]]

## Preserva

- INV2: El pipeline interno sigue siendo genérico y manifest-driven.
  - Verificar: cambios se aplican a `DiscoverConfig`/input engine, no a reglas provider-specific.
- INV4: App config no aporta rutas de ingesta.
  - Verificar: missing roots se manejan dentro de manifests globales, no con fallback a `session_dirs`.

## Contexto

Los presets globales de Claude y Pi pueden estar instalados activos en máquinas donde una de las herramientas no existe. Por decisión de producto, roots inexistentes deben hacer skip/warn, no fail global. En el código actual, `src/input_config.rs` valida roots activos y falla si no existen, mientras `src/core/sync.rs` ya tiende a capturar errores de discovery por root y continuar. `src/core/reader.rs` puede abortar si un root ausente falla antes de encontrar otro root válido.

## Alcance

**In**:
1. Quitar hard-fail de validación de `discover.roots` inexistentes o ausentes en load-time.
2. Mantener errores fuertes para manifest inválido: TOML/schema/version/campos desconocidos/globs/selectors/regex/encoding.
3. Asegurar que `sync` salta roots inexistentes con warning y continúa con roots existentes.
4. Asegurar que `read` no aborta por un root inexistente si otro root del mismo input o de otro input descubre el archivo solicitado.
5. Agregar tests para roots mixtos existentes/inexistentes y roots inexistentes-only.
6. Definir el warning con `tracing::warn!` o salida clara existente sin hacer ruidoso cada comando si no hay logging configurado.

**Out**:
- Silenciar manifests inválidos.
- Fallback implícito a Claude/Pi.
- Cambiar semántica de include/exclude globs.

## Estado inicial esperado

- T001 cambió el loader a config dir global.
- `discover_candidate_files` todavía puede devolver error por root inexistente.
- `reader.rs` descubre candidates a partir del config completo y puede fallar temprano.

## Criterios de Aceptación

- `backscroll inputs validate` con `inputs/claude.inputs.toml` y root inexistente no falla solo por la ausencia del root.
- `backscroll sync` con un input que tiene un root inexistente y otro existente indexa el existente y no falla.
- `backscroll read <file>` funciona si un root válido descubre el archivo aunque otro root configurado no exista.
- Tests prueban que invalid glob/selector/regex siguen fallando en load/validate.
- No se introduce provider-specific handling para Claude/Pi.

## Fuente de verdad

- `src/input_config.rs`
- `src/core/sync.rs`
- `src/core/reader.rs`
- `tests/input_config.rs`
- `tests/cli.rs`
