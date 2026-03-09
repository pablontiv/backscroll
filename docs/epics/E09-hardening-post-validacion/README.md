# E09: Hardening Post-Validacion

**Objetivo**: Cerrar los gaps detectados en la validacion de research docs vs implementacion. Completar filtros de ruido faltantes, optimizar compilacion regex, y eliminar dead code.

## Postcondiciones

| # | Postcondicion | Features | Verificacion |
|---|---------------|----------|-------------|
| P1 | Todos los patrones de ruido del research estan filtrados | F01 | Test unitario por cada patron nuevo pasa |
| P2 | Zero dead code en `errors.rs` | F02 | `#[allow(dead_code)]` eliminado y `just check` pasa |
| P3 | Regex compilados una sola vez (no por invocacion) | F01 | `filter_noise` usa `LazyLock<Regex>` |

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales (heredado de E07)
- INV2: `just check` pasa (clippy nursery+pedantic, -D warnings)
- INV3: Tests existentes no regresan

## Out of Scope

- `--topics` mode (diferido post-v1, documentado en E08)
- Nuevos filtros heuristicos (NLP, etc.)

## Features

| Feature | Descripcion |
|---------|-------------|
| [F01](F01-completitud-filtros/) | Completitud de Filtros de Ruido |
| [F02](F02-limpieza-error-handling/) | Limpieza de Error Handling |

## Dependencias

Requiere E07 y E08 completados.
