# Backscroll: Research — Next Features (Marzo 2026)

**Fecha**: 2026-03-20
**Contexto**: Gap analysis completo de E01–E15 (todos implementados). Este documento captura investigacion de factibilidad para el siguiente ciclo de desarrollo (E16–E18+) basado en pain points reales de usuarios y analisis competitivo.

## 1. Landscape Competitivo

Herramientas que usuarios usan para buscar sesiones de Claude Code (2026):

| Herramienta | Enfoque | Ventaja de Backscroll |
|-------------|---------|----------------------|
| [claude-history](https://github.com/raine/claude-history) | Fuzzy search CLI, sin indice persistente | Indice FTS5 persistente, BM25 ranking, sync incremental |
| Session Finder (MCP Skill) | Servidor MCP, busqueda por titulo | Multi-source (sessions + plans), filtros ricos |
| Session Search (MCP Skill) | Servidor MCP | Sync incremental, no re-escanea |
| CCHV | GUI browser | CLI-native, scriptable, output LLM-friendly |
| Mantra | Session manager | Full-text search vs solo titulos |

**Moat de Backscroll**: Unica herramienta con indexing incremental persistente + multi-source (sessions + plans) + ranked search + corpus maintenance (reindex/purge/validate).

**Gap competitivo**: Multiples competidores operan como MCP servers — integracion nativa con Claude Code. Backscroll requiere skill wrapper con shell indirection.

### Fuentes
- [DEV Community — 4 Tools for Claude Code Session History](https://dev.to/gonewx/i-tested-4-tools-for-browsing-claude-code-session-history-17ie)
- [Definite — Building a Claude Code Skill to Search Past Sessions](https://www.definite.app/blog/claude-code-search-skill)
- [MCPMarket — Session Finder](https://mcpmarket.com/tools/skills/session-finder-for-claude-code)

---

## 2. Pain Points Identificados Online

### 2.1 Busqueda por variaciones morfologicas (stemming)

**Problema**: FTS5 con `unicode61` requiere match exacto. "error" no encuentra "errors".

**Evidencia**: Queja comun en foros de SQLite FTS5. Documentada en [SQLite FTS5 docs](https://www.sqlite.org/fts5.html) y [Medium — FTS in SQLite practical guide](https://medium.com/@johnidouglasmarangon/full-text-search-in-sqlite-a-practical-guide).

**Solucion**: Cambiar tokenizer a `porter unicode61` — built-in en SQLite, 1 linea de SQL. Requiere schema migration + reindex.

**Riesgo**: Over-matching. Porter stemmer reduce "universe" y "university" al mismo stem "univers". Evaluacion empírica necesaria sobre corpus real.

**Factibilidad**: Alta. Implementado como E16/F01.

### 2.2 Busqueda semantica (embeddings)

**Problema**: "Discutimos autenticacion la semana pasada" — el usuario recuerda el concepto, no las palabras exactas.

**Evidencia**: [Medium — Semantic search cuts AI code complexity](https://medium.com/@ricoledan/navigate-by-meaning-5f12910b6955), [Google Cloud — What is semantic search](https://cloud.google.com/discover/what-is-semantic-search).

**Solucion propuesta**: Capa opcional de embeddings (Ollama local o API) + hybrid scoring (BM25 + cosine similarity).

**Trade-offs**:

| Opcion | Pro | Contra |
|--------|-----|--------|
| Ollama local | Gratis, privado | Requiere GPU, ~384-dim, lento en sync |
| API (Anthropic/OpenAI) | Alta calidad | Costo por token, requiere internet, privacy |
| SQLite vec extension | Integrado | Experimental, performance no probada |
| Tantivy | Maduro en Rust | Cambio de backend, complejidad |

**Factibilidad**: Media. Requiere prototipo antes de comprometer. Ningun competidor de session search ofrece semantic search — first-mover advantage.

**Recomendacion**: Prototipar con Ollama embeddings en subset de corpus (100 sesiones). Medir recall improvement vs BM25 solo. Si delta > 20%, planificar como E19.

### 2.3 Corrupcion de sessions-index.json

**Problema**: Usuarios reportan perdida de sesiones despues de actualizaciones de Claude Code por corrupcion de sessions-index.json.

**Evidencia**: [GitHub — anthropics/claude-code#29154](https://github.com/anthropics/claude-code/issues/29154).

**Impacto en Backscroll**: Positivo — SHA-256 dedup y sync desde archivos JSONL (no desde index) hace a backscroll resiliente a esta corrupcion. Oportunidad de marketing.

### 2.4 Insights y analytics de sesiones

**Problema**: Usuarios quieren entender patrones de trabajo, no solo buscar contenido.

**Evidencia**: Anthropic lanzo `/insights` (Feb 2026) con analisis HTML interactivo — [angelo-lima.fr](https://angelo-lima.fr/en/claude-code-insights-command/).

**Solucion**: `backscroll insights` con agregaciones SQL (sesiones/dia, distribucion de categorias).

**Factibilidad**: Alta. Datos ya existen en SQLite. Implementado como E17/F02.

### 2.5 Integracion nativa (MCP)

**Problema**: CLI tools requieren shell wrapping para integrarse con Claude Code. MCP es el standard de facto.

**Evidencia**: Multiples session search tools ya son MCP servers.

**Solucion**: `backscroll serve` como MCP stdio server.

**Riesgo**: Madurez del ecosistema Rust MCP SDK. `rmcp` y `mcp-rs` son jovenes.

**Factibilidad**: Media. Requiere evaluacion de SDK antes de comprometer. Implementado como E18/F02 con research gate.

---

## 3. Incertidumbres Abiertas del Research Original

### S2: Valor de sesiones de subagentes

**Pregunta**: Excluir 75% del corpus (1,640 de 2,191 archivos subagent) — se pierde contexto valioso?

**Estado**: Abierto. `--include-agents` existe pero nadie ha evaluado calidad de resultados.

**Metodo de investigacion**: Sync con y sin `--include-agents`, comparar resultados de 10 queries frecuentes. Medir precision y noise ratio.

**Factibilidad de investigacion**: Alta — experiment puede ejecutarse en <1 hora.

### S4: Deteccion de proyecto sin sessions-index.json

**Pregunta**: Fallback de deteccion por directorio — que tan preciso es?

**Estado**: Parcialmente resuelto. Implementado pero no validado empiricamente.

**Metodo de investigacion**: Comparar `project` column en DB vs directorio real de sesion para todos los archivos indexados.

### M7: Asociacion plan-a-proyecto

**Pregunta**: Pueden los plans asociarse a proyectos?

**Estado**: Abierto. Plans son globales (`~/.claude/plans/`), no tienen project identifier.

**Metodo de investigacion**: Analizar filenames y contenido de plans para detectar referencias a proyectos. Evaluar si plan filenames contienen slugs de proyecto.

---

## 4. Roadmap Priorizado

| Prioridad | Epic | Esfuerzo | Impacto | Dependencias |
|-----------|------|----------|---------|-------------|
| P1 | E16: Search Quality | Bajo-Medio | Alto | Ninguna |
| P2 | E17: Session Insights | Medio | Medio-Alto | E16 (comparte schema migration) |
| P3 | E18/F01: Export | Bajo | Medio | Ninguna |
| P4 | E18/F02: MCP Server | Medio-Alto | Alto | Investigacion SDK |
| P5 | Semantic Search (E19?) | Alto | Muy Alto | Prototipo Ollama |

## 5. Siguiente Accion

1. Resolver S2 (subagent value) con experimento empirico
2. Iniciar E16/F01 (Porter stemmer) — quick win con alto impacto
3. Evaluar Rust MCP SDKs para gate de E18/F02
