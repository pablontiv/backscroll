---
estado: Completed
tipo: test
ejecutable_en: 1 sesion
---
# T047: Tests unitarios para los 3 patrones nuevos

**Story**: [S040 Patrones de ruido faltantes](README.md)
**Contribuye a**: P1 — cada patron nuevo tiene test dedicado

[[blocks:T046-agregar-regex-faltantes]]

## Preserva

- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Tests existentes no regresan
  - Verificar: `just test`

## Contexto

Cada patron de ruido nuevo (T046) necesita un test unitario que demuestre que se filtra correctamente y que contenido no-ruidoso se preserva.

## Especificacion Tecnica

Agregar al modulo `tests` en `src/core/sync.rs`:

1. `test_noise_filter_local_command_stdout` — verifica que `<local-command-stdout>contenido</local-command-stdout>` se elimina y que tag vacio tambien
2. `test_noise_filter_command_name_tags` — verifica que `<command-name>foo</command-name>`, `<command-message>bar</command-message>`, `<command-args>baz</command-args>` se eliminan
3. `test_noise_filter_caveat_prefix` — verifica que linea `Caveat: The messages below...` se elimina pero "the caveat is..." no se toca
4. `test_noise_filter_mixed_new_patterns` — mensaje con ruido nuevo + contenido util preserva el contenido util

## Alcance

**In**:
1. 4 tests unitarios nuevos
2. Cada test verifica filtering Y preservation

**Out**: No agregar tests para patrones existentes (ya cubiertos por T025).

## Estado inicial esperado

- T046 completado (5 patterns nuevos en filter_noise)

## Criterios de Aceptacion

- `cargo test test_noise_filter_local_command_stdout` pasa
- `cargo test test_noise_filter_command_name_tags` pasa
- `cargo test test_noise_filter_caveat_prefix` pasa
- `cargo test test_noise_filter_mixed_new_patterns` pasa
- `just check` pasa

## Fuente de verdad

- Tests en `src/core/sync.rs` modulo tests
