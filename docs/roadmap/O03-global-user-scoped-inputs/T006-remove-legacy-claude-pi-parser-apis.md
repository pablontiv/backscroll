---
estado: Completed
tipo: task
---
# T006: Remove legacy Claude/Pi parser APIs

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE3, INV2

[[blocked_by:./T001-implement-os-aware-global-input-loader.md]]

## Preserva

- INV1: Claude/Pi siguen soportados por manifests TOML con `source = "session"`.
  - Verificar: presets shipped y tests genéricos cubren Claude/Pi sin parsers hardcodeados.
- INV2: El único camino canónico de ingesta es el input engine genérico.
  - Verificar: no quedan call sites runtime hacia parsers legacy.

## Contexto

Aunque O02 migró el flujo principal a manifests, el código aún conserva APIs/parsers legacy Claude/Pi: `SessionInput`, registry de parsers, `ClaudeInputParser`, `PiInputParser`, `parse_session_inputs`, `parse_legacy_claude_sessions`, `core/session_inputs` y helpers hardcodeados como `filter_noise`. La decisión final pide limpieza total para que Backscroll sea realmente agnóstico.

Esta task puede ser breaking para consumidores de librería legacy, pero ese es el objetivo del cierre O03.

## Alcance

**In**:
1. Remover `SessionInput` y conversiones legacy desde `src/input_config.rs`.
2. Remover `active_session_inputs()` y `InputDefinition::to_legacy_session_input()` si existen.
3. Eliminar `src/core/session_inputs/` y su export en `src/core/mod.rs`.
4. Eliminar parsers hardcodeados Claude/Pi y registry desde `src/core/sync.rs`.
5. Eliminar `parse_session_inputs`, `parse_legacy_claude_sessions`, `ClaudeInputParser`, `PiInputParser`, `SessionInputParserRegistry` y tipos asociados.
6. Eliminar `filter_noise` si solo existe como helper legacy Claude-specific; su comportamiento debe vivir en `[inputs.text].remove` del preset Claude.
7. Eliminar `src/core/models.rs` si queda sin uso tras remover parser Claude legacy.
8. Actualizar tests/lib API para ejercitar solo APIs genéricas.

**Out**:
- Remover `core::sources` u otras APIs de documentos no relacionadas, salvo dependencia directa inevitable.
- Cambiar la estructura de `ParsedFile`/`ParsedMessage`.
- Reintroducir semántica Claude/Pi en Rust bajo otro nombre.

## Estado inicial esperado

- T001 cambió loader canónico.
- Presets TOML cubren comportamiento Claude/Pi suficiente para mantener soporte.
- Tests legacy todavía pueden fallar hasta ser removidos/migrados por T005.

## Criterios de Aceptación

- `rg "SessionInput|SessionInputParser|ClaudeInputParser|PiInputParser|parse_session_inputs|parse_legacy_claude_sessions|parse_session_file_claude|parse_pi_file" src tests` no encuentra runtime/tests legacy, salvo menciones históricas explícitamente aceptadas en docs/roadmap.
- `filter_noise` no se exporta como API pública si era solo legacy Claude-specific.
- `cargo clippy --all-targets --all-features -- -D warnings` no reporta código muerto tras la eliminación.
- Tests de presets TOML demuestran que Claude/Pi siguen indexándose por el engine genérico.

## Fuente de verdad

- `src/input_config.rs`
- `src/core/sync.rs`
- `src/core/session_inputs/`
- `src/core/mod.rs`
- `src/core/models.rs`
- `tests/lib_api.rs`
- `tests/input_config.rs`
- `tests/cli.rs`
- `docs/input-contract.md`
