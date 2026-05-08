---
estado: Completed
tipo: task
---
# T003: Extraer parser existente en adapter nativo `claude`

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Proteger comportamiento existente al migrar a capa declarativa.

## Preserva

- INV1: Mantener la limpieza de ruido y clasificación de mensajes (`text`/`code`/`tool`).
  - Verificar: pruebas de filtrado de ruido y parseo siguen iguales.

## Contexto

Hoy `parse_sessions` contiene parsing exclusivo de Claude; debe convertirse en adapter nativo (`source = "claude"`) para reutilizarse desde el motor de inputs.

## Alcance

**In**:
1. Extraer lógica de parseo a módulo `core/session_inputs/` o equivalente.
2. Encapsularlo como implementación interna de la nueva interfaz.
3. Asegurar que preserve `source`, `source_path`, `uuid` y `project`.

**Out**:
- Rediseñar por completo el pipeline de sync del motor.

## Estado inicial esperado

- `parse_sessions` funciona sólo para `SessionRecord` de Claude.

## Criterios de Aceptación

- Parser `claude` produce mismo `ParsedFile` lógico que el actual.
- Se reutiliza en ambos modos: sync explícito y autosync.
- Se documentan reglas soportadas y límites.

## Fuente de verdad

- `src/core/sync.rs`
- `src/core/models.rs`
