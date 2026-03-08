---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T024: Implementar 8+ filtros de ruido

**Story**: [S030 Filtrado de patrones de ruido](README.md)
**Contribuye a**: Search por system-reminder y task-notification retorna 0

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Mensajes user/assistant pueden contener ruido inyectado por el sistema: tags XML como `<system-reminder>`, `<task-notification>`, `<caveat>`, bloques de `tool_use`/`tool_result`, etc. Este contenido contamina los resultados de busqueda. Se necesitan filtros que limpien o descarten este contenido antes de indexar.

## Especificacion Tecnica

Patrones a filtrar (contenido completo del mensaje o seccion):
1. `<system-reminder>...</system-reminder>` — strip tags y contenido
2. `<task-notification>...</task-notification>` — strip
3. `<caveat>...</caveat>` — strip
4. Command XML blocks — strip
5. `<local-command-caveat>` / stdout blocks — strip
6. "Request interrupted" messages — skip mensaje completo
7. "Base directory:" prefix lines — strip
8. `tool_use` / `tool_result` content blocks (en MessageContent array) — skip

Implementar como funcion `filter_noise(text: &str) -> Option<String>` que retorna None si el mensaje es 100% ruido, o Some(cleaned) si queda contenido util.

## Alcance

**In**:
1. Crear funcion `filter_noise` en `src/core/sync.rs` (o modulo dedicado)
2. Implementar los 8 filtros listados
3. Integrar en parse_sessions: aplicar filter_noise despues de extraer texto
4. Skip mensajes que retornan None

**Out**: No agregar filtros heuristicos complejos (NLP, etc.).

## Estado inicial esperado

- Pre-filtrado por record type funciona (S029)
- Mensajes user/assistant se parsean correctamente

## Criterios de Aceptacion

- `cargo test test_noise_filter_system_reminder` pasa
- `cargo test test_noise_filter_tool_blocks` pasa
- filter_noise retorna None para mensajes 100% ruido
- filter_noise retorna Some(cleaned) para mensajes mixtos
- `just check` pasa

## Fuente de verdad

- `src/core/sync.rs` (o nuevo modulo de filtrado)
- `docs/research/backscroll-session-search-cli.md` — seccion "Noise Patterns"
