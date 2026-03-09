---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T058: Resume search logic

**Story**: [S045 Resume subcommand](README.md)
**Contribuye a**: P3 (resume produce session ID), P4 (pipe-friendly)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Implementar la logica completa de resume: buscar, tomar top-1, formatear output.

## Especificacion Tecnica

En `src/main.rs` Resume dispatch:

1. Llamar `engine.search(query, project)` (reusar search existente)
2. Tomar primer resultado (si existe)
3. Llamar `engine.get_session_id(source_path)` (T060) para obtener UUID
4. Text mode: imprimir session path, session ID, y snippet del primer user message
5. Robot mode: imprimir `session_id\tsource_path` (single line, pipe-ready)
6. Sin resultados: imprimir error informativo y salir con code 1

## Alcance

**In**: Dispatch completo de resume en main.rs
**Out**: No implementar get_session_id (T060), solo usar el trait method

## Criterios de Aceptacion

- Text mode muestra 3 lineas informativas
- Robot mode es single-line tab-separated
- Exit code 1 si no hay resultados

## Fuente de verdad

- `src/main.rs` — Commands::Resume dispatch
