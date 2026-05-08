---
estado: Completed
tipo: task
---
# T005: Implement declarative filters and content transforms

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE3, CE4

[[blocked_by:./T004-implement-jsonl-jsonpath-engine.md]]

## Preserva

- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: filters/transforms operan antes de emitir esos tipos.
- INV3: No se introducen plugins/scripts ejecutables para el MVP.
  - Verificar: filtros son operadores declarativos cerrados definidos por el contrato.

## Contexto

Hoy filtros Claude/Pi están hardcodeados: `isMeta`, `type`, tags de ruido, tool blocks, `Request interrupted`, y Pi `think` si se agrega. En O02 deben expresarse con las secciones del contrato final:

- `[inputs.record].include_when` / `exclude_when`
- `[inputs.content].include_when` / `exclude_when`
- `[inputs.text].remove`, `join`, `trim`, `drop_empty`

No se agrega lenguaje de expresión arbitrario ni JMESPath en el MVP.

## Alcance

**In**:
1. Implementar predicados declarativos para `record.include_when`, `record.exclude_when`, `content.include_when` y `content.exclude_when`.
2. Soportar exactamente los operadores MVP del contrato: `eq`, `ne`, `in`, `exists` y `missing`.
3. Evaluar predicados con selectors JSONPath sobre el record o block correspondiente.
4. Implementar normalización de `[inputs.text]`: `join`, `trim`, `drop_empty` y reglas `remove`.
5. Soportar `remove.kind = "regex"`, `"prefix"` y `"suffix"`.
6. Emitir diagnósticos de records/blocks descartados para el dry-run futuro de T010.

**Out**:
- Operadores fuera del contrato MVP, como `contains`, `not_in` o expresiones arbitrarias.
- JMESPath.
- Semánticas hardcodeadas de Claude/Pi en el core.

## Estado inicial esperado

- `filter_noise()` tiene regexes estáticos.
- `parse_claude_message_lines()` filtra records y blocks en Rust.
- `parse_pi_value()` contiene lógica de blocks propia.

## Criterios de Aceptación

- Claude noise actual puede expresarse con `text.remove` y predicados TOML.
- Pi puede excluir blocks `think` desde `content.exclude_when`.
- Core no contiene strings específicos como `system-reminder`, `task-notification`, `subagents` o `think` salvo en tests/fixtures/presets.
- Tests unitarios cubren cada operador declarativo MVP y cada tipo de `text.remove`.
- Manifests con operadores desconocidos fallan con mensaje claro.

## Fuente de verdad

- `docs/input-contract.md`
- `src/core/sync.rs`
- `docs/sync.md`
- `tests/fixtures/`
