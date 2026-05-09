---
estado: Completed
tipo: task
---
# T010: Harden test command against user config

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE1, CE4, validación reproducible

[[blocked_by:./T009-isolate-cli-tests-from-user-config.md]]

## Preserva

- INV4: La configuración de aplicación y los inputs globales reales no deben ser prerequisitos de la suite.
  - Verificar: `just test` usa un config dir temporal o equivalente y no depende de `~/.config/backscroll`.
- INV2: La suite sigue ejercitando el pipeline genérico cuando los tests declaran manifests explícitos.
  - Verificar: tests con inputs continúan creando manifests en `<BACKSCROLL_CONFIG_DIR>/backscroll/inputs`.

## Contexto

Aunque T009 debe aislar los tests CLI, el comando estándar de desarrollo `just test` también debe ser robusto para evitar regresiones futuras y ejecuciones contaminadas por el entorno local. El fallo observado se reproduce cuando la suite hereda manifests activos reales y comandos CLI con autosync intentan indexar sesiones del usuario.

Esta task endurece la receta de test para que la ejecución completa sea hermética por defecto, sin impedir que tests específicos configuren sus propios manifests vía `.env()`.

## Alcance

**In**:
1. Actualizar la receta `test` del `Justfile` para ejecutar `cargo test --all-features` con `BACKSCROLL_CONFIG_DIR` apuntando a un tempdir aislado.
2. Asegurar que el tempdir se limpia o queda bajo una ruta temporal gestionada por el sistema.
3. Documentar en `AGENTS.md`, README de desarrollo o comentario del `Justfile` que los tests deben aislar config global del usuario.
4. Verificar que la receta sigue permitiendo que tests individuales sobrescriban `BACKSCROLL_CONFIG_DIR` con `.env()` cuando necesitan manifests fixtures.
5. Mantener `just check` enfocado en formato/clippy/check salvo necesidad justificada.

**Out**:
- Cambiar rutas de config runtime para usuarios finales.
- Desactivar manifests globales en el binario real.
- Reemplazar las regresiones de T009; esta task es defensa adicional, no sustituto.
- Introducir dependencias externas para crear tempdirs.

## Estado inicial esperado

- `just test` ejecuta `cargo test --all-features` sin aislar `BACKSCROLL_CONFIG_DIR`.
- La suite pasa si se invoca manualmente con `BACKSCROLL_CONFIG_DIR=$(mktemp -d)`.
- T009 ya corrigió los tests CLI que heredaban config real accidentalmente.

## Criterios de Aceptación

- `just test` pasa en una máquina con manifests activos en `~/.config/backscroll/inputs`.
- El output o implementación de la receta demuestra que `BACKSCROLL_CONFIG_DIR` apunta a un tempdir durante la ejecución.
- `cargo test --all-features` con `BACKSCROLL_CONFIG_DIR` temporal sigue pasando.
- La documentación de desarrollo indica que los tests no deben depender de config global real del usuario.
- No se modifica el comportamiento runtime de `backscroll inputs`, `sync`, `search` o `status`.

## Fuente de verdad

- `Justfile`
- `AGENTS.md`
- `README.md`
- `tests/cli.rs`
- `docs/roadmap/O03-global-user-scoped-inputs/T009-isolate-cli-tests-from-user-config.md`
