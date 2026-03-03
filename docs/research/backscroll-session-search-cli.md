# Backscroll — Investigación Estructurada

**Fecha**: 2026-02-18 (research original) / 2026-03-03 (reestructuración)
**Tipo**: Research
**Ecosistema**: Backscroll provee Tier 2 search para [Kedral](kedral/README.md) (Known Error Database). Backscroll = event store + búsqueda. Kedral = bridge entity + lifecycle. Rootline = structured store + validación.

> Estado: Fase 3 parcial. CAP-02 (vs CASS) pendiente evaluación empírica. Fases 4-5 en construcción.

---

## Glosario de dominio

| Término | Definición |
|---------|-----------|
| Session | Conversación Claude Code almacenada como archivo JSONL en `~/.claude/projects/<slug>/` |
| Plan | Archivo markdown generado por plan mode en `~/.claude/plans/*.md` |
| FTS5 | Full-Text Search 5, extensión de SQLite para búsqueda indexada con bm25 y snippet |
| Subagent session | JSONL generado por agentes delegados (prefijo `agent-` en filename) |
| sessions-index.json | Índice automático de Claude Code con `summary` + `firstPrompt` por sesión |
| CASS | Coding Agent Session Search — herramienta Rust/Tantivy con búsqueda híbrida BM25+semántica |
| Kedral | Known Error Database orchestrator que delega Tier 1 (structured) a Rootline y Tier 2 (full-text) a Backscroll |
| Content heuristic | Método de asociación plan→proyecto basado en markers del proyecto encontrados en el contenido del plan |

---

## Fase 1: Idea → Tesis

### Axiomas del entorno [VERIFICADO 2026-03-03]

| ID | Axioma | Evidencia |
|----|--------|-----------|
| C1 | Claude Code almacena sesiones como JSONL sin búsqueda full-text nativa | Verificado: `sessions-index.json` solo contiene `summary` + `firstPrompt` |
| C2 | No existe schema oficial del formato JSONL; parseo debe ser defensivo | Verificado: campos estables observados pero sin garantía de estabilidad |
| C3 | El corpus crece ~12 sessions/día por proyecto activo (~15-19 MB/día) | Medido: rootline 167 sessions en 12 días, homeserver 310 en 26 días |
| C4 | El entorno es single-user, single-host (Proxmox) | No hay requisitos multi-usuario ni sync remoto |
| C5 | 74.9% del total de archivos JSONL son subagent sessions (1,640 de 2,191) | Medido: subagents dominan el volumen |
| C6 | Plans son globales (`~/.claude/plans/` flat) sin separación por proyecto | Verificado: 300 plans en un directorio, sin índice |
| C7 | `content` en JSONL puede ser string o array de ContentBlocks | Verificado: parser debe manejar ambos formatos |

### Premisas de diseño [VERIFICADO]

| ID | Premisa | Tipo |
|----|---------|------|
| D1 | Stack: Go + modernc.org/sqlite (binario estático, zero CGO, FTS5 built-in) | Decisión tomada |
| D2 | CLI on-demand, no daemon (incremental sync al invocar) | Decisión tomada |
| D3 | v1 solo sessions; plans diferidos a v2 | Decisión 2026-03-03 |
| D4 | Tabla FTS5 única con columna `source` (no tablas separadas) | Decisión 2026-03-03, resuelve bm25 cross-table |
| D5 | Backscroll es componente del ecosistema Kedral, no herramienta standalone aislada | Decisión de arquitectura |

### Modelo deseado [VERIFICADO]

| ID | Capacidad | Prioridad |
|----|-----------|-----------|
| M1 | Búsqueda full-text indexada sobre mensajes de usuario Y assistant con snippet + bm25 | v1 |
| M2 | Indexación incremental mtime-based (<50ms para 0-5 archivos nuevos) | v1 |
| M3 | Scoping por proyecto actual (detectado via git root → projectPath) | v1 |
| M4 | Modo --read para extraer contenido filtrado de una sesión | v1 |
| M5 | Output compacto optimizado para consumo por LLM (no TUI) | v1 |
| M6 | Búsqueda unificada sessions + plans con ranking comparable | v2 |
| M7 | Asociación plan→proyecto (session refs + content heuristics) | v2 |

### Clarificaciones (falacias resueltas)

| ID | Falacia detectada | Resolución |
|----|-------------------|-----------|
| F1 | "Baja frecuencia de uso de `/sessions` (5 invocaciones) implica baja demanda" | Falso: el skill actual es tan malo que no se usa. La demanda se mide por el problema (941 MB sin indexar), no por el uso de una herramienta rota. |
| F2 | "Research docs como workaround para sessions" | Falso: son productos distintos. Research docs son output estructurado creado durante sessions. Sessions son el registro crudo. No se sustituyen. |
| F3 | "bm25() es comparable entre tablas FTS5 distintas" | Falso: IDF y avgdl son per-table. Resuelto con tabla única (D4). |
| F4 | "Plan search tiene demand signal del usuario directo" | Parcialmente falso: 528 refs a nivel sistema (skills, agents), 0 búsquedas directas del usuario. Demand real es programática. Diferido a v2 (D3). |

### Mapa de inferencias

```
C1 (no hay FTS nativo) + C3 (corpus crece 15-19 MB/día)
  → El problema empeora con el tiempo
  → M1 (búsqueda FTS5 indexada) es necesaria

C2 (sin schema oficial) + C7 (content string|array)
  → M2 requiere parseo defensivo
  → ⚠ Riesgo: formato puede cambiar sin aviso (S1)

D1 (Go + modernc FTS5) + D2 (CLI on-demand)
  → Binario estático, zero deps, <50ms incremental
  → ⚠ Pendiente: ¿CASS (Rust) resuelve mejor? (CAP-02)

D3 (v1 solo sessions) + D4 (tabla FTS5 única)
  → Schema simple en v1, extensible a plans en v2 sin migración
  → bm25 comparable by design desde v1

C5 (74.9% subagents) + M3 (scoping por proyecto)
  → Decisión: ¿indexar subagent sessions? Incluirlas aumenta coverage pero también ruido
  → ⚠ Requiere evaluación (S2)
```

### Supuestos explícitos

| ID | Supuesto | Impacto si falso |
|----|----------|-----------------|
| S1 | El formato JSONL de Claude Code se mantiene suficientemente estable para parseo defensivo | Parser se rompe; requiere actualización reactiva |
| S2 | Subagent sessions contienen información valiosa para búsqueda (no solo ruido de herramientas) | Indexar 1,640 archivos extra sin valor; filtrarlos reduce corpus a 551 |
| S3 | modernc.org/sqlite FTS5 performance es suficiente para ~2,000 archivos, ~1 GB | Primera indexación toma >30s; usuario percibe lentitud inaceptable |
| S4 | El proyecto actual se puede detectar confiablemente via git root → projectPath en sessions-index.json | Scoping falla en repos sin sessions-index.json (6 de 11 proyectos no lo tienen) |

---

## Fase 2: Tesis → Plan de investigación

### Hipótesis testable (H1)

**H1**: Existe una CLI en Go que, usando SQLite FTS5 con tabla única y sincronización incremental mtime-based, permite buscar full-text sobre sesiones de Claude Code con latencia <1s en búsqueda y <50ms en sync incremental, produciendo output útil para consumo por LLM, bajo el corpus actual (2,191 archivos, 941 MB, 11 proyectos).

**Condición de falsación**: H1 es falsa si (a) CASS ya resuelve el problema sin necesidad de construir Backscroll, (b) modernc.org/sqlite FTS5 no soporta el volumen, o (c) el formato JSONL cambia de forma que invalida el parser.

### Capabilities mínimas (CAPs)

| ID | Capability | Método | Dependencias | Prioridad | Crítica? |
|----|-----------|--------|-------------|-----------|---------|
| CAP-01 | modernc.org/sqlite FTS5 funciona para el volumen requerido | Empírico | Ninguna | Alta | Sí |
| CAP-02 | Backscroll aporta valor diferencial vs CASS | Empírico | Instalar CASS | Alta | Sí |
| CAP-03 | Parser JSONL defensivo maneja formatos actuales sin pérdida | Empírico | CAP-01 | Alta | Sí |
| CAP-04 | Scoping por proyecto funciona en los 11 proyectos | Empírico | CAP-03 | Media | No |
| CAP-05 | Sync incremental <50ms con corpus de 2,191 archivos | Empírico | CAP-01, CAP-03 | Media | No |
| CAP-06 | Output es consumible por LLM (snippet + contexto suficiente) | Lógico | CAP-03 | Media | No |

### Sub-hipótesis

| ID | Sub-hipótesis | Pregunta de falsación |
|----|--------------|----------------------|
| H1-a | Si modernc.org/sqlite compila con FTS5 enabled y opera sobre ~50K rows con snippet+bm25, entonces la búsqueda retorna en <1ms porque FTS5 usa índice invertido optimizado | ¿Es falso que modernc.org/sqlite FTS5 soporta snippet+bm25 sobre 50K rows en <1ms? |
| H1-b | Si CASS (v0.1.53) no produce output para LLM, no indexa plans, y requiere Rust runtime, entonces Backscroll aporta valor diferencial porque ofrece Go binary + LLM output + ecosistema Kedral | ¿Es falso que CASS no cubre output LLM, plan indexing, y zero-dep binary? |
| H1-c | Si el parser maneja `content` como string y como array de ContentBlocks, filtra 6 patrones de ruido, y usa skip-on-error, entonces procesa >=95% de los JSONL actuales sin crash | ¿Es falso que el parser maneja >=95% de los archivos JSONL actuales? |
| H1-d | Si se usa git root → sessions-index.json lookup para detectar proyecto, entonces el scoping funciona en proyectos que tienen sessions-index.json (5/11) | ¿Es falso que git root → projectPath lookup funciona en los 5 proyectos con index? |
| H1-e | Si stat() de 2,191 archivos toma <10ms y solo se re-parsean archivos con mtime cambiado, entonces sync incremental <50ms | ¿Es falso que stat 2,191 files + 0-5 re-parses completa en <50ms? |
| H1-f | Si el output incluye snippet con 10 tokens de contexto, role prefix, fecha y slug, entonces un LLM puede determinar relevancia sin leer la sesión completa | ¿Es falso que snippet+metadata es suficiente para que un LLM evalúe relevancia? |

### Criterios de decisión

| Decisión | Condición Go | Condición Pivot | Condición Stop |
|----------|-------------|-----------------|---------------|
| CAP-01 FTS5 | FTS5 funciona, <1ms query | Performance >10ms → optimizar schema | FTS5 no disponible en modernc → cambiar a CGO |
| CAP-02 vs CASS | CASS no cubre >=2 de: LLM output, plan indexing, Kedral integration | CASS cubre 2/3 → wrapper sobre CASS | CASS cubre 3/3 → adoptar CASS, cancelar Backscroll |
| CAP-03 Parser | >=95% archivos parseados sin error | 90-95% → ampliar noise filters | <90% → formato cambió, re-investigar |

### Reglas de parada

| ID | Regla |
|----|-------|
| R1 | Si CAP-02 resulta Stop (CASS cubre todo), no invertir en CAP-01/03/04/05/06 |
| R2 | Si CAP-01 resulta Stop (FTS5 no funciona), evaluar CGO como alternativa antes de cancelar |
| R3 | Si CAP-03 muestra <90% parse rate, detener e investigar cambios de formato JSONL |

---

## Fase 3: Investigación → Argumento actualizado

### Matriz Premisa-Evidencia

| Premisa | Método | Evidencia | Calidad | Estado |
|---------|--------|-----------|---------|--------|
| C1: No hay FTS nativo en Claude Code | Empírico | sessions-index.json verificado: solo summary+firstPrompt | Alta | ✅ true |
| C2: Sin schema oficial JSONL | Empírico | No hay docs oficiales; campos observados estables desde v42+ | Media | ✅ true |
| C3: Corpus crece ~12 sessions/día | Empírico | Medido: rootline 167/12d, homeserver 310/26d | Alta | ✅ true |
| C5: 74.9% son subagent sessions | Empírico | Conteo directo: 1,640 agent-* de 2,191 total | Alta | ✅ true |
| C7: content string o array | Empírico | Observado en múltiples sesiones | Alta | ✅ true |
| D1: Go + modernc FTS5 funciona | Empírico | FTS5 compiled-in por defecto, CVE-2025-7709 parchado en 3.51.2, ~2x slower que CGO | Alta | ✅ true |
| D4: Tabla FTS5 única resuelve bm25 | Lógico | bm25 usa IDF per-table; corpus unificado → scores comparables by definition | Alta | ✅ true |
| S1: Formato JSONL estable | Empírico | Core estable; adiciones recientes: isMeta, thinkingMetadata, todos, agentId. Bug de UUID duplicado (#22526) | Media | ⚠️ parcial |
| S2: Subagent sessions tienen valor | Empírico | No investigado aún | — | ❓ unknown |
| S3: FTS5 performance suficiente | Empírico | Benchmark: modernc ~2x slower INSERT, 10-100% slower SELECT vs CGO. Regression 3.51.0 en prepared statements (8.4x). No bloqueante para scope personal | Media | ⚠️ parcial |
| S4: Scoping via sessions-index.json | Empírico | Solo 5/11 proyectos tienen sessions-index.json. Fallback necesario para los 6 restantes | Alta | ⚠️ parcial |
| CAP-02: Backscroll vs CASS | Empírico | CASS v0.1.53: BM25+semántica, 11 providers, <60ms. Backscroll diferencia: Go binary, LLM output, Kedral integration, plan indexing (v2) | Alta | ❓ pendiente evaluación |

### Evidencia lógica: Invariantes

| ID | Invariante | Derivado de |
|----|-----------|-------------|
| INV-01 | Toda búsqueda FTS5 retorna resultados ordenados por bm25 dentro del mismo corpus | D4 (tabla única) |
| INV-02 | Sync incremental solo re-parsea archivos con mtime > stored_mtime | D2 + M2 |
| INV-03 | Un mensaje indexado pertenece a exactamente un proyecto (determinado por el path del JSONL) | C1 (sessions scoped por directorio) |
| INV-04 | El parser nunca crashea por un archivo malformado; skip con warning | C2 + M1 (parseo defensivo) |

### Evidencia lógica: Constraints derivados

| ID | Constraint | Derivado de | Consecuencia |
|----|-----------|-------------|-------------|
| CD-01 | Primera indexación del corpus completo tomará ~10-15s (941 MB) | C3 + S3 | Aceptable: ocurre una vez; posteriores son incrementales |
| CD-02 | Proyectos sin sessions-index.json requieren fallback de scoping | S4 | Fallback: derivar project slug del path del directorio (`-opt-rootline` → `/opt/rootline`) |
| CD-03 | UUID duplicado entre archivos JSONL (#22526) requiere deduplicación | S1 | Deduplicar por UUID antes de INSERT |
| CD-04 | v2 (plans) no requiere migración de schema si tabla FTS5 ya tiene columna `source` | D4 + D3 | Solo INSERT nuevos rows con source='plan' |

### Registro de incertidumbre

| Item | Estado | Impacto si falso | Severidad |
|------|--------|-----------------|-----------|
| CAP-02: CASS cubre el gap | ❓ pendiente | Si CASS cubre LLM output + Kedral integration → Backscroll innecesario | **Alta** (Go/Stop) |
| S2: Valor de subagent sessions | ❓ unknown | Si no tienen valor → excluir 1,640 archivos, reducir corpus 75% | Media |
| S3: FTS5 regression 3.51.0 | ⚠️ parcial | Si prepared statements son 8.4x más lentos → primera indexación ~2min en vez de ~15s | Baja (one-time cost) |
| S4: Scoping en 6 proyectos sin index | ⚠️ parcial | Si fallback falla → búsqueda no filtrada en esos proyectos | Baja (fallback simple) |

### Conclusión provisional

**Go condicional a CAP-02**: La evidencia lógica y empírica soporta H1 excepto por la incertidumbre de CAP-02 (vs CASS). El camino crítico es:

1. **Evaluar CASS** (CAP-02): instalar, probar con corpus real, verificar si produce output útil para LLM
2. Si CASS no cubre → **Go**: construir Backscroll v1 (sessions only)
3. Si CASS cubre parcialmente → **Pivot**: wrapper sobre CASS para LLM output + Kedral
4. Si CASS cubre todo → **Stop**: adoptar CASS

---

## Fase 4: Argumento → Factibilidad

### Restricciones como axiomas

Referencian C1-C7 y CD-01 a CD-04. No se duplican.

### Claims técnicos

| ID | Claim | CAP | Spike necesario | Resultado | Estado |
|----|-------|-----|----------------|-----------|--------|
| T-01 | `modernc.org/sqlite` crea tabla FTS5 con snippet+bm25 | CAP-01 | `go test` con FTS5 CREATE + INSERT + MATCH + snippet | — | Pendiente |
| T-02 | Parser JSONL maneja string y array content sin crash en 551 archivos principales | CAP-03 | Script que parsea todos los JSONL y reporta errores | — | Pendiente |
| T-03 | stat() de 2,191 archivos completa en <10ms | CAP-05 | Benchmark con `time` | — | Pendiente |
| T-04 | Slug derivado de path coincide con projectPath en sessions-index.json | CAP-04 | Comparar derivación vs entries reales en los 5 índices | — | Pendiente |
| T-05 | CASS v0.1.53 no produce output consumible por LLM desde CLI | CAP-02 | `cass search "keyword" --format text` o equivalente | — | Pendiente |

### Matriz riesgos vs mitigaciones

| Riesgo | Premisa frágil | Impacto | Mitigación |
|--------|---------------|---------|-----------|
| CASS ya resuelve el problema | CAP-02 | Alto: Backscroll innecesario | Evaluar antes de implementar (R1) |
| Formato JSONL cambia en update de Claude Code | S1 | Medio: parser se rompe | Parseo defensivo + monitorear releases |
| modernc.org regression en prepared statements | S3 | Bajo: primera indexación lenta | One-time cost; alternativa CGO disponible |
| 6 proyectos sin sessions-index.json | S4 | Bajo: scoping falla | Fallback: derivar slug del path |

### Regla Go/No-Go

**Go** si y solo si:
1. CAP-02 ≠ Stop (CASS no cubre >=2 de: LLM output, plan indexing v2, Kedral integration)
2. T-01 pasa (FTS5 funciona en modernc)
3. T-02 pasa (>=95% parse rate)

---

## Fase 5: Factibilidad → Prototipo

### Teorema de valor

El prototipo demuestra que: dado un corpus de ~500 sesiones JSONL (~180 MB para un proyecto), Backscroll indexa incrementalmente y retorna resultados full-text con snippet en <1s, produciendo output directamente consumible por un LLM.

El prototipo NO demuestra: superioridad sobre CASS (requiere CAP-02), ni viabilidad de plan indexing (v2).

### Especificación mínima (v1 sessions-only)

| Aspecto | Especificación |
|---------|---------------|
| Input | `~/.claude/projects/<slug>/*.jsonl` (excluir agent-* por defecto, flag `--include-agents`) |
| Output | Líneas formateadas: `[SESSION] fecha · slug` + snippet con `>>>match<<<` |
| Modos | `backscroll` (list), `backscroll KEYWORD` (search), `backscroll --read ID` (read), `backscroll --stats` (stats), `backscroll --topics` (topics) |
| Index | SQLite FTS5 tabla única `search_items(source, source_path, slug, role, text, timestamp)` + `search_fts USING fts5(text)` |
| Sync | mtime-based incremental; DELETE+reinsert por archivo |
| Scope | git root → project slug → filter por `source_path LIKE ?` |
| DB location | `~/.claude/backscroll.db` |

### Instrumentación y métricas

| Métrica | Cómo se mide | Umbral éxito | Umbral fallo |
|---------|-------------|-------------|-------------|
| Primera indexación (proyecto activo) | `time backscroll --stats` primera vez | <15s | >60s |
| Sync incremental | `time backscroll --stats` segunda vez | <50ms | >500ms |
| Búsqueda FTS5 | `time backscroll "keyword"` | <100ms | >1s |
| Parse rate | Archivos parseados sin error / total | >=95% | <90% |
| Index size | `ls -la ~/.claude/backscroll.db` | <100 MB | >500 MB |

### Reporte de resultados

| Claim | Validado | Refutado | Observaciones | Siguiente paso |
|-------|----------|----------|--------------|---------------|
| T-01 FTS5 en modernc | — | — | — | Spike |
| T-02 Parser robustez | — | — | — | Spike |
| T-03 stat <10ms | — | — | — | Spike |
| T-04 Scoping correcto | — | — | — | Spike |
| T-05 CASS no cubre gap | — | — | — | Instalar y evaluar |

---

## Pieza transversal: Matriz de trazabilidad

| CAP | Claim | Fase actual | Método | Resultado | Confianza | Decisión |
|-----|-------|-------------|--------|-----------|-----------|----------|
| CAP-01 | FTS5 funciona en modernc para volumen requerido | Fase 3 | Empírico | ⚠️ parcial (docs confirman, no spike) | Media | Go condicional a spike T-01 |
| CAP-02 | Backscroll aporta valor vs CASS | Fase 3 | Empírico | ❓ pendiente evaluación | — | **Bloqueante**: evaluar antes de implementar |
| CAP-03 | Parser maneja formatos actuales | Fase 3 | Empírico | ❓ pendiente spike T-02 | — | Go condicional a spike |
| CAP-04 | Scoping funciona en 11 proyectos | Fase 3 | Empírico | ⚠️ 5/11 con index, 6 requieren fallback | Media | Go con fallback CD-02 |
| CAP-05 | Sync incremental <50ms | Fase 3 | Empírico | ⚠️ lógicamente viable, no medido | Media | Go condicional a spike T-03 |
| CAP-06 | Output consumible por LLM | Fase 3 | Lógico | ✅ snippet+metadata es patrón probado | Alta | Go |

---

## Apéndice A: Datos técnicos del research original

### Formato JSONL de sesiones

No existe schema oficial. Campos estables observados:

#### Record types

| Type | Contenido | Indexar? |
|------|-----------|---------|
| `user` | Mensajes del usuario | Sí |
| `assistant` | Respuestas de Claude (código, decisiones, razonamiento) | Sí |
| `progress` | Tool execution progress | No |
| `file-history-snapshot` | File change tracking | No |
| `system` | System events | No |
| `summary` | Session summary | No (ya en sessions-index.json) |
| `queue-operation` | Queue operations | No |

#### Estructura de mensaje

```json
{
  "type": "user",
  "message": {
    "role": "user",
    "content": "string OR [{type: 'text', text: '...'}]"
  },
  "uuid": "message-uuid",
  "slug": "session-slug-human-readable",
  "timestamp": "2026-02-18T08:41:00Z",
  "version": 42
}
```

Campos recientes (late 2025 / early 2026): `isMeta`, `thinkingMetadata`, `todos`, `agentId`, `isSidechain`, `teamName`.

Bug conocido: UUID duplicado entre archivos JSONL durante session branching (#22526). Deduplicar por UUID.

#### Patrones de ruido verificados

Mensajes que deben filtrarse (verificado en 2,191 sesiones reales):

- `<local-command-caveat>Caveat:...` — hooks de comandos locales
- `<local-command-stdout></local-command-stdout>` — output vacío de hooks
- `<command...` — command XML tags
- `Caveat:` — prefijo de caveats
- `[Request interrupted` — interrupciones de usuario
- `Base directory for this skill:` — dumps de prompts de skills
- `<system-reminder>` — system reminders inyectados

### sessions-index.json

```json
{
  "version": 1,
  "entries": [{
    "sessionId": "UUID",
    "fullPath": "/absolute/path/to/session.jsonl",
    "fileMtime": 1768957737046,
    "firstPrompt": "primer mensaje del usuario",
    "summary": "Auto-generated summary",
    "messageCount": 35,
    "created": "2026-01-20T20:57:53.184Z",
    "modified": "2026-01-20T21:12:51.482Z",
    "gitBranch": "master",
    "projectPath": "/opt/homeserver/automation",
    "isSidechain": false
  }]
}
```

Disponible en 5 de 11 proyectos. Los 6 restantes no lo generan (posiblemente por falta de actividad reciente o por ser proyectos efímeros).

### Corpus medido (2026-03-03)

| Proyecto | Main JSONL | Subagent JSONL | Total | Tamaño |
|----------|-----------|----------------|-------|--------|
| homeserver-automation | 310 | 1,103 | 1,413 | 691.7 MB |
| rootline | 167 | 328 | 495 | 181.6 MB |
| incubadora | 15 | 44 | 59 | 22.6 MB |
| /opt | 30 | 52 | 82 | 17.2 MB |
| forge | 11 | 33 | 44 | 13.6 MB |
| vdc | 9 | 65 | 74 | 10.9 MB |
| terraform-provider-localops | 0 | 10 | 10 | 1.3 MB |
| nanobot | 1 | 1 | 2 | 0.8 MB |
| homeserver | 0 | 4 | 4 | 0.8 MB |
| crucible-test | 6 | 0 | 6 | 0.2 MB |
| test-claude-skills | 2 | 0 | 2 | 0.0 MB |
| **Total** | **551** | **1,640** | **2,191** | **940.7 MB** |

### Estado del arte (actualizado 2026-03-03)

| Tool | Lenguaje | Search | Index | LLM Output | Estado |
|------|----------|--------|-------|-----------|--------|
| **cc-sessions** v1.3.2 | Rust | fzf + Ctrl+S grep | sessions-index.json | No | Activo |
| **CASS** v0.1.53 | Rust/Tantivy | BM25 + semántica (MiniLM) | Tantivy segments | No (TUI) | Activo, 11 providers |
| **CTK** v2.6.0 | Python/SQLite | SQLite FTS + LLM query | SQLite | Parcial | Activo, multi-provider |
| **claude-code-log** | Python | No | Parser JSONL | No | Activo, 736 stars |
| **claude-code-sync** | Rust | No | Git smart merge | No | Activo |
| **atuin** | Rust/SQLite | SQLite FTS5, sync | SQLite | No | Referencia análoga |

### Schema SQLite propuesto (v1 sessions-only, tabla única)

```sql
PRAGMA journal_mode=WAL;

-- Metadata de archivos indexados
CREATE TABLE files (
    path       TEXT PRIMARY KEY,
    mtime      REAL NOT NULL,
    size       INTEGER NOT NULL,
    slug       TEXT,
    session_date TEXT,
    msg_count  INTEGER
);

-- Items de búsqueda (tabla única, extensible a plans en v2)
CREATE TABLE search_items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source      TEXT NOT NULL DEFAULT 'session',  -- 'session' | 'plan' (v2)
    source_path TEXT NOT NULL,                     -- FK → files.path (o plans.path en v2)
    ordinal     INTEGER NOT NULL,
    role        TEXT,                               -- 'user' | 'assistant' (NULL para plans)
    heading     TEXT,                               -- NULL para sessions, heading para plans (v2)
    text        TEXT NOT NULL,
    timestamp   TEXT
);

-- FTS5 sobre tabla única
CREATE VIRTUAL TABLE search_fts USING fts5(
    text,
    content=search_items,
    content_rowid=id
);

-- Triggers para mantener FTS sincronizado
CREATE TRIGGER search_items_ai AFTER INSERT ON search_items BEGIN
    INSERT INTO search_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER search_items_ad AFTER DELETE ON search_items BEGIN
    INSERT INTO search_fts(search_fts, rowid, text) VALUES ('delete', old.id, old.text);
END;
```

### Diseño de plan indexing (v2)

Diferido. Cuando se implemente:
- Agregar tabla `plans` (path, mtime, size, title, plan_date, section_count)
- Agregar tabla `plan_projects` (plan_path, project_path, method)
- INSERT en `search_items` con `source='plan'`, `heading` poblado, `role` NULL
- Sin cambios en `search_fts` ni en queries de búsqueda (solo filtrar por `source` si `--plans`/`--sessions`)
- Split por `##` headers: ~6 secciones promedio por archivo

### Arquitectura de archivos (v1)

```
src/backscroll/
├── main.go       # Entry point, arg parsing
├── db.go         # SQLite schema, WAL, sync, FTS5 queries
├── parser.go     # JSONL defensive parsing + noise filtering
├── scope.go      # Project detection (git root → project path)
├── reader.go     # --read mode: JSONL filtrado
├── output.go     # Compact formatter
└── topics.go     # Topic analysis
```

## Apéndice B: Referencias

- [cc-sessions](https://github.com/chronologos/cc-sessions) — Rust session picker (v1.3.2)
- [CASS](https://github.com/Dicklesworthstone/coding_agent_session_search) — Rust hybrid search (v0.1.53)
- [CTK](https://github.com/queelius/ctk) — Python/SQLite multi-provider (v2.6.0)
- [claude-code-log](https://github.com/daaain/claude-code-log) — Python export
- [claude-code-sync](https://github.com/perfectra1n/claude-code-sync) — Rust Git sync
- [atuin](https://github.com/atuinsh/atuin) — Rust/SQLite shell history
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — Pure Go SQLite con FTS5
- [Claude Code session format](https://kentgigger.com/posts/claude-code-conversation-history)
- [Claude Code JSONL analysis with DuckDB](https://liambx.com/blog/claude-code-log-analysis-with-duckdb)
- [SQLite FTS5 documentation](https://sqlite.org/fts5.html)
- [CVE-2025-7709 FTS5 tombstone overflow](https://progosling.com/en/dev-digest/sqlite-fts5-cve-2025-7709)
- [Bug: UUID duplicado en JSONL (#22526)](https://github.com/anthropics/claude-code/issues/22526)
