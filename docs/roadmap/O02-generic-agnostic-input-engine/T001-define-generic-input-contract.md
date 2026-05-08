---
estado: Specified
tipo: task
---
# T001: Define generic inputs manifest contract

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE2, CE3, CE4

## Preserva

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: ejemplos del contrato usan `source = "session"` para Claude/Pi.
- INV3: No se introducen plugins/scripts ejecutables para el MVP.
  - Verificar: contrato no define `command`, `exec`, `script` ni equivalente.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: contrato usa JSONPath/selectores declarativos mínimos, no JMESPath.

## Contexto

El estado deseado es que Backscroll sea genérico/agnóstico: Claude, Pi y futuros CLIs se describen con `*.inputs.toml`. El core interpreta un pipeline común `discover -> decode -> filter -> map -> emit`.

## Alcance

**In**:
1. Definir el schema TOML para `[[inputs]]`.
2. Separar claramente `source` semántico de `decode.format`/parser técnico.
3. Definir secciones mínimas: `discover`, `decode`, `record`, `map`, `content`, `text`.
4. Documentar ejemplos completos para Claude y Pi.
5. Definir política de campos desconocidos y validación.

**Out**:
- Implementación del loader o engine.
- JMESPath.
- Plugins ejecutables.

## Estado inicial esperado

- Existe implementación previa de `SessionInput` en `src/config.rs` con campos insuficientes.
- Docs actuales usan `backscroll.inputs.toml` y ejemplos con `source` ambiguo.

## Criterios de Aceptación

- Hay documento o módulo de contrato que especifica `*.inputs.toml`.
- El contrato permite expresar exclusión Claude `**/subagents/**` sin semántica hardcodeada.
- El contrato permite expresar exclusión Pi de blocks `think` sin semántica hardcodeada.
- El contrato define que conversaciones usan `source = "session"` aunque el input sea Claude/Pi.
- El contrato deja JMESPath explícitamente fuera del MVP y remite a T013.

## Fuente de verdad

- `docs/intention-agentic-input-definitions.md`
- `docs/configuration.md`
- `docs/sync.md`
- `src/config.rs`
- `src/core/mod.rs`
