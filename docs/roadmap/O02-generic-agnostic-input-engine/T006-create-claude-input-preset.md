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
  - Verificar: preset Claude emite `source = "session"`, no `source = "claude"`.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: preset usa JSONPath y operadores declarativos MVP.

## Contexto

Claude debe dejar de ser parser especial en Rust. Su comportamiento actual debe materializarse como un manifest `claude.inputs.toml` compatible con `docs/input-contract.md`.

O01 podía descubrir Claude de forma implícita. En O02, Claude solo entra al flujo canónico si existe un manifest activo válido.

## Alcance

**In**:
1. Crear `claude.inputs.toml` o fixture equivalente en la ubicación definida por T001/T002.
2. Declarar `version = 1` y `[[inputs]]` con `id = "claude"`, `source = "session"` y `active = true`.
3. Declarar discovery con `roots`, `include = ["**/*.jsonl"]` y `exclude = ["**/subagents/**"]`.
4. Declarar `decode.format = "jsonl"`.
5. Declarar filtros de record para tipos `user`/`assistant` y metadata Claude usando `include_when`/`exclude_when`.
6. Declarar mapping `message.role`, `uuid`, `timestamp` y `sessionId` usando `[inputs.map]`.
7. Declarar contenido desde `message.content`, incluyendo blocks de texto con `content.include_when` en lugar de lógica Rust específica.
8. Declarar regexes de ruido actualmente en `filter_noise()` mediante `text.remove`.

**Out**:
- Mantener `ClaudeInputParser` o `parse_sessions` Claude-only como camino principal.
- Fallback Claude implícito cuando no hay manifest activo.
- Resolver `sessions-index.json` si no cabe en el contrato MVP; si queda pendiente, documentar gap explícito.

## Estado inicial esperado

- `parse_session_file_claude()` reproduce el comportamiento esperado.
- Tests existentes de Claude pasan con parser Rust.

## Criterios de Aceptación

- Fixture Claude se indexa usando solo manifest + generic engine.
- Resultado normalizado conserva roles, texto, timestamp, uuid y `source = "session"` esperados.
- Subagents se excluyen por TOML, no por lógica del core.
- Strings de ruido Claude viven en preset TOML.
- Si el manifest Claude está ausente o inválido, el flujo canónico falla claramente en vez de asumir Claude.

## Fuente de verdad

- `docs/input-contract.md`
- `src/core/sync.rs`
- `src/core/models.rs`
- `tests/cli.rs`
- `docs/sync.md`
