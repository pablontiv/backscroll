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

## Alcance

**In**:
1. Tests unitarios del engine: discovery, selectors, filters, text transforms.
2. Tests CLI con `claude.inputs.toml` y `pi.inputs.toml`.
3. Test de `read` vía manifest.
4. Test de `sync/search` vía manifest.
5. Test de invalid manifest fail-fast.
6. Test de filters downstream (`--source`, role/content/hybrid).
7. Snapshots de output normalizado si aporta estabilidad.

**Out**:
- Benchmarks extensos.
- Tests de plugin/script adapters.

## Estado inicial esperado

- Tests actuales cubren parser Claude/Pi hardcodeado y CLI legacy con `--path`/env.

## Criterios de Aceptación

- Ningún test principal requiere parser Claude/Pi hardcodeado.
- Fixtures prueban exclusión `subagents` por TOML y `think` por TOML.
- `cargo test` pasa completo.
- Los tests fallarían si se reintroduce parser implícito Claude en el flujo canónico.

## Fuente de verdad

- `tests/cli.rs`
- `tests/lib_api.rs`
- `src/core/sync.rs`
- `tests/fixtures/`
