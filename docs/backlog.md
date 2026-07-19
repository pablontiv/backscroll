# Backlog — Pattern Discovery

Estado al 2026-07-19, tras north star (F0–F4), ciclo backfill (B1–B3) y
ciclo de calidad (Q1–Q3). Ledger detallado de ejecución en
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

- **T1 — VAR= stripping en command_head** (último hallazgo del north star sin
  implementar): la extracción toma `SP=/path;` como command_head; strip de
  prefijos `VAR=` en `commandHead()` (reader) + backfill de heads históricos.
  Destraba la señal real de sequences (SHELL_OTHER hoy domina 72%).
- **T2 — Re-minado de templates del corpus histórico**: el minado v2
  (error-only) solo aplica a syncs nuevos; los archivos ya minados en v1 no
  se re-minan (predicado never-mined). Diseñar re-mine por
  normalization_version (análogo a extraction_version de B1).
- **T3 — Epoch visibility**: exponer `normalization_version` en el output de
  `--kind templates` (TemplateRow) para distinguir épocas de minado.
- **T4 — CALIBRACIÓN (requiere humano)**: etiquetar a mano 50 candidatos de
  corrections según `docs/eval/corrections-calibration.md`; medir precisión
  por detector; ajustar confianzas. GATE de todo el nivel 4.
- **T5 — Primer loop real de clasificación**: tras T4, correr
  `--pending --batch 50` + `annotate` a escala; con las etiquetas libres,
  agrupar y congelar `label_enum` (migración nueva).
- **T6 — Fix fixture reresolve**: el test de no-churn usa paths sin el marker
  `/.claude/projects/` y ejercita la rama de decode-failure, no la de
  FromRegistry (warning del panel Q2).
- **T7 — F5 clustering** (solo si 1-4 dejan hambre): generación de embeddings
  (sidecar opt-in) sobre la infra existente (chunks/VectorSearch/RRF).
- **T8 — Nivel 6, discovery→regla**: sin diseñar; patrones validados del loop
  se formalizan como reglas vigilables. Esperar a tener patrones validados.

## Deuda menor registrada

- `--trend` limitado a commands/failures (por diseño, documentado).
- exit_code con supply casi nulo (texto Bash rara vez trae "exit code N").
- `backscroll export`/backup: DB perenne con datos irremplazables sin
  historia de respaldo (named future slice del north star).
