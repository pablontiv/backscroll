# S040: Patrones de ruido faltantes

**Feature**: [F01 Completitud de Filtros de Ruido](../README.md)
**Capacidad**: 3 patrones de ruido del research que faltan en `filter_noise()` se agregan y testean.
**Cubre**: P1 del Epic (todos los patrones filtrados)

## Antes / Despues

**Antes**: `filter_noise()` cubre 7 patrones (system-reminder, task-notification, caveat, local-command-caveat, command, Base directory, Request interrupted) pero omite 3 del research: `<local-command-stdout>`, `<command-name/message/args>`, y `Caveat:` standalone prefix.

**Despues**: Los 3 patrones faltantes implementados. Cada uno con test unitario dedicado. Busquedas ya no retornan tags XML de hooks ni prefijos de caveat sin wrapper.

## Criterios de Aceptacion (semanticos)

- [ ] `cargo test test_noise_filter_local_command_stdout` pasa
- [ ] `cargo test test_noise_filter_command_name_tags` pasa
- [ ] `cargo test test_noise_filter_caveat_prefix` pasa
- [ ] `cargo test test_noise_filter_mixed_new_patterns` pasa

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: tests existentes de parse siguen pasando
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Tests existentes no regresan
  - Verificar: `just test`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T046](T046-agregar-regex-faltantes.md) | Agregar 3 patrones regex faltantes a filter_noise() |
| [T047](T047-tests-patrones-nuevos.md) | Tests unitarios para los 3 patrones nuevos |

## Fuente de verdad

- `src/core/sync.rs` — funcion `filter_noise()`
- `docs/research/backscroll-session-search-cli.md` — seccion "Patrones de ruido verificados"
