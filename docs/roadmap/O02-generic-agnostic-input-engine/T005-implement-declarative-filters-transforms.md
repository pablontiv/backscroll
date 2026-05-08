---
estado: Specified
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
  - Verificar: filtros son operadores declarativos cerrados.

## Contexto

Hoy filtros Claude/Pi están hardcodeados: `isMeta`, `type`, tags de ruido, tool blocks, `Request interrupted`, y Pi `think` si se agrega. Deben vivir en TOML.

## Alcance

**In**:
1. Implementar `record.drop_if` con operadores mínimos: `equals`, `not_equals`, `in`, `not_in`, `exists`, `contains` si aplica.
2. Implementar `content.drop_block_types` para arrays/blocks.
3. Implementar `text.strip_regex` y `text.drop_if_contains`.
4. Implementar descarte de mensajes vacíos post-transform.
5. Emitir diagnósticos de records dropeados para dry-run futuro.

**Out**:
- Lenguaje de expresiones arbitrario.
- JMESPath.

## Estado inicial esperado

- `filter_noise()` tiene regexes estáticos.
- `parse_claude_message_lines()` filtra records y blocks en Rust.
- `parse_pi_value()` contiene lógica de blocks propia.

## Criterios de Aceptación

- Claude noise actual puede expresarse en TOML.
- Pi puede excluir blocks `think` desde TOML.
- Core no contiene strings específicos como `system-reminder`, `task-notification`, `subagents`, `think` salvo en tests/fixtures/presets.
- Tests unitarios cubren cada operador declarativo.

## Fuente de verdad

- `src/core/sync.rs`
- `docs/sync.md`
- `tests/fixtures/`
