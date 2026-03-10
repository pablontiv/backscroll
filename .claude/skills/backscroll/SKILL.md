---
name: backscroll
description: |
  Buscar y analizar el historial de conversaciones de sesiones anteriores de
  Claude Code usando backscroll (búsqueda full-text con FTS5 y ranking BM25).
  Recupera contexto perdido, filtra por keyword con relevancia, muestra
  distribución de temas, y extrae fragmentos relevantes con snippets rankeados.
  Usar este skill siempre que el usuario mencione sesiones anteriores, pregunte
  "de que hablamos sobre X?", quiera encontrar algo de una sesión pasada, diga
  "lo discutimos antes", "we talked about this before", "find that conversation",
  o necesite recuperar contexto — incluso si no dice "backscroll" e incluso si
  solo dice "no me acuerdo que habíamos decidido" o "I forgot what we discussed."
  Para snapshots de estado estructurado (guardar/restaurar progreso), usar
  context-save en su lugar.
user-invocable: true
disable-model-invocation: false
argument-hint: "[query] | --topics | --recent N | --context"
allowed-tools: Bash
---

# Skill: Backscroll

## Contexto
Este skill busca en las sesiones anteriores de Claude Code usando `backscroll`, un motor de búsqueda full-text con FTS5 y ranking BM25. Los resultados se ordenan por relevancia, no solo por coincidencia textual.

## Proceso

### 1. Gate check

Verificar que backscroll está instalado:
```bash
command -v backscroll >/dev/null 2>&1
```

Si no está disponible, informar al usuario:
> backscroll no está instalado. Instalar con:
> ```
> cargo install --git https://github.com/pablontiv/backscroll.git
> ```

### 2. Aplicar según argumento

Backscroll auto-sincroniza el índice y filtra por proyecto (derivado del CWD) automáticamente. No hay pasos manuales de sync ni detección de proyecto.

| Argumento | Acción |
|-----------|--------|
| (vacío) | Vista general: status del índice + sesiones recientes |
| `[query]` | Búsqueda full-text rankeada por relevancia |
| `--topics` | Análisis de temas frecuentes |
| `--recent N` | Últimas N sesiones con resumen |
| `--context` | Query rootline session-state (requiere rootline + /context-save) |

#### 2a. Búsqueda por keyword (camino principal)

```bash
backscroll search "QUERY" --robot --max-tokens 4000
```

El formato `--robot` produce output compacto tab-separated (path, score, snippet) optimizado para consumo por LLM. Presentar resultados al usuario agrupados por sesión, mostrando:
- Ruta de la sesión y score de relevancia
- Snippets con contexto alrededor de la coincidencia

Si no devuelve resultados, intentar en todos los proyectos:
```bash
backscroll search "QUERY" --all-projects --robot --max-tokens 4000
```

#### 2b. Vista general (sin argumentos)

Mostrar status del índice y sesiones recientes:
```bash
backscroll status
```

Complementar con listado de archivos recientes:
```bash
SESSION_DIR="$HOME/.claude/projects/$(pwd | tr '/' '-')/"
ls -lt "$SESSION_DIR"*.jsonl 2>/dev/null | grep -v agent- | head -10
```

#### 2c. Sesiones recientes (`--recent N`)

Listar archivos y usar `backscroll read` para obtener contenido limpio de cada sesión:
```bash
SESSION_DIR="$HOME/.claude/projects/$(pwd | tr '/' '-')/"
ls -lt "$SESSION_DIR"*.jsonl 2>/dev/null | grep -v agent- | head -N
```

Para cada sesión relevante:
```bash
backscroll read PATH_TO_SESSION
```

Esto produce output limpio `[role] message` con ruido filtrado automáticamente (system-reminders, tool calls, XML tags).

#### 2d. Temas (`--topics`)

Para un análisis de temas, buscar en TODAS las sesiones del usuario con `--all-projects` para obtener una vista completa:

```bash
backscroll search "deploy" --all-projects --robot --max-tokens 8000
```

FTS5 no soporta wildcards, así que ejecutar múltiples búsquedas temáticas amplias (ej: "deploy", "migración", "architecture", "error", "roadmap", "testing", "database") y combinar resultados.

Analizar los snippets devueltos y sintetizar una distribución de temas discutidos. Agrupar por proyecto y tema frecuente, reportando cuántas sesiones lo mencionan. Incluir desglose por proyecto para que el usuario vea la distribución de su actividad.

#### 2e. Contexto (`--context`)

Requiere rootline y `/context-save`. Verificar disponibilidad:
```bash
command -v rootline 2>/dev/null
```

Si rootline está disponible y `.claude/session-state/` existe con un schema `.stem`:

```bash
# Contextos de sesión guardados para este proyecto
rootline query .claude/session-state/ --where "proyecto == '$(basename $(pwd))'" --output table

# Contexto más reciente
rootline query .claude/session-state/ --where "proyecto == '$(basename $(pwd))'" --output table --limit 1
```

Mostrar también el estado actual de artefactos R&D y planificación:

```bash
# Líneas de investigación activas
rootline query lines/ --where 'tipo == "question"' --output table 2>/dev/null

# Investigaciones activas
rootline query . --where 'metodo == "hypothesize"' --output table 2>/dev/null

# Progreso del roadmap (si configurado)
if [ -f .claude/roadmap.local.md ]; then
  ROADMAP_ROOT=$(grep 'roadmap-root:' .claude/roadmap.local.md | awk '{print $2}')
  rootline stats "$ROADMAP_ROOT" --output table 2>/dev/null
  rootline tree "$ROADMAP_ROOT" --output table 2>/dev/null
fi

# Teorías
rootline query theories/ --output table 2>/dev/null
```

Presentar datos de session-state junto con datos live de rootline para una imagen completa de recuperación: qué se discutió (session-state) y dónde está el proyecto ahora (rootline live).

## Modos de uso

| Comando | Descripción |
|---------|-------------|
| `/backscroll` | Status del índice + sesiones recientes |
| `/backscroll [query]` | Búsqueda full-text con ranking BM25 |
| `/backscroll --topics` | Distribución de temas discutidos |
| `/backscroll --recent N` | Últimas N sesiones con resumen limpio |
| `/backscroll --context` | Contexto estructurado via rootline (requiere /context-save) |

## Cuándo usar

- **Recuperar contexto**: "¿Qué discutimos sobre X en sesiones anteriores?"
- **Continuidad**: Antes de retomar una línea, buscar qué se avanzó
- **Conexiones**: Descubrir que un tema se discutió en múltiples sesiones
- **Al inicio de sesión**: Si el estado no es suficiente para recuperar contexto

## Notas

- Solo lee datos — no modifica ningún archivo
- Auto-sync incremental (SHA-256 dedup) en cada query — no requiere sync manual
- Por defecto filtra por proyecto del CWD; `--all-projects` para buscar en todo
- Los resultados se rankean por relevancia (BM25), no solo coincidencia textual
- El ruido (system-reminders, tool calls, XML tags) se filtra automáticamente
- Ignora sesiones de subagentes por defecto
