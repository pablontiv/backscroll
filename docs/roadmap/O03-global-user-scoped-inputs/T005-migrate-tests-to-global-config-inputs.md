---
estado: Specified
tipo: task
---
# T005: Migrate tests to global config inputs

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE1, CE4

[[blocked_by:./T001-implement-os-aware-global-input-loader.md]]
[[blocked_by:./T002-skip-missing-discovery-roots.md]]
[[blocked_by:./T003-add-shipped-claude-and-pi-input-presets.md]]

## Preserva

- INV2: Tests deben ejercitar el mismo engine genérico que runtime.
  - Verificar: tests de sync/read usan manifests globales + `parse_input_definitions`, no parsers legacy.
- INV4: App config no contribuye input roots canónicos.
  - Verificar: tests con `BACKSCROLL_SESSION_DIR`, `session_dirs` o cwd manifests sin global inputs no indexan.

## Contexto

Los tests actuales crean manifests en cwd/temp dirs y usan `InputConfig::load_from_dir`. Con O03, los tests deben modelar el runtime real: manifests bajo `<BACKSCROLL_CONFIG_DIR>/backscroll/inputs/*.inputs.toml`. En Rust 2024, mutar env global con `std::env::set_var` es unsafe y el crate deniega unsafe; por eso los tests CLI deben usar `.env()` en `assert_cmd`, y los tests unitarios deben preferir helpers puros o construir configs sin mutar env global cuando sea posible.

## Alcance

**In**:
1. Migrar `tests/input_config.rs` a helpers que escriban manifests en `<temp>/backscroll/inputs` y ejerciten `InputConfig::load()` con override seguro.
2. Migrar `tests/cli.rs` para pasar `.env("BACKSCROLL_CONFIG_DIR", temp_base)` en cada command relevante.
3. Migrar `tests/lib_api.rs` eliminando dependencia de `load_from_dir` y APIs legacy.
4. Agregar regresiones negativas donde cwd contiene `claude.inputs.toml`, `backscroll.inputs.toml` o `backscroll.inputs.d/broken.toml` y la CLI los ignora.
5. Agregar regresiones positivas donde comandos desde cwd arbitrario leen global config inputs.
6. Cubrir invalid global manifest, missing roots, roots mixtos y `inputs test` sin DB.
7. Remover `#![allow(deprecated)]` si ya no hay APIs deprecadas usadas.

**Out**:
- Usar home real del desarrollador en tests.
- Reintroducir `load_from_dir` solo para tests.
- Usar unsafe env mutation salvo que se justifique y se cambie la policy del crate.

## Estado inicial esperado

- T001 eliminó `InputConfig::load_from_dir`.
- T002 cambió missing roots.
- T003 agregó presets shipped.
- Tests existentes todavía pueden depender de cwd manifests y fixtures directos.

## Criterios de Aceptación

- `cargo test --test input_config` pasa sin `load_from_dir`.
- `cargo test --test cli inputs` y `cargo test --test cli sync` pasan usando `BACKSCROLL_CONFIG_DIR`.
- Tests prueban explícitamente que manifests locales/cwd son ignorados.
- Tests prueban que invalid manifests globales fallan claramente.
- Tests prueban que missing roots no rompen `validate`, `sync` ni `read` en los casos definidos.

## Fuente de verdad

- `tests/input_config.rs`
- `tests/cli.rs`
- `tests/lib_api.rs`
- `src/input_config.rs`
- `src/core/sync.rs`
- `src/core/reader.rs`
