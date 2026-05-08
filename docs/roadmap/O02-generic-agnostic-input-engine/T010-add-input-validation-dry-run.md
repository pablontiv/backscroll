---
estado: Specified
tipo: task
---
# T010: Add input validation and dry-run tooling

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE2, CE3

[[blocked_by:./T005-implement-declarative-filters-transforms.md]]

## Preserva

- INV3: No se introducen plugins/scripts ejecutables para el MVP.
  - Verificar: tooling solo valida/interpreta manifests locales en proceso.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: validación no requiere JMESPath.

## Contexto

Al mover semántica a TOML, errores de config pasan a ser parte central de UX. El contrato final exige que campos desconocidos, required fields ausentes, operadores inválidos y selectors inválidos fallen claramente.

La policy mínima para flujos manifest-driven es fail-fast para manifests requeridos/activos inválidos. Esta task agrega UX detallada de validación y dry-run encima de esa policy.

## Alcance

**In**:
1. Agregar tooling CLI bajo `backscroll inputs` para listar, validar y probar/dry-run manifests.
2. Validar TOML, `version`, `[[inputs]]`, campos requeridos y campos desconocidos en todos los niveles.
3. Validar `discover.roots`, `include`, `exclude`, `follow_symlinks`, `decode.format`, encoding MVP y `source` explícito.
4. Validar selectors JSONPath en `record`, `map` y `content`.
5. Validar predicados con operadores MVP `eq`, `ne`, `in`, `exists`, `missing`.
6. Validar regexes y reglas `text.remove` (`regex`, `prefix`, `suffix`).
7. Dry-run sobre sample file mostrando records leídos, records/blocks descartados, razones de drop y mensajes normalizados compatibles con `ParsedMessage`.
8. Asegurar salida machine-readable si aplica (`--json`).

**Out**:
- UI interactiva.
- Plugins externos.
- JMESPath o extensiones fuera del contrato MVP.

## Estado inicial esperado

- Config parse failures de input no tienen UX específica suficiente.
- No hay comando específico para validar inputs.

## Criterios de Aceptación

- Manifest inválido falla con mensaje accionable y ruta/campo relevante.
- Manifests activos inválidos fallan antes de sync/autosync/read manifest-driven.
- Dry-run muestra output normalizado compatible con `ParsedMessage` sin escribir en SQLite.
- Tests CLI cubren validación exitosa, selector inválido, regex inválida, campo desconocido y operador inválido.
- La validación usa JSONPath y no introduce JMESPath como dependencia MVP.

## Fuente de verdad

- `docs/input-contract.md`
- `src/main.rs`
- `src/config.rs`
- `src/core/sync.rs`
- `tests/cli.rs`
