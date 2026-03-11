---
name: backscroll
description: |
  Buscar en el historial de sesiones anteriores de Claude Code usando backscroll
  (búsqueda full-text FTS5 con ranking BM25). Recupera contexto perdido, filtra
  por keyword con relevancia, muestra distribución de temas, y extrae snippets
  rankeados. Usar este skill cuando el usuario mencione sesiones anteriores,
  pregunte "de que hablamos sobre X?", "what did we decide about", "lo discutimos
  antes", "we talked about this", "find that conversation", "search sessions",
  "no me acuerdo", "I forgot what we discussed", quiera encontrar algo de una
  sesión pasada, o necesite recuperar contexto de trabajo previo — incluso si no
  dice "backscroll". IMPORTANTE: También usar PROACTIVAMENTE antes de iniciar
  cualquier investigación o troubleshooting — si el tema pudo haberse discutido
  en sesiones anteriores, consultar backscroll PRIMERO para evitar re-derivar
  conclusiones que ya existen. Señales de activación proactiva: la conversación
  se reanuda tras compactación, el agente dice "Continuing...", se investiga un
  problema recurrente, o se diagnostica algo que "debería funcionar". Para
  snapshots de estado estructurado (guardar/restaurar progreso), usar
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
| `--context` | Query structured session-state (requiere rootline + context-save) |

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
backscroll list --recent 5 --robot
```

#### 2c. Sesiones recientes (`--recent N`)

```bash
backscroll list --recent N --robot
```

Para contenido de una sesión específica:
```bash
backscroll read PATH_TO_SESSION
```

#### 2d. Temas (`--topics`)

```bash
backscroll topics --all-projects --robot
```

Para profundizar en un tema específico:
```bash
backscroll search "TOPIC" --all-projects --robot --max-tokens 4000
```

Analizar los resultados y sintetizar una distribución de temas discutidos. Agrupar por proyecto y tema frecuente.

#### 2e. Contexto (`--context`)

Requiere rootline y context-save. Ver [ref-context-mode.md](ref-context-mode.md) para el procedimiento completo de queries rootline (session-state, líneas activas, investigaciones, roadmap, teorías).

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
