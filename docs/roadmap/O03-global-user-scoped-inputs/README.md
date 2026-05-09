---
estado: Pending
tipo: outcome
---
# O03: Global user-scoped inputs

## Objetivo

Completar la refactorización del motor genérico agnóstico de Backscroll para que la ingesta canónica dependa exclusivamente de manifests declarativos instalados en la configuración de usuario OS-aware, sin manifests locales por repo ni parsers hardcodeados Claude/Pi.

## Criterios de Éxito

- CE1: `InputConfig::load()` carga inputs solo desde `<config_dir>/backscroll/inputs/*.inputs.toml`, donde `<config_dir>` es `BACKSCROLL_CONFIG_DIR` si está definido o `dirs::config_dir()` según el OS.
  - Verificar: tests con `BACKSCROLL_CONFIG_DIR` pasan y tests con manifests locales poison prueban que cwd se ignora.
- CE2: Backscroll instala presets base de Claude y Pi como archivos TOML versionados del repo en el config dir del usuario.
  - Verificar: `install.sh`, `install.ps1` y hooks copian `inputs/claude.inputs.toml` y `inputs/pi.inputs.toml` sin sobrescribir por defecto.
- CE3: El runtime canónico no conserva APIs/parsers legacy específicos de Claude/Pi.
  - Verificar: grep no encuentra `SessionInputParser`, `ClaudeInputParser`, `PiInputParser`, `parse_session_inputs` ni `parse_legacy_claude_sessions` en código runtime.
- CE4: Roots inexistentes se saltan sin romper validación/sync/read; manifests inválidos siguen fallando claramente.
  - Verificar: tests cubren missing roots, invalid TOML/schema/selectors/globs/regex y `backscroll read` con roots mixtos.
- CE5: Documentación, skill y ejemplos describen solo el modelo global/user-scoped.
  - Verificar: grep no encuentra referencias canónicas actuales a `./*.inputs.toml`, `backscroll.inputs.d`, `backscroll.inputs.toml`, `sync --path` ni `--include-agents`.

## Invariantes

- INV1: Conversaciones Claude/Pi emiten `source = "session"`; el provider vive en `id`, selectors y predicates del TOML.
  - Verificar: tests de presets producen `ParsedFile.source == "session"`.
- INV2: El pipeline interno sigue siendo genérico: discover → decode → record/filter → map → content/text → emit `ParsedFile`/`ParsedMessage`.
  - Verificar: `sync`, autosync, `read` e `inputs test` usan `parse_input_definitions`/input engine y no parsers provider-specific.
- INV3: No se agregan plugins/scripts ejecutables ni JMESPath como dependencia de este MVP.
  - Verificar: `Cargo.toml` no agrega `jmespath` y los manifests no incluyen comandos ejecutables.
- INV4: `backscroll.toml` permanece como configuración de aplicación, no como fuente de rutas de ingesta canónicas.
  - Verificar: tests prueban que app config/session_dirs no indexan sin manifests globales.

## Alcance

**In**:
- Loader OS-aware user-scoped para manifests `*.inputs.toml`.
- Override `BACKSCROLL_CONFIG_DIR` para tests/casos avanzados.
- Presets versionados `inputs/claude.inputs.toml` y `inputs/pi.inputs.toml`.
- Instalación de binary + presets en scripts y hooks.
- Skip/warn de roots inexistentes.
- Migración de tests a config dir global.
- Eliminación de parser APIs legacy Claude/Pi.
- Actualización de docs, README, skill y ejemplos.

**Out**:
- JMESPath.
- Plugins/adapters ejecutables.
- Cambios de esquema SQLite obligatorios.
- Reintroducir `--path`, `session_dirs`, implicit Claude fallback o manifests cwd como ruta canónica.
- Remover APIs de document sources no relacionadas salvo que queden directamente acopladas a legacy Claude/Pi.

## Tasks

| Task | Descripción |
|------|-------------|
| [T001](T001-implement-os-aware-global-input-loader.md) | Implementar loader global OS-aware para input manifests |
| [T002](T002-skip-missing-discovery-roots.md) | Saltar roots inexistentes sin romper sync/read/validate |
| [T003](T003-add-shipped-claude-and-pi-input-presets.md) | Agregar presets versionados Claude y Pi |
| [T004](T004-install-input-presets-with-binary.md) | Instalar presets junto con binario en scripts/hooks |
| [T005](T005-migrate-tests-to-global-config-inputs.md) | Migrar tests a manifests globales y cubrir regresiones |
| [T006](T006-remove-legacy-claude-pi-parser-apis.md) | Eliminar APIs/parsers legacy Claude/Pi |
| [T007](T007-update-docs-skill-and-examples.md) | Actualizar docs, skill y ejemplos al modelo global |
| [T008](T008-run-validation-and-smoke-tests.md) | Ejecutar validación completa y smoke tests |
| [T009](T009-isolate-cli-tests-from-user-config.md) | Aislar tests CLI de la config global real del usuario |
| [T010](T010-harden-test-command-against-user-config.md) | Endurecer `just test` contra contaminación de config global |
