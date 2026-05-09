---
estado: Specified
tipo: task
---
# T001: Implement OS-aware global input loader

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE1, CE4, INV4

## Preserva

- INV1: Conversaciones Claude/Pi emiten `source = "session"`; el provider vive en TOML.
  - Verificar: no cambiar semántica de `InputDefinition.source` ni filtros downstream.
- INV4: `backscroll.toml` sigue siendo app config, no input config.
  - Verificar: tests sin manifests globales no indexan aunque existan `session_dirs` o cwd manifests.

## Contexto

El loader actual en `src/input_config.rs` carga manifests desde el cwd mediante `InputConfig::load_from_dir(Path::new("."))` y busca `*.inputs.toml` más `backscroll.inputs.d/*.toml`. La decisión final del producto es que Backscroll sea global/user-scoped: inputs canónicos viven únicamente en `<config_dir>/backscroll/inputs/*.inputs.toml`, con `<config_dir>` OS-aware (`dirs::config_dir()`) y override `BACKSCROLL_CONFIG_DIR`.

`InputConfig::load_from_dir` debe desaparecer para evitar una ruta pública de carga arbitraria/local. Los tests deberán usar el override de config dir, no un loader por directorio.

## Alcance

**In**:
1. Cambiar `InputConfig::load()` para resolver base config dir como `BACKSCROLL_CONFIG_DIR` si existe, o `dirs::config_dir()` si no.
2. Cargar solo archivos directos que terminen en `.inputs.toml` bajo `<config_dir>/backscroll/inputs/`, ordenados determinísticamente.
3. Si el directorio de inputs no existe, devolver config vacía sin error.
4. Eliminar `InputConfig::load_from_dir` como API pública y actualizar call sites/tests.
5. Mantener resolución de `discover.roots` relativos contra el directorio del manifest.
6. Eliminar cualquier discovery canónico de `./*.inputs.toml`, `./backscroll.inputs.d` o `./backscroll.inputs.toml`.
7. Actualizar mensajes de error/listado para mencionar el config dir global y `BACKSCROLL_CONFIG_DIR` como override.

**Out**:
- Instalar presets.
- Cambiar parser genérico.
- Remover legacy parser APIs; eso queda en T006.
- Cambiar app config `Config::load()` salvo referencias/documentación necesarias.

## Estado inicial esperado

- `src/input_config.rs` contiene `InputConfig::load_from_dir` y `manifest_paths_from_dir` cwd-local.
- `src/main.rs`, `src/core/reader.rs`, tests CLI y tests unitarios llaman `InputConfig::load()` o `load_from_dir`.
- `docs/roadmap/.stem` permite `On Hold`; no usar roadmapctl statuses como fuente de verdad de estados.

## Criterios de Aceptación

- `InputConfig::load_from_dir` no existe como API pública.
- `InputConfig::load()` no inspecciona cwd ni `backscroll.inputs.d`.
- Con `BACKSCROLL_CONFIG_DIR=/tmp/cfg`, el loader lee `/tmp/cfg/backscroll/inputs/*.inputs.toml`.
- Si `/tmp/cfg/backscroll/inputs` no existe, `InputConfig::load()` retorna cero manifests/inputs sin error.
- Un manifest inválido en el config dir falla con path claro.
- Manifests locales poison en cwd son ignorados por `inputs validate`, `sync`, `read` y autosync.

## Fuente de verdad

- `docs/roadmap/O03-global-user-scoped-inputs/README.md`
- `src/input_config.rs`
- `src/main.rs`
- `src/core/reader.rs`
- `tests/input_config.rs`
- `tests/cli.rs`
- `tests/lib_api.rs`
