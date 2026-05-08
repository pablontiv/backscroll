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
  - Verificar: tooling solo valida/interpreta manifests locales.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: validación no requiere JMESPath.

## Contexto

Al mover semántica a TOML, errores de config pasan a ser parte central de UX. Herramientas maduras como Vector/OTel/Redpanda tienen validate/dry-run/lint.

## Alcance

**In**:
1. Agregar subcomandos `backscroll inputs list`, `backscroll inputs validate`, `backscroll inputs test`.
2. Validar TOML, campos requeridos, parsers/formatos, globs, JSONPath, regex y operadores.
3. Dry-run sobre sample file mostrando records leídos, dropeados y mensajes emitidos.
4. Mostrar razones de drop y errores por línea.
5. Asegurar salida machine-readable si aplica (`--json`).

**Out**:
- UI interactiva.
- Plugins externos.

## Estado inicial esperado

- Config parse failures de input se ignoran con warnings en algunos casos.
- No hay comando específico para validar inputs.

## Criterios de Aceptación

- Manifest inválido falla con mensaje accionable.
- `inputs test` muestra output normalizado compatible con `ParsedMessage`.
- Tests CLI cubren validación exitosa, selector inválido y regex inválida.

## Fuente de verdad

- `src/main.rs`
- `src/config.rs`
- `src/core/sync.rs`
- `tests/cli.rs`
