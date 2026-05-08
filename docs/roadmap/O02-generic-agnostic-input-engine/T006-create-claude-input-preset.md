---
estado: Specified
tipo: task
---
# T006: Create claude.inputs.toml reproducing current Claude behavior

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE1, CE4

[[blocked_by:./T005-implement-declarative-filters-transforms.md]]

## Preserva

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: preset Claude emite `source = "session"`.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: preset usa JSONPath y operadores declarativos.

## Contexto

Claude debe dejar de ser parser especial en Rust. Su comportamiento actual debe materializarse como preset TOML.

## Alcance

**In**:
1. Crear `claude.inputs.toml` o fixture equivalente en la ubicación definida por T001.
2. Declarar discovery de JSONL con exclude de subagents mediante glob.
3. Declarar filtros de record `isMeta` y `type`.
4. Declarar mapping `message.role`, `message.content`, `uuid`, `timestamp`.
5. Declarar eliminación de blocks `tool_use`/`tool_result`.
6. Declarar regexes/text drops actualmente en `filter_noise()`.

**Out**:
- Mantener `ClaudeInputParser` como camino principal.
- Resolver `sessions-index.json` si no cabe en el contrato MVP; si queda pendiente, documentar gap explícito.

## Estado inicial esperado

- `parse_session_file_claude()` reproduce el comportamiento esperado.
- Tests existentes de Claude pasan con parser Rust.

## Criterios de Aceptación

- Fixture Claude se indexa usando solo manifest + generic engine.
- Resultado normalizado conserva roles, texto, timestamp y uuid esperados.
- Subagents se excluyen por TOML, no por lógica del core.
- Strings de ruido Claude viven en preset TOML.

## Fuente de verdad

- `src/core/sync.rs`
- `src/core/models.rs`
- `tests/cli.rs`
- `docs/sync.md`
