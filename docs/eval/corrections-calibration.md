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

2. Extract 50 candidates from the live DB:
   ```bash
   backscroll patterns --kind corrections --min-confidence 0.0 --limit 50 --json > /tmp/corrections-sample.json
   ```

3. For each candidate:
   - Note `source_path`, `ordinal`, `detectors` (array), `max_confidence`.
   - Read the 3-message window (the user message + the preceding 2 messages for context).

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

5. For each candidate, read the 3-message window and judge: Is the message a genuine correction of agent behavior?
   - **True**: user explicitly says "wrong", "again", "denied", etc., or message is a rephrase seeking different action.
   - **False**: user is asking a normal follow-up, rephrasing an unrelated thought, or using correction-like language in a non-correction context (the documented "no, eso no es un bug, es esperado" case).

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
