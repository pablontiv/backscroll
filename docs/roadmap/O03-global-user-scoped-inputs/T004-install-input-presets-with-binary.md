---
estado: Specified
tipo: task
---
# T004: Install input presets with binary

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE2

[[blocked_by:./T003-add-shipped-claude-and-pi-input-presets.md]]

## Preserva

- INV4: Runtime input config vive en el config dir de usuario, no en el repo.
  - Verificar: scripts copian presets a `<config_dir>/backscroll/inputs`, no configuran cwd manifests.

## Contexto

`install.sh` e `install.ps1` instalan el binario pero no los input presets. Los hooks `.githooks/pre-push` y `.githooks/post-merge` copian binario/skill con lógica duplicada. El modelo final requiere que instalar Backscroll deje al usuario con binario e inputs base disponibles en la ruta OS-aware.

La ruta debe coincidir con T001: `BACKSCROLL_CONFIG_DIR/backscroll/inputs` si el override existe; si no, `dirs::config_dir()` equivalente por OS.

## Alcance

**In**:
1. Actualizar `install.sh` para instalar binario y copiar/fetchear `inputs/claude.inputs.toml` y `inputs/pi.inputs.toml` al config dir OS-aware.
2. Actualizar `install.ps1` para Windows usando `%APPDATA%` como config dir por defecto y `BACKSCROLL_CONFIG_DIR` como override.
3. No sobrescribir manifests existentes por defecto; imprimir mensaje `exists, skipping` o equivalente.
4. Si se agrega force opt-in, usar variable explícita como `BACKSCROLL_FORCE_INPUTS=1` y cubrirla en tests.
5. Actualizar `.githooks/pre-push` y `.githooks/post-merge` para llamar lógica compartida o copiar presets desde `inputs/` sin red.
6. Actualizar tests de install scripts para comprobar instalación/copia de inputs y resolución de config dir.

**Out**:
- Sobrescribir manifests de usuario sin opt-in.
- Network calls desde git hooks.
- Crear manifests locales en el repo.
- Reestructurar release assets si no hace falta.

## Estado inicial esperado

- T003 creó `inputs/claude.inputs.toml` y `inputs/pi.inputs.toml`.
- `install.sh` descarga binario a user bin.
- `install.ps1` instala binario en Windows.
- Hooks ya instalan skill/binario pero no inputs.

## Criterios de Aceptación

- `bash tests/test-install.sh` cubre que `install.sh` crea/copiaría los inputs al config dir esperado.
- `tests/test-install.ps1` cubre resolución/copia de inputs en Windows o al menos sintaxis y paths esperados según el patrón existente.
- Hooks copian `inputs/*.inputs.toml` a la config de usuario sin sobrescribir por defecto.
- Los scripts documentan o muestran la ruta de inputs instalada.
- No se introducen manifests locales/cwd como parte de instalación.

## Fuente de verdad

- `install.sh`
- `install.ps1`
- `tests/test-install.sh`
- `tests/test-install.ps1`
- `.githooks/pre-push`
- `.githooks/post-merge`
- `inputs/claude.inputs.toml`
- `inputs/pi.inputs.toml`
