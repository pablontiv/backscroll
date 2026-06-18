---
estado: Completed
tipo: task
---
# T019: Skill `backscroll` — sincronizar receta con el binario (subcomando `events query`)

**Contribuye a**: Los agentes que invocan `/skill:backscroll` pueden drillear en una sesión específica sin caer a parseo manual de `.jsonl` con Python.

## Preserva

- INV1: El resto de las secciones de la receta sigue funcionando como antes (Preflight, Sync, Source/role behavior, "No results" troubleshooting).
  - Verificar: leer el archivo editado completo y confirmar que solo cambian las secciones 3.1, 4 (línea 85), 5 y se agregan las secciones nuevas; no se borran ni alteran las otras.
- INV2: Todo comando documentado en la receta es ejecutable contra el binario actual sin "unknown flag".
  - Verificar: ejecutar cada comando de los bloques `bash` de la receta editada y confirmar que ninguno retorna `unknown flag`.

## Contexto

`/home/shared/harness/backscroll/.claude/skills/backscroll/SKILL.md` documenta `backscroll search --source-path '*PATTERN*'` (sección 3.1 líneas 60-61 y comentario en sección 4 línea 85) como mecanismo para scopear la búsqueda a un archivo o UUID específico. Ese flag no existe en el binario actual:

```
$ backscroll search "git push" --source-path "*33b6199f*"
Error: unknown flag: --source-path
```

El binario sí tiene la capacidad — bajo otra subcomanda. `backscroll events query --help` lista `--source-path string  Filter by source path (exact or * glob pattern)`. Existen además `backscroll read <FILE>` (volcado humano-legible) y `backscroll sessions` (inspección) que la receta tampoco menciona.

Resultado del gap: agentes que siguen la receta verbatim fallan con `unknown flag` y caen a `python3` parseando los `.jsonl` directamente. Se pierde el beneficio del índice y la "receta autosuficiente" deja de serlo en el caso que motivó el skill.

## Alcance

**In**:

1. **Sección 3.1 "UUID/session-id path lookup"** — reemplazar el comando central:
   - De: `backscroll search "$UUID" --source sessions --source-path "*$UUID*" --all-projects --robot --max-tokens 4000`
   - A: `backscroll events query "$UUID" --source-path "*$UUID*" --all-projects --robot`
   - Y reformular el párrafo posterior para apuntar el fallback de "remembered terms" a `events query` (con `--event-type message` para limitar a texto si se quiere).

2. **Sección 3 "Invocation-to-command mapping" línea 50** — corregir la fila UUID:
   - De: `if QUERY matches UUID pattern, use search --source-path '*UUID*'`
   - A: `if QUERY matches UUID pattern, use events query '*UUID*' --source-path '*UUID*'`

3. **Sección 4 "Non-UUID search routing" línea 85** — reemplazar el comentario y comando:
   - De: `# Narrow retrieval to an explicit indexed file/path fragment` seguido de `backscroll search ... --source-path ...`
   - A: `# Narrow retrieval to an explicit indexed file/path fragment (chronological, not ranked):` seguido de `backscroll events query "QUERY" --source-path "PATH_OR_*PATTERN*" --all-projects --robot`
   - Agregar una línea aclaratoria: `events query` emite eventos en orden determinístico; `search` rankea por relevancia (BM25 + vector + RRF). No se sustituyen — drill por path → `events query`; búsqueda semántica → `search`.

4. **Nueva sección "Drill into one session"** (después de la sección 4) con ejemplos canónicos:
   ```bash
   # Volcado humano-legible:
   backscroll read /home/pones/.claude/projects/<slug>/<UUID>.jsonl

   # Tool_calls cronológicos:
   backscroll events query <UUID> --event-type tool_call --robot

   # Filtro por rol:
   backscroll events query <UUID> --role user --robot

   # Ventana temporal:
   backscroll events query <UUID> --after 2026-05-19 --before 2026-05-20 --robot

   # JSONL para post-procesar con jq:
   backscroll events query <UUID> --event-type tool_call --json | jq '...'
   ```

5. **Sección 5 "Command validity"** — actualizar:
   - Agregar `events query` como subcomando que acepta `--robot` y `--json`.
   - Agregar nota explícita: `--source-path` aplica **solo** a `events query`, NO a `search`.

6. **Nueva sección "Cuándo el fallback a Python sigue justificado"** (al final) reconociendo la limitación real del indexador:
   > El indexador almacena los bloques `tool_use` como texto plano para BM25 + embeddings, no como campos relacionales. Si necesitás extraer **inputs estructurados** (e.g. el `command` de cada `Bash` como string aislado, separado de `description`), `events query` te da los eventos en orden pero el `input` queda como blob serializado. Para ese caso específico, parsear el `.jsonl` con Python sigue siendo correcto.
   >
   > Todos los demás casos (listar tool_uses cronológicamente, scopear por archivo o UUID, filtrar por rol, exportar texto) están cubiertos por `events query` + `read` y NO requieren Python.

7. **Sección 6 "Source and role behavior"** — agregar nota sobre `--project`: si el path target no está registrado en `backscroll projects list`, usar `--all-projects` y filtrar la salida por patrón de `filepath` (e.g. grep `*-home-shared-roadmapctl-*`).

8. **Sincronización user-scope** — después de editar la fuente de verdad, re-sincronizar:
   - Si existe `scripts/install-user.sh` (o equivalente) en el repo de backscroll, ejecutarlo.
   - Si no, `cp -f /home/shared/harness/backscroll/.claude/skills/backscroll/SKILL.md /home/pones/.claude/skills/backscroll/SKILL.md`.
   - Verificar: `diff -q /home/shared/harness/backscroll/.claude/skills/backscroll/SKILL.md /home/pones/.claude/skills/backscroll/SKILL.md` no produce output.

**Out**:
- Cualquier cambio al binario `backscroll` (vive en `cmd/backscroll/`, fuera de esta task).
- Agregar un nuevo flag/comando para `tool_use` estructurado (sería feature, no fix de docs).
- Cambios a otras secciones del SKILL.md no listadas arriba.

## Estado inicial esperado

- `grep -n "search.*--source-path" /home/shared/harness/backscroll/.claude/skills/backscroll/SKILL.md` encuentra al menos 2 matches (sección 3.1 y línea 85).
- `backscroll events query --help` lista `--source-path` como flag válido.
- `backscroll search --help` no lista `--source-path`.

## Criterios de Aceptación

- AC1: `grep -nE "backscroll search.*--source-path" /home/shared/harness/backscroll/.claude/skills/backscroll/SKILL.md` retorna exit 1 (sin matches — ningún `search` invoca `--source-path` después del cambio).
- AC2: `grep -nE "backscroll events query.*--source-path" /home/shared/harness/backscroll/.claude/skills/backscroll/SKILL.md` retorna al menos 2 matches (sección 3.1 + sección 4).
- AC3: El SKILL.md contiene una sección con el título "Drill into one session" (o equivalente español) con los 5 ejemplos canónicos del Alcance punto 4.
- AC4: El SKILL.md contiene una sección con el título "Cuándo el fallback a Python sigue justificado" (o equivalente) que reconoce la limitación de `tool_use` no estructurado.
- AC5: La sección de "Command validity" lista `events query` como subcomando que acepta `--robot`, `--json`, y aclara que `--source-path` no aplica a `search`.
- AC6: Smoke test: `backscroll events query 33b6199f --source-path "*33b6199f*" --all-projects --robot --limit 5` ejecuta y retorna eventos (no "unknown flag"). Si la sesión `33b6199f` ya no está indexada localmente, usar un UUID conocido de `backscroll list --recent 1 --all-projects --robot`.
- AC7: `diff -q /home/shared/harness/backscroll/.claude/skills/backscroll/SKILL.md /home/pones/.claude/skills/backscroll/SKILL.md` no produce output (las dos copias quedan sincronizadas).

## Fuente de verdad

- `/home/shared/harness/backscroll/.claude/skills/backscroll/SKILL.md` (fuente canónica del skill)
- `/home/pones/.claude/skills/backscroll/SKILL.md` (espejo user-scope, se sincroniza desde la fuente)
