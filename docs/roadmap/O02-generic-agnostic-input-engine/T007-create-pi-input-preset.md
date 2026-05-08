---
estado: Specified
tipo: task
---
# T007: Create pi.inputs.toml reproducing Pi behavior

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE1, CE4

[[blocked_by:./T005-implement-declarative-filters-transforms.md]]

## Preserva

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: preset Pi emite `source = "session"`.
- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: Pi fixture termina en esas estructuras.

## Contexto

Pi no usa semánticas Claude como subagents, pero puede tener blocks `think`. Esa diferencia debe vivir en `pi.inputs.toml`, no en Rust.

## Alcance

**In**:
1. Crear `pi.inputs.toml` o fixture equivalente en la ubicación definida por T001.
2. Declarar discovery JSONL apropiado para Pi.
3. Declarar mappings `role`, `content`, `timestamp`, `uuid`/`session_id` según fixtures reales.
4. Declarar eliminación de blocks `think` si el formato lo requiere.
5. Cubrir fallback de contenido si fue parte del comportamiento aprobado.

**Out**:
- Parser Pi dedicado como camino principal.
- Semánticas Claude dentro del preset Pi.

## Estado inicial esperado

- `parse_pi_file()` y `parse_pi_value()` implementan parsing hardcodeado.
- `test_parse_session_inputs_pi` cubre un fixture mínimo.

## Criterios de Aceptación

- Fixture Pi se indexa desde TOML usando generic engine.
- Blocks `think` se excluyen mediante configuración.
- `source = "session"` y roles/timestamps esperados se preservan.
- No hay `PiInputParser` en el camino principal.

## Fuente de verdad

- `src/core/sync.rs`
- `tests/fixtures/`
- `docs/configuration.md`
