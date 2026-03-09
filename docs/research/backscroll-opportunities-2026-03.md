---
estado: "Fase 3"
fecha: "2026-03-09"
metodo: hypothesize
origen: "Unificación de research docs + análisis estado del arte marzo 2026"
fase_actual: 3
---
# Backscroll — Análisis de Oportunidades (Marzo 2026)

**Fecha**: 2026-03-09
**Tipo**: Research — Gap Analysis & Opportunity Map
**Método**: Cruce de research existente + estado del arte + análisis de implementación actual
**Fuentes**: `backscroll-session-search-cli.md`, `backscroll-rust-architecture-2026.md`, codebase actual, ecosistema marzo 2026

**Posición arquitectónica**: Backscroll es componente Tier 2 (full-text search) del ecosistema Kedral (D5). No es herramienta standalone para usuarios finales. Kedral = KEDB orchestrator. Backscroll = event store + búsqueda. Rootline = structured store + validación. La integración Kedral→backscroll es via CLI+skill, no via MCP (ver sección 4.2).

---

## 1. Estado actual del proyecto

### v1 completada — 50/50 tasks across E01-E09

| Capability | Estado | Evidencia |
|-----------|--------|-----------|
| M1: FTS5 + BM25 + snippet | ✅ Implementada | `storage/sqlite.rs`: FTS5 external content, ORDER BY rank |
| M2: Sync incremental SHA-256 | ✅ Implementada | `core/sync.rs`: hash dedup, skip unchanged |
| M3: Scoping por proyecto | ✅ Implementada | sessions-index.json + path fallback |
| M4: `read` mode | ✅ Implementada | `core/reader.rs` + subcommand |
| M5: Output LLM-native | ✅ Implementada | Text/JSON/Robot, --fields, --max-tokens |
| M6: Búsqueda unificada sessions+plans | ❌ No iniciada | Diferida a v2 (D3) |
| M7: Asociación plan→proyecto | ❌ No iniciada | Diferida a v2 (D3) |

### Qué NO se implementó del research original

| Item | Descripción | Razón |
|------|------------|-------|
| Plan indexing (M6/M7) | Búsqueda unificada sessions+plans | Diferido por diseño (D3): sin demand signal directo |
| S2 validation | ¿Subagent sessions tienen valor real? | Nunca investigado; marcado como ❓ unknown |
| `--topics` mode | Análisis de temas por sesión | Mencionado en spec v1 pero no implementado |
| T-01..T-05 spikes | Claims técnicos de Fase 4 | Resueltos implícitamente por la implementación, no validados formalmente |

---

## 2. Evolución del ecosistema (Feb → Mar 2026)

### 2.1 Herramientas de sesión existentes

Todas las herramientas del ecosistema son **user-facing** (TUI, dashboards, fuzzy pickers). Ninguna está diseñada como componente de integración programática. Backscroll no compite con ellas por usuarios — opera en un nicho diferente (componente CLI consumido por skills/programas).

| Tool | Stack | Enfoque | Categoría |
|------|-------|---------|-----------|
| **CASS** | Python/Rust | Hybrid BM25+semántica+RRF, 11 providers, `cass_memory_system` | Search standalone (user-facing) |
| **ccboard** | Rust TUI + Leptos/WASM | FTS5, 11 tabs, cost tracking, 377 tests | Dashboard (user-facing) |
| **ccsearch** | Rust | Hybrid BM25+vector+RRF | Search standalone (user-facing) |
| **recall** | Go/Rust | FTS + resume para Claude/Codex/OpenCode | Resume (user-facing) |
| **claude-history** | Rust | Fuzzy search + noise filtering | Search standalone (user-facing) |
| **claude-code-log** | Python | TUI interactivo, resumen, exportación | Dashboard (user-facing) |
| **Agent Sessions** | Swift macOS | Browser + analytics, 15 providers | Dashboard (user-facing) |
| **cc-sessions** | Node | fzf picker | Picker (user-facing) |
| **CTK** | Python/SQLite | Export + search | Search standalone (user-facing) |
| **claude-code-sync** | Rust | Git sync, smart merge por UUID | Sync (user-facing) |
| **meta_skill** | Rust | Session mining → skill generation | Complementario — podría consumir backscroll |

**Pregunta competitiva relevante**: ¿Podría Kedral usar alguno de estos en lugar de backscroll? No — ninguno ofrece output LLM-native (`--robot`, `--max-tokens`), CLI stateless optimizado para subprocess invocation, ni diseño como componente de integración.

### 2.2 Tendencias del ecosistema

1. **Hybrid search es table stakes entre tools user-facing**: ≥5 tools implementan BM25 + vector embeddings + RRF fusion. Irrelevante para backscroll: BM25 es suficiente cuando el consumidor (skill) puede hacer queries iterativas refinadas.
2. **Multi-agent support**: CASS, recall, Agent Sessions soportan 11-15+ providers. Irrelevante para backscroll: Claude Code-only por diseño (D5).
3. **Dashboard proliferation**: ≥6 proyectos de observabilidad. Irrelevante: backscroll no sirve usuarios directamente.
4. **Session-as-memory**: CASS → `cass_memory_system`. Fuera de scope: la capa de memoria sería consumidor de backscroll, no parte.
5. **MCP context injection**: Tools inyectan contexto via MCP servers. **Evaluado y descartado** para backscroll — CLI+skill es 96-99% más eficiente en tokens (ver sección 4.2).
6. **Agentic search > RAG puro**: El modelo decide qué buscar via queries iterativas. **Valida** el approach de backscroll: BM25 simple + skill que permite al modelo refinar queries es más efectivo que semántica compleja.

### 2.3 Cambios en el entorno de Claude Code

| Cambio | Impacto en backscroll | Acción |
|--------|----------------------|--------|
| `sessions-index.json` dejó de actualizarse (~v2.1.31+) | ✅ Favorable — valida WalkDir+SHA-256 | Ninguna; approach ya es correcto |
| Migración `local-agent-mode-sessions/` → `claude-code-sessions/` | ⚠️ Pérdida silenciosa de sesiones legacy | Auto-discovery de directorios (O-07) |
| Plans creciendo (~300+ archivos en `~/.claude/plans/`) | Corpus sin indexar | Plan indexing (O-01) |

---

## 3. Problemas del ecosistema que afectan calidad de datos

Estos pain points de la comunidad son relevantes porque afectan la **completitud y calidad de datos** que backscroll indexa para Kedral, no porque backscroll los resuelva directamente para usuarios.

| Problema | Evidencia | Impacto en datos de backscroll |
|----------|-----------|-------------------------------|
| **sessions-index.json roto** (~v2.1.31+) | Issues #25032, #26485, #24729, #23614, #29778 | ✅ Sin impacto — backscroll usa WalkDir + SHA-256 |
| **Migración de directorio** (v1.1.4328) | Issues #29373, #29154 | ⚠️ Sesiones legacy invisibles si no se configura path manual |
| **Compaction** hace historial inaccesible (#27242) | JSONL preserva datos, UI no | ✅ Sin impacto — `backscroll read` accede directo al JSONL |
| **No hay búsqueda cross-project** | Community demand | ✅ Resuelto — `backscroll search` sin `--project` busca todo |
| **Plans no indexados** | 300+ archivos sin búsqueda | ⚠️ Corpus incompleto para KEDB |

---

## 4. Mapa de oportunidades

### 4.1 Oportunidades por impacto y esfuerzo

Impacto medido como **valor para Kedral como consumidor** (completitud de datos, capabilities de integración), no como valor para usuarios directos.

```
               ALTO IMPACTO (para KEDB)
                       │
  ┌────────────────────┼────────────────────┐
  │                    │                    │
  │  O-07 Dir discovery│  O-04 Semantic     │
  │  O-01 Plan indexing│  O-06 Multi-agent  │
  │  O-10 Resume (CLI) │                    │
  │                    │                    │
  ├────────────────────┼────────────────────┤
  │                    │                    │
  │  O-03 Topics       │  O-05 Memory       │
  │  O-08 Cost metrics │  O-09 TUI          │
  │  O-11 Source filter│  O-02 MCP server   │
  │                    │                    │
  └────────────────────┼────────────────────┘
                       │
  BAJO ESFUERZO ───────┼──────── ALTO ESFUERZO
               BAJO IMPACTO (para KEDB)
```

**Cambios vs análisis previo**:
- O-02 (MCP) baja a bajo-impacto/alto-esfuerzo — CLI+skill ya integra, MCP añade overhead sin beneficio
- O-10 (Resume) sube — capability consumida por skill, no feature de usuario
- O-11 (Source filter) es nueva — `--source sessions|plans` para que skills distingan fuentes

### 4.2 MCP vs CLI+Skill: análisis de costo

Backscroll se integra via skill que ejecuta `backscroll search --robot` como subprocess Bash. Se evaluó MCP como alternativa de integración y se descartó por overhead de tokens:

| Approach | Overhead por sesión | Por operación | Fuente |
|----------|-------------------|---------------|--------|
| **CLI via Bash** (actual) | ~245 tokens (tool def, cacheado) | ~50 tokens | Claude Code internals |
| **MCP server (3 tools)** | ~750-3,000 tokens (schemas en contexto) | ~50 tokens | SEP-1576, mcp2cli benchmarks |
| **MCP server típico (30 tools)** | ~7,500-30,000 tokens | ~50 tokens | Layered Systems analysis |

**Por qué CLI gana para backscroll**:
- Binario local, operaciones stateless → no necesita servidor persistente
- Output ya optimizado para LLM (`--robot`, `--max-tokens`)
- El modelo ya sabe invocar CLIs — entrenado en billones de interacciones de terminal
- Skill puede hacer queries iterativas (agentic search) sin overhead de protocol
- 96-99% menos tokens que MCP nativo (fuente: mcp2cli benchmarks, Speakeasy analysis)

**MCP tendría sentido si**: backscroll fuera servicio remoto, necesitara autenticación OAuth, o fuera consumido por clients que no tienen Bash (e.g., web apps). Ninguno aplica.

---

### O-01: Plan indexing (v2 — M6)

**Origen**: Research original, D3 (diferido a v2)
**Estado**: No iniciado. Schema ya preparado (columna `source` en `search_items`).

**Qué implica**:
- Parsear `~/.claude/plans/*.md` (300+ archivos, flat directory)
- Splitear por `##` headers (~6 secciones/plan)
- INSERT con `source='plan'` en tabla existente
- Flag `--source plans|sessions` para filtrar por fuente

**Esfuerzo**: Medio (schema listo, solo parser + sync pipeline)
**Impacto para KEDB**: Alto — 300+ plans representan decisiones de arquitectura, diseño, y contexto que la KEDB necesita indexar. Demand signal es programático (528 refs en skills/agents).

**Riesgo**: Plans son archivos markdown globales sin metadata de proyecto. Asociación plan→proyecto (M7) requiere heurísticas frágiles → se excluye de scope.

**Recomendación**: Implementar indexación básica sin asociación plan→proyecto.

---

### O-07: Resiliencia ante migración de directorios

**Origen**: Migración v1.1.4328 (`local-agent-mode-sessions/` → `claude-code-sessions/`)
**Estado**: Parcialmente cubierto (backscroll indexa paths configurados).

**Qué implica**:
- Detectar automáticamente ambos directorios (legacy + nuevo)
- Configuración de múltiples paths de sesión
- Re-scan cuando se detectan paths nuevos

**Esfuerzo**: Bajo (ya usa WalkDir, solo agregar path discovery)
**Impacto para KEDB**: Alto — sin auto-discovery, sesiones legacy son invisibles. KEDB tiene datos incompletos.

**Recomendación**: Quick win. Prioridad máxima por impacto en completitud de datos.

---

### O-10: Resume via CLI

**Origen**: sessions-index.json roto impide `/resume` nativo de Claude Code
**Estado**: No implementado.

**Qué implica**:
- `backscroll resume <query> --robot` → session ID + path (una línea, pipe-friendly)
- `get_session_id()` en SearchEngine trait (resuelve path → UUID)
- Integración con `claude --resume <session-id>` via skill

**Esfuerzo**: Bajo (search ya existe, solo agregar resolución de session ID + subcommand)
**Impacto para KEDB**: Medio — permite a Kedral resolver "¿cuál fue la sesión sobre X?" programáticamente.

**Recomendación**: Implementar como CLI subcommand. El skill lo consume via Bash.

---

### O-11: Source filter (nuevo)

**Origen**: Necesidad de distinguir sessions de plans una vez ambos estén indexados (O-01)
**Estado**: Nuevo — emerge de O-01.

**Qué implica**:
- Flag `--source sessions|plans` en `backscroll search`
- Filtro SQL en WHERE clause usando columna `source` existente
- Default: buscar en todo (preserva comportamiento actual)

**Esfuerzo**: Bajo (columna ya existe en schema, solo agregar flag + WHERE)
**Impacto para KEDB**: Medio — permite a skills buscar solo en el corpus relevante.

**Recomendación**: Implementar junto con O-01 (plan indexing).

---

### Oportunidades descartadas

| ID | Oportunidad | Razón de descarte |
|----|------------|-------------------|
| O-02 | **MCP server** | CLI+skill es la integración natural para Kedral. MCP añade 750-3000 tokens/sesión de overhead sin beneficio funcional. Backscroll es local y stateless — no necesita protocolo de servidor. Ver sección 4.2 |
| O-03 | Topics/clustering | Nice-to-have. No afecta completitud de datos ni capabilities de integración |
| O-04 | Semantic search | BM25 suficiente + queries iterativas via skill compensan. Complejidad desproporcionada para corpus single-user |
| O-05 | Session memory | Capa superior — debe ser consumidor de backscroll, no parte |
| O-06 | Multi-agent | Scope creep. Backscroll es componente Kedral, Claude Code-only por diseño (D5) |
| O-08 | Cost metrics | Fuera del core de búsqueda. Si se necesita, otro componente puede parsear JSONL |
| O-09 | TUI | Backscroll no sirve usuarios directamente. TUI es para herramientas user-facing |

---

## 5. Incertidumbres heredadas del research original

| ID | Incertidumbre | Estado actual | Acción recomendada |
|----|--------------|--------------|-------------------|
| S1 | Formato JSONL estable | ✅ Confirmado estable (core). Nuevos campos: `thinkingMetadata`, `isMeta`, `todos`, `agentId`. Bug #22526 (parentUuid) activo. | No action needed — parser defensivo funciona |
| S2 | Subagent sessions tienen valor | ❓ Nunca investigado | Investigar empíricamente: sample 50 subagent sessions, evaluar ratio señal/ruido |
| S3 | FTS5 performance suficiente | ✅ Confirmado — Rust+rusqlite (bundled) elimina el riesgo de modernc | No action needed |
| S4 | Scoping via sessions-index.json | ⚠️ **Empeorado** — sessions-index.json dejó de actualizarse (~v2.1.31+) | Path fallback es ahora el mecanismo primario de facto. Considerar deprecar sessions-index.json como fuente |

---

## 6. Priorización recomendada

Evaluada por **valor para Kedral** (completitud de datos, capabilities de integración), no por valor para usuarios directos.

### Tier 1 — Completitud de datos

| # | Oportunidad | Esfuerzo | Impacto | Razón |
|---|------------|----------|---------|-------|
| 1 | **O-07: Auto-discovery de directorios** | Bajo | Alto | Sin esto, KEDB tiene datos incompletos. Quick win |
| 2 | **O-01: Plan indexing** | Medio | Alto | 300+ plans = decisiones de arquitectura sin indexar. Extiende corpus para KEDB |

### Tier 2 — Capabilities de integración

| # | Oportunidad | Esfuerzo | Impacto | Razón |
|---|------------|----------|---------|-------|
| 3 | **O-10: Resume via CLI** | Bajo | Medio | Permite resolución programática de session IDs. Skill lo consume via Bash |
| 4 | **O-11: Source filter** | Bajo | Medio | Distinguir sessions de plans. Implementar junto con O-01 |

### Descartado

| Oportunidad | Razón |
|------------|-------|
| O-02: MCP server | CLI+skill más eficiente (96-99% menos tokens). No hay beneficio funcional |
| O-03: Topics | No afecta datos ni integración |
| O-04: Semantic search | BM25 + queries iterativas es suficiente |
| O-05: Session memory | Capa superior, no backscroll |
| O-06: Multi-agent | Fuera de scope Kedral |
| O-08: Cost metrics | Fuera del core de búsqueda |
| O-09: TUI | Backscroll no sirve usuarios |

---

## 7. Posición en el ecosistema

### Nicho de backscroll

Backscroll es el único tool del ecosistema diseñado como **componente de integración programática**. Todas las demás herramientas son user-facing (TUI, dashboards, pickers). Esto no es una debilidad — es la posición de diseño correcta para un componente Kedral Tier 2.

### Fortalezas como componente

1. **No depende de sessions-index.json** — WalkDir + SHA-256 es el approach correcto ahora que el índice nativo está roto
2. **Output LLM-native by design** — `--robot`, `--fields`, `--max-tokens` como ciudadanos de primera clase. Ningún competidor ofrece esto
3. **Binario estático zero-deps** — rusqlite bundled + zigbuild = distribución trivial
4. **Ports & adapters** — SearchEngine trait permite migración futura a Tantivy sin rewrite
5. **CLI stateless** — integración via skill sin overhead de protocolo (MCP, HTTP, etc.)
6. **Integración probada** — skill `/sessions` ya consume backscroll via Bash en producción

### Lo que NO necesita

1. **Búsqueda semántica** — el skill permite queries iterativas (agentic search), compensando BM25-only
2. **Multi-agent** — Claude Code-only por diseño, Kedral orquesta
3. **TUI/Dashboard** — los usuarios nunca interactúan directamente con backscroll
4. **MCP server** — CLI+skill es más eficiente en tokens y ya funciona

### Ventaja defensiva

La degradación de `sessions-index.json` es un evento favorable. Herramientas que dependen del índice (cc-sessions, /resume nativo) están rotas. Backscroll es una de las pocas que funciona de forma confiable via file traversal directa.

---

## 8. Decisiones pendientes

| ID | Decisión | Opciones | Criterio |
|----|---------|----------|---------|
| D-01 | ¿Deprecar sessions-index.json como fuente? | (a) Mantener como enrichment opcional (b) Eliminar | sessions-index.json no se actualiza desde ~v2.1.31. Path fallback ya es de facto primario |
| D-02 | ¿Investigar S2 (valor de subagents)? | (a) Sample empírico 50 sesiones (b) Mantener como unknown | Informa si `--include-agents` debería invertir su default |

**Decisiones resueltas** (este análisis):
- ~~D-03: ¿MCP server?~~ → **No**. CLI+skill es la integración correcta. MCP añade overhead sin beneficio (ver 4.2)
- ~~D-04: ¿Plan indexing antes o después de MCP?~~ → **Plan indexing sin MCP**. MCP descartado

---

## Apéndice A: Fuentes consultadas

### Issues de Claude Code (evidencia de problemas del ecosistema)
- [#25032](https://github.com/anthropics/claude-code/issues/25032) — sessions-index.json not updating
- [#26485](https://github.com/anthropics/claude-code/issues/26485) — sessions-index.json empty summaries
- [#29373](https://github.com/anthropics/claude-code/issues/29373) — directory migration breaks sessions
- [#29154](https://github.com/anthropics/claude-code/issues/29154) — local-agent-mode-sessions → claude-code-sessions
- [#22526](https://github.com/anthropics/claude-code/issues/22526) — parentUuid corruption
- [#27242](https://github.com/anthropics/claude-code/issues/27242) — compaction makes history inaccessible

### Repositorios evaluados
- [CASS v0.2.3+](https://github.com/Dicklesworthstone/coding_agent_session_search)
- [ccboard](https://github.com/FlorianBruniaux/ccboard)
- [ccsearch](https://github.com/madzarm/ccsearch)
- [recall](https://github.com/zippoxer/recall)
- [Agent Sessions](https://github.com/jazzyalex/agent-sessions)
- [meta_skill](https://github.com/Dicklesworthstone/meta_skill)
- [claude-code-log v1.1.0](https://github.com/daaain/claude-code-log)
- [claude-history](https://github.com/raine/claude-history)

### Análisis MCP vs CLI token costs
- [SEP-1576: Mitigating Token Bloat in MCP](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/1576)
- [MCP Tool Schema Bloat: The Hidden Token Tax — Layered Systems](https://layered.dev/mcp-tool-schema-bloat-the-hidden-token-tax-and-how-to-fix-it/)
- [Mcp2cli — 96-99% fewer tokens than native MCP](https://dev.to/mgobea/show-hn-mcp2cli-one-cli-for-every-api-96-99-fewer-tokens-than-native-mcp-5c49)
- [Reducing MCP token usage by 100x — Speakeasy](https://www.speakeasy.com/blog/how-we-reduced-token-usage-by-100x-dynamic-toolsets-v2)
- [Built-in tools + MCP causing 10-20k token overhead — Claude Code #3406](https://github.com/anthropics/claude-code/issues/3406)

### Research original del proyecto
- `docs/research/backscroll-session-search-cli.md` — Feasibility study (Feb 2026)
- `docs/research/backscroll-rust-architecture-2026.md` — Architecture pivot (Mar 2026)
