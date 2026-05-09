---
estado: Specified
tipo: task
---
# T009: Isolate CLI tests from user config

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE1, CE4, INV4

[[blocked_by:./T005-migrate-tests-to-global-config-inputs.md]]

## Preserva

- INV2: Los tests CLI ejercitan el engine genérico real.
  - Verificar: los comandos bajo test usan manifests en `<BACKSCROLL_CONFIG_DIR>/backscroll/inputs/*.inputs.toml` cuando necesitan inputs.
- INV4: La config global real del desarrollador no afecta la suite.
  - Verificar: `cargo test --test cli test_cli_status` no indexa ni consulta `~/.config/backscroll/inputs`.

## Contexto

Después de O03, Backscroll carga inputs canónicos desde el config dir global del usuario. Se detectó que `just test` puede colgarse en máquinas con manifests activos reales porque varios tests CLI invocan `Command::cargo_bin("backscroll")` sin aislar `BACKSCROLL_CONFIG_DIR`. En ese caso comandos como `status` ejecutan autosync contra sesiones reales (`~/.claude`/`~/.pi`) en lugar de fixtures temporales.

La causa raíz no es el motor de inputs sino la contaminación del entorno de tests. La solución permanente es que cada comando CLI de `tests/cli.rs` tenga un config dir explícito: vacío para tests que no necesitan inputs, o poblado con manifests fixture para los que sí los necesitan.

## Alcance

**In**:
1. Crear un helper común en `tests/cli.rs` para construir comandos `backscroll` con `BACKSCROLL_CONFIG_DIR` apuntando a un tempdir controlado.
2. Reemplazar las invocaciones directas restantes de `Command::cargo_bin("backscroll")` que heredan el entorno real.
3. Para tests sin inputs, pasar un config dir temporal vacío.
4. Para tests con inputs, escribir manifests bajo `<temp>/backscroll/inputs/*.inputs.toml` y pasar ese tempdir al comando.
5. Agregar o ajustar regresiones para comandos con autosync/status de modo que fallen si leen config real del usuario.
6. Documentar en el propio test/helper la razón del aislamiento.

**Out**:
- Cambiar semántica runtime de `InputConfig::load()`.
- Quitar autosync de comandos de usuario.
- Usar `std::env::set_var` global en tests CLI.
- Depender de que el desarrollador no tenga manifests globales instalados.

## Estado inicial esperado

- `tests/cli.rs` contiene múltiples usos de `Command::cargo_bin("backscroll")`.
- Algunos tests ya pasan `.env("BACKSCROLL_CONFIG_DIR", ...)`, pero otros no.
- `cargo test --all-features` puede colgarse localmente si existen manifests activos en `~/.config/backscroll/inputs`.
- `BACKSCROLL_CONFIG_DIR=$(mktemp -d) cargo test --all-features` pasa.

## Criterios de Aceptación

- No queda ningún `Command::cargo_bin("backscroll")` en `tests/cli.rs` que ejecute un comando susceptible de cargar inputs sin `BACKSCROLL_CONFIG_DIR` explícito o helper equivalente.
- `cargo test --test cli test_cli_status` pasa aunque existan manifests activos en la config real del usuario.
- `cargo test --test cli inputs` y `cargo test --test cli sync` siguen pasando con manifests temporales controlados.
- Existe una regresión o comentario de helper que explica que la suite no debe leer `~/.config/backscroll`.
- `BACKSCROLL_CONFIG_DIR=$(mktemp -d) cargo test --all-features` pasa.

## Fuente de verdad

- `tests/cli.rs`
- `src/main.rs`
- `src/input_config.rs`
- `docs/roadmap/O03-global-user-scoped-inputs/T005-migrate-tests-to-global-config-inputs.md`
