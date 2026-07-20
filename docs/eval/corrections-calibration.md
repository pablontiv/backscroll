# F3 Correction Detection — Calibration Procedure

> **Status:** MANDATORY before F3b. This document defines the 50-candidate hand-label procedure to measure detector precision and validate v1 thresholds.

## Objective

Before running the F3b agent-classification loop, we must establish baseline precision per detector on real correction windows from the indexed corpus. v1 confidences (lexicon 0.8, interrupt 0.5, denial 0.4, rephrase 0.6) are priors only — hand-labeled data tunes thresholds.

## Procedure

### Phase 1: Candidate Extraction (automated)

1. Run full sync on your corpus:
   ```bash
   backscroll sync
   ```

2. Extract 50 candidates from the live DB (stratified by detector, capped per session):
   ```bash
   go run scripts/calibration-extract/main.go \
     --total 50 \
     --per-detector 12 \
     --per-session 10 \
     --output ~/calibration/corrections-labeling-$(date +%Y-%m-%d).csv
   ```
   **CRITICAL**: The worksheet contains private session text. Write it **OUTSIDE the repository** (e.g., `~/calibration/` or `/tmp/`), never inside `docs/eval/`.

3. For each candidate in the CSV:
   - Note `source_path`, `ordinal`, `detectors` (array), `max_confidence`.
   - Read the labeling window (see Labeling Windows below for context definitions).

### Labeling Windows (Detection Context)

Each correction candidate requires a labeling window to establish judgment context. Window definitions vary by detector:

| Detector | Window Definition | Includes |
|----------|-------------------|----------|
| **Lexicon** | Current message only | The user message flagged by lexicon match |
| **Rephrase** | Current message only | The user message with Jaccard ≥0.6 rephrase |
| **Interrupt** | Preceding (assistant) + current (user) + following (user) | Last assistant message before interrupt; the resumed user message; user's next message (if exists) to see if correction follows |
| **Denial** | Preceding (assistant/tool) + current (user) + following (user) | The permission denial or error message; the user's response; follow-up if present |

**Window Retrieval**: The extraction tool (`scripts/calibration-extract/main.go`) populates `labeling_window_before` and `labeling_window_after` columns in the output CSV for interrupt/denial strata. For lexicon/rephrase, only the current message is required.

### Phase 2: Hand Labeling (manual)

4. Create a spreadsheet with columns:
   - **Candidate #** (1–50)
   - **Source Path** (file)
   - **Ordinal** (message index)
   - **Detectors Fired** (comma-separated)
   - **Max Confidence** (v1 prior)
   - **Context** (3-message window as text)
   - **True Correction?** (yes/no) — YOUR JUDGMENT: did the user actually correct the agent?
   - **Correction Type** (if yes): lexicon|interrupt|denial|rephrase|unknown
   - **Notes** (any ambiguity, edge case, or false-positive reason)

5. For each candidate, read the labeling window and apply **per-detector judgment criteria** (see below).

### Per-Detector Judgment Criteria

| Detector | "Correction" is TRUE when | FALSE when | Example True | Example False |
|----------|---------------------------|-----------|--------------|---------------|
| **Lexicon** | User explicitly steers toward **different action** due to agent misunderstanding. Es ej., "no, not X, do Y instead." | User expresses preference, disagreement, or heated tone without changing instruction. Example: "no, eso no es un bug, es esperado" (user says it's NOT a bug—information, not correction). | Message: "No, I need you to search for files named X, not Y" | Message: "No thanks, I prefer X to Y" |
| **Rephrase** | User re-phrased **because agent misunderstood**, and follow-up shows changed instruction or clarification of intent. | User restates same preference, asks the same question again, or polishes wording without changing substance. | Initial: "Fix auth"; Agent: "fixed it"; User rephrase: "Actually, I mean fix the JWT validation in the login endpoint" (clarification, different scope). | Initial: "Fix auth"; Agent: "fixing..."; User: "Please fix auth again" (same intent, just repeated). |
| **Denial** | Context window shows permission/access denial from agent, **followed by user pivot to alternative approach** (signals user corrected strategy). Requires manual session lookup to disambiguate. | Context shows procedural denial (e.g., "I cannot execute root commands") without user strategy change, or user message is unrelated to denial. | Assistant: "That command requires sudo, denied."; User: "OK, let me try a non-root alternative instead." | Assistant: "That requires admin permission, denied."; User: "OK" (acceptance, not correction). |
| **Interrupt** | User's resumed message (after interrupt) **changes instruction or strategy** compared to the paused message. Co-occurrence measure: interruption + new direction = correction signal. | Message resumes same instruction unchanged, or interrupt flag is noise. | Paused: "Implement feature X"; Resumed: "Actually, let me try a different approach for feature Y instead." (changed scope/strategy). | Paused: "Implement feature X"; Resumed: "OK, let me implement feature X now" (same instruction, just resumed). |

### Phase 3: Analysis (automated + manual)

6. After labeling, compute per-detector precision:
   - Count: (detections where True Correction? = yes) / (total detections by that detector)
   - Example: if 40 of 50 candidates fired "lexicon", and 36 were True Corrections, precision ≈ 0.90.

7. Document findings:
   - Per-detector precision (target ≥0.7 for v1; document if lower)
   - Confidence threshold recommendations (if data suggests a different split)
   - Known false positives (patterns that are hard to disambiguate)

### Phase 4: Threshold Freeze (manual decision)

8. **Before F3b launch**, the team reviews the calibration report and approves:
   - Green-light thresholds (or proposes adjustments)
   - Accept documented false positives as v1 limitations
   - Lock detector version: `internal/corrections` detectors must NOT change until re-calibration

## Output Artifact

File: `docs/eval/corrections-calibration-v1.md` (or `corrections-calibration-<date>.md`)

Contents:
- 50-candidate ground-truth table (source_path, ordinal, user_judgment, correction_type)
- Per-detector precision table
- Threshold recommendations
- Known limitations and edge cases
- Sign-off: date, labeler name, approve/reject F3b launch

## Constraints

- **Labeling must be done by a human** (the user, a domain expert, or domain-knowledgeable reviewer — not the LLM).
- **One-time, pre-loop**: if detectors change significantly, re-run calibration on a fresh 50-candidate sample.
- **Ground truth is authoritative**: the calibration table is committed to git for audit and future reference.

## Timeline

- Target: 2–3 hours for labeling + analysis
- Blocker for F3b: must complete before agent loop runs
