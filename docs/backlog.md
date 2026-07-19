# Backlog — Pattern Discovery

Estado al 2026-07-19, tras north star (F0–F4), ciclo backfill (B1–B3), 
ciclo de calidad (Q1–Q3), y **SDD mining-signal-quality** (T1/T2/T3/T6 
implementados). Ledger detallado de ejecución en
`.superpowers/sdd/progress.md` (local); diseño en
`docs/superpowers/specs/2026-07-17-pattern-discovery-northstar-design.md`.

## Escalera de discovery — estado

| Nivel | Superficie | Estado |
|---|---|---|
| 1. Agregación eje fijo | `patterns --kind commands\|failures [--trend]` | ✅ completo |
| 2. Contenido en eje fijo | `--kind templates` (Drain v2, min-support) | ✅ con supply parcial (ver T2) |
| 3. Estructura temporal | `--kind sequences` (PrefixSpan, categorías v2) | ✅ con ruido SHELL_OTHER (ver T1) |
| 4a. Errores de proceso | `--kind corrections` (4 detectores) | ✅ sin calibrar (ver T4) |
| 4b. Loop de clasificación | `annotate` + `--pending` | ✅ construido, nunca corrido a escala |
| 4c. Taxonomía emergente | `label_enum` freeze | ❌ diseñado, bloqueado por T4/T5 |
| 5. Descubrimiento de eje | clustering semántico (F5) | ❌ diferido; infra vectorial dormida en repo |
| 6. Discovery → regla | matching formalizado de patrones validados | ❌ no diseñado |

## Tareas accionables (orden sugerido)

- **T1 — VAR= stripping en command_head** ✅ **COMPLETO**: la extracción toma `SP=/path;` como command_head; 
  strip de prefijos `VAR=` en `commandHead()` (reader) implementado + backfill de heads históricos activado 
  (extraction_version v2). Destraba la señal real de sequences (SHELL_OTHER share esperado caiga de 72%).
- **T2 — Re-minado de templates del corpus histórico** ✅ **COMPLETO**: el minado v2 (error-only) se 
  aplica automáticamente; archivos con v1 templates se re-minan incrementalmente via `StaleTemplatePaths()` 
  y `BackfillDerived()` con upsert semantics (normalization_version bump a v2, template_matches idempotent).
- **T3 — Epoch visibility** ✅ **COMPLETO**: `normalization_version` expuesto en output de `--kind templates` 
  (TemplateRow JSON/robot/text formatters actualizados) para distinguir épocas de minado. T4 calibración 
  ahora tiene metadatos de version.
- **T4 — CALIBRACIÓN (requiere humano)**: etiquetar a mano 50 candidatos de
  corrections según `docs/eval/corrections-calibration.md`; medir precisión
  por detector; ajustar confianzas. GATE de todo el nivel 4.
- **T5 — Primer loop real de clasificación**: tras T4, correr
  `--pending --batch 50` + `annotate` a escala; con las etiquetas libres,
  agrupar y congelar `label_enum` (migración nueva).
- **T6 — Fix fixture reresolve** ✅ **COMPLETO**: tests de reresolve-projects ejercitan ambas ramas 
  (decode-success + registry-match) via fixture path con marker `/.claude/projects/` y companion test 
  de upsert semantics.
- **T7 — F5 clustering** (solo si 1-4 dejan hambre): generación de embeddings
  (sidecar opt-in) sobre la infra existente (chunks/VectorSearch/RRF).
- **T8 — Nivel 6, discovery→regla**: sin diseñar; patrones validados del loop
  se formalizan como reglas vigilables. Esperar a tener patrones validados.

## Deuda menor registrada

- `--trend` limitado a commands/failures (por diseño, documentado).
- exit_code con supply casi nulo (texto Bash rara vez trae "exit code N").
- `backscroll export`/backup: DB perenne con datos irremplazables sin
  historia de respaldo (named future slice del north star).
