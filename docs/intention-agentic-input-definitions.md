# Intención de Refactor: Backscroll agnóstico de CLI agentic

## 1) Propósito
Transformar Backscroll de un parser específico de Claude a un sistema agnóstico de CLI agentic, moviendo la interpretación de sesiones desde código Rust hardcodeado hacia **definiciones externas en TOML** (`backscroll.inputs.toml` / `backscroll.inputs.d/*.toml`), para poder agregar soporte de nuevos agentes **sin recompilar**.

## 2) Alcance (MVP)

- Fase 1: soporte externo de inputs para sesiones con `source = "session"`.
- Incluir desde el inicio:
  - `claude` (migración desde parser actual)
  - `pi` (nuevo mapper externo)
- Mantener compatibilidad completa con comportamiento actual mientras se agrega la capa declarativa.
- **Regla absoluta:** no habrá soporte de `command` adapters ejecutables ni scripts externos de integración en esta fase.

## 3) Principios del refactor

1. **Sin recompilar para agregar inputs:** agregar/modificar TOML es suficiente.
2. **Agnóstico por diseño:** Backscroll no debe depender de la semántica concreta de cada CLI.
3. **Compatibilidad hacia atrás primero:** soportar flujo actual de usuarios/config existente.
4. **Contrato interno estable:** seguir normalizando a estructuras actuales (`ParsedFile`, `ParsedMessage`).
5. **Incremental y riesgo acotado:** no tocar almacenamiento/semántica de búsqueda salvo lo necesario para resolver paths/inputs.

## 4) Invariantes a preservar

### A. Invariantes funcionales de usuario/CLI
1. Los comandos (`search`, `read`, `resume`, `list`, `topics`, `insights`, `export`, `status`, `sync`) siguen disponibles y con comportamiento estable.
2. El autosync previo a comandos se conserva.
3. Precedencia de paths: `--path` mantiene prioridad máxima.
4. `source = "session"` permanece como valor estable para sesiones.
5. No degradar resultados existentes para flujos actuales de sesión cuando no haya cambios deliberados.

### B. Invariantes de configuración
6. `database_path` por defecto sigue en `~/.backscroll.db`.
7. Soporte legado de configuración se conserva:
   - `session_dir` (alias)
   - `session_dirs` (string o arreglo)
8. Orden base actual de config se conserva (`backscroll.toml` → `~/.config/backscroll/config.toml` → `BACKSCROLL_*`), extendiéndolo con inputs de forma ordenada.
9. Fallback actual de descubrimiento de sesiones se mantiene, con precedencia explícita para nuevos manifiestos.

### C. Invariantes de ingestión y datos
10. `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
11. Deduplicación incremental por hash/path sigue vigente.
12. `uuid` y `source_path` siguen siendo la identidad operativa en esta fase.
13. Parseo tolerante: fallos puntuales de registros no deben romper toda la sync.
14. Semántica de mensajes relevantes de sesión se conserva inicialmente.

### D. Invariantes de calidad y pruebas
15. Tests existentes continúan pasando.
16. Se agregan pruebas de precedencia (`--path` > config > inputs > fallback) y compatibilidad.
17. Docs se actualizan y alinean (`README`, `docs/configuration.md`, `docs/sync.md`, `backscroll.toml.example`).

### E. Invariantes de deuda conocida (no resueltas en esta fase)
18. El estado actual de filtros por `--source` para fuentes externas (`ke`, `decision`, `memory`, etc.) permanece como deuda técnica y no se resuelve aquí.
19. `source_metadata` y extensiones de metadatos quedan fuera de alcance de esta fase.

## 5) No-goals (esta fase)

- No introducir adapters/executables `command`.
- No cambiar esquema SQLite.
- No reescribir por completo `parse_sessions` antes de definir la capa de inputs.
- No rediseñar el CLI ni la UX.

## 6) Criterios de éxito del MVP

- Backscroll indexa nuevos inputs definidos por TOML sin recompilar (inicialmente Claude + Pi).
- `claude.toml` y `pi.toml` quedan cubiertos como casos de configuración.
- `session_dir` / `session_dirs` heredado sigue funcionando.
- `--path` conserva precedencia máxima.
- Pruebas de regresión y documentación actualizada pasan y quedan alineadas.