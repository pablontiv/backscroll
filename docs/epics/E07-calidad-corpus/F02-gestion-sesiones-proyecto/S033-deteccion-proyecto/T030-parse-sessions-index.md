---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T030: Parser sessions-index.json para lookup projectPath

**Story**: [S033 Deteccion automatica de proyecto](README.md)
**Contribuye a**: SELECT count(*) FROM search_items WHERE project IS NULL = 0

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Algunos directorios de Claude Code contienen `sessions-index.json` con metadata de sesiones, incluyendo `projectPath`. Se puede usar para asociar sesiones a proyectos. Disponible en ~36% de directorios reales (5/14).

## Alcance

**In**:
1. Funcion `load_session_index(dir: &Path) -> HashMap<String, String>` (session_id → project_path)
2. Parsear sessions-index.json si existe en el directorio raiz de sesiones
3. Extraer projectPath de cada entry
4. Devolver hashmap para lookup rapido durante sync

**Out**: No implementar fallback (T031). No integrar en sync loop (se hara al conectar con T031).

## Estado inicial esperado

- sync.rs no lee sessions-index.json
- project siempre es None

## Criterios de Aceptacion

- `cargo test test_parse_sessions_index` — parsea fixture de sessions-index.json
- Funcion retorna HashMap correcto
- Funcion retorna HashMap vacio si archivo no existe (no falla)
- `just check` pasa

## Fuente de verdad

- `src/core/sync.rs` (o modulo dedicado)
- `~/.claude/projects/*/sessions-index.json` — formato de referencia
