---
estado: Specified
tipo: task
---
# T012: Add comprehensive manifest-driven regression tests

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE1, CE2, CE3, CE4

[[blocked_by:./T006-create-claude-input-preset.md]]
[[blocked_by:./T007-create-pi-input-preset.md]]
[[blocked_by:./T010-add-input-validation-dry-run.md]]
[[blocked_by:./T011-make-search-filters-generic.md]]

## Preserva

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: regresiones Claude/Pi assertan source session.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: tests MVP no requieren JMESPath.

## Contexto

El cambio central desplaza semántica de Rust a TOML; la suite debe probar manifests reales, no solo funciones internas.

Las regresiones deben demostrar que el flujo O02 es TOML-only: no `--path`, no `session_dirs` como fuente canónica, no fallback Claude/Pi implícito y no parsers provider-specific en el camino principal.

## Alcance

**In**:
1. Tests unitarios del engine: discovery, selectors JSONPath, predicates, content selection y text transforms.
2. Tests CLI con `claude.inputs.toml` y `pi.inputs.toml`.
3. Test de `read` vía manifest.
4. Test de `sync/search` vía manifest.
5. Test de invalid manifest fail-fast para sync/autosync/read manifest-driven.
6. Tests de separación app config vs input config: `backscroll.toml` no aporta rutas de ingesta canónicas.
7. Tests de ausencia de fallback: sin manifest activo no se asume Claude/Pi.
8. Test de filters downstream (`--source`, role/content/hybrid) cuando corresponda al estado de T011.
9. Snapshots de output normalizado si aporta estabilidad.

**Out**:
- Benchmarks extensos.
- Tests de plugin adapters.
- Tests que dependan de JMESPath.

## Estado inicial esperado

- Tests actuales cubren parser Claude/Pi hardcodeado y CLI legacy con `--path`/env.

## Criterios de Aceptación

- Ningún test principal requiere parser Claude/Pi hardcodeado.
- Fixtures prueban exclusión `subagents` por TOML y `think` por TOML.
- Tests fallan si se reintroduce parser implícito Claude/Pi en el flujo canónico.
- Tests cubren que `source = "session"` se conserva para Claude/Pi.
- `cargo test` pasa completo.

## Fuente de verdad

- `docs/input-contract.md`
- `tests/cli.rs`
- `tests/lib_api.rs`
- `src/core/sync.rs`
- `tests/fixtures/`
