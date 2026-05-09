---
estado: Completed
tipo: task
---
# T003: Add shipped Claude and Pi input presets

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE2, INV1, INV2

## Preserva

- INV1: Claude y Pi son conversaciones con `source = "session"`.
  - Verificar: ambos presets tienen `source = "session"` y tests producen `ParsedFile.source == "session"`.
- INV3: No se agregan plugins/scripts ejecutables ni JMESPath.
  - Verificar: presets solo usan discovery, decode, selectors, predicates, mapping y text normalization.

## Contexto

El repo hoy tiene presets como fixtures (`tests/fixtures/claude-preset/claude.inputs.toml`, `tests/fixtures/pi.inputs.toml`) y puede existir un manifest local no trackeado `backscroll.inputs.d/local-sessions.toml`. La decisión final exige que el repo shippee presets base reales para instalación de usuario: `inputs/claude.inputs.toml` y `inputs/pi.inputs.toml`.

Los presets deben ser data-only. Toda semántica específica de Claude/Pi (subagents, noise tags, think/tool blocks, mappings) debe vivir en TOML, no en Rust.

## Alcance

**In**:
1. Crear directorio trackeado `inputs/` si no existe.
2. Crear `inputs/claude.inputs.toml` con root default `~/.claude/projects`, JSONL, filtros user/assistant, exclusión subagents por glob, mapping Claude y regexes de noise tags en `[inputs.text].remove`.
3. Crear `inputs/pi.inputs.toml` con root default real de Pi. Usar `~/.pi/agent/sessions` salvo evidencia local/actual que indique otro path canónico.
4. Mantener ambos presets `active = true`, confiando en T002 para skip de roots inexistentes.
5. Alinear fixtures/tests para reutilizar o comparar contra los presets shipped cuando sea práctico.
6. Documentar en comentarios o docs que los usuarios pueden editar/copiar estos manifests en su config dir.

**Out**:
- Auto-generar manifests dinámicamente.
- Leer manifests desde `inputs/` en runtime; `inputs/` es fuente de instalación, no config runtime.
- Cambiar storage schema.

## Estado inicial esperado

- T001/T002 pueden no estar implementados aún, pero esta task puede crear los archivos fuente de presets.
- El fixture Claude usa root relativo para tests; el preset shipped debe usar root user-home.
- El path Pi debe confirmarse con datos locales o decisión explícita antes de finalizar si hay duda.

## Criterios de Aceptación

- Existen `inputs/claude.inputs.toml` y `inputs/pi.inputs.toml` trackeados.
- Ambos pasan `backscroll inputs validate` cuando se copian al config dir de test y sus roots existen o se saltan por T002.
- Tests prueban que ambos presets producen `source = "session"`.
- Grep no encuentra semánticas Claude/Pi equivalentes reintroducidas en Rust core como decisiones hardcodeadas.

## Fuente de verdad

- `inputs/claude.inputs.toml`
- `inputs/pi.inputs.toml`
- `tests/fixtures/claude-preset/claude.inputs.toml`
- `tests/fixtures/pi.inputs.toml`
- `docs/input-contract.md`
- `src/core/sync.rs`
