---
estado: Completed
tipo: task
---
# T008: Run validation and smoke tests

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE1, CE2, CE3, CE4, CE5

[[blocked_by:./T004-install-input-presets-with-binary.md]]
[[blocked_by:./T005-migrate-tests-to-global-config-inputs.md]]
[[blocked_by:./T006-remove-legacy-claude-pi-parser-apis.md]]
[[blocked_by:./T007-update-docs-skill-and-examples.md]]

## Preserva

- INV1: Claude/Pi siguen indexándose como `source = "session"`.
  - Verificar: tests/smoke de presets y search source `sessions`.
- INV2: Pipeline genérico único.
  - Verificar: grep anti-regresión no encuentra parsers legacy ni local loaders.
- INV3: Sin JMESPath/plugins.
  - Verificar: `Cargo.toml` y docs no agregan esas dependencias/features.
- INV4: App config no define inputs.
  - Verificar: tests no-fallback pasan.

## Contexto

Esta task cierra O03. Debe ejecutar validación técnica, install-script tests, smoke manual con `BACKSCROLL_CONFIG_DIR`, y greps anti-regresión antes de marcar el Outcome como completado. No debe implementar features nuevas salvo arreglos necesarios para que las tasks previas cumplan sus ACs.

## Alcance

**In**:
1. Ejecutar formato, clippy y tests completos.
2. Ejecutar tests de install scripts Bash y, si está disponible, PowerShell/Pester.
3. Ejecutar smoke con config dir temporal copiando `inputs/claude.inputs.toml`/`inputs/pi.inputs.toml`.
4. Ejecutar greps anti-regresión para loader local y parsers legacy.
5. Ejecutar `rootline validate --all docs/roadmap/`, `rootline graph docs/roadmap/ --check` y `roadmapctl check --repo /home/shared/harness/backscroll --roadmap-root docs/roadmap --output json --strict`.
6. Documentar cualquier warning aceptable y por qué no bloquea.

**Out**:
- Cambios de comportamiento no cubiertos por O03.
- Saltarse install-script tests si el entorno los soporta.
- Declarar éxito sin evidencia de comandos.

## Estado inicial esperado

- T004, T005, T006 y T007 están completadas.
- Los scripts instalan presets.
- Tests ya usan config dir global.
- Legacy parser APIs ya fueron removidas.

## Criterios de Aceptación

- Pasan:
  ```bash
  cargo fmt --all -- --check
  cargo clippy --all-targets --all-features -- -D warnings
  cargo test --all-features
  bash tests/test-install.sh
  ```
- Si PowerShell/Pester está disponible, pasa `Invoke-Pester tests/test-install.ps1`; si no, se reporta explícitamente.
- Smoke con `BACKSCROLL_CONFIG_DIR=$(mktemp -d)` demuestra `backscroll inputs list --json` y `backscroll inputs validate --json` con presets copiados.
- Grep anti-regresión no encuentra `load_from_dir`, `SessionInputParser`, `ClaudeInputParser`, `PiInputParser`, `parse_session_inputs`, `parse_legacy_claude_sessions`, ni docs canónicas de manifests cwd.
- Rootline y roadmapctl validan el roadmap.

## Fuente de verdad

- `Justfile`
- `tests/test-install.sh`
- `tests/test-install.ps1`
- `inputs/claude.inputs.toml`
- `inputs/pi.inputs.toml`
- `src/input_config.rs`
- `src/core/sync.rs`
- `README.md`
- `docs/`
- `.claude/skills/backscroll/SKILL.md`
