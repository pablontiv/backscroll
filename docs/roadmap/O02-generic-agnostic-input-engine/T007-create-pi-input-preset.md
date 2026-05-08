---
estado: Completed
tipo: task
---
# T007: Create pi.inputs.toml reproducing Pi behavior

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE1, CE4

[[blocked_by:./T005-implement-declarative-filters-transforms.md]]

## Preserva

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: preset Pi emite `source = "session"`, no `source = "pi"`.
- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: Pi fixture termina en esas estructuras.

## Contexto

Pi no usa semánticas Claude como subagents, pero puede tener blocks `think`. Esa diferencia debe vivir en `pi.inputs.toml`, no en Rust.

En O02, Pi se incorpora igual que Claude: manifest TOML activo, decode genérico, selectors JSONPath, predicados y normalización declarativa.

## Alcance

**In**:
1. Crear `pi.inputs.toml` o fixture equivalente en la ubicación definida por T001/T002.
2. Declarar `version = 1` y `[[inputs]]` con `id = "pi"`, `source = "session"` y `active = true`.
3. Declarar discovery JSONL apropiado para Pi con `roots`, `include` y `exclude`.
4. Declarar mappings `role`, `content`, `timestamp`, `uuid`/`session_id` según fixtures reales.
5. Declarar `role_aliases` si Pi usa roles como `human` que deben normalizarse a `user`.
6. Declarar eliminación de blocks `think` con `content.exclude_when` si el formato lo requiere.
7. Declarar `default_content_type = "text"` y normalización de texto según el contrato.

**Out**:
- Parser Pi dedicado como camino principal.
- Semánticas Claude dentro del preset Pi.
- Fallback de contenido no descrito por el contrato final.

## Estado inicial esperado

- `parse_pi_file()` y `parse_pi_value()` implementan parsing hardcodeado.
- `test_parse_session_inputs_pi` cubre un fixture mínimo.

## Criterios de Aceptación

- Fixture Pi se indexa desde TOML usando generic engine.
- Blocks `think` se excluyen mediante configuración.
- `source = "session"` y roles/timestamps esperados se preservan.
- No hay `PiInputParser` ni lógica Pi hardcodeada en el camino principal.
- Si el manifest Pi está ausente o inválido, no se activa un parser Pi implícito.

## Fuente de verdad

- `docs/input-contract.md`
- `src/core/sync.rs`
- `tests/fixtures/`
- `docs/configuration.md`
