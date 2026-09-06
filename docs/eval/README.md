# Backscroll recall evaluation

The evaluation keeps two datasets in `docs/eval/queries.toml`:

- `legacy`: the original 20 operator-backed queries. They retain the 80% local recall@5 gate and require an already populated configured index.
- `synthetic`: nine tracked cases backed only by `docs/eval/fixtures/recall`. They establish an observational baseline without reading the operator's corpus or creating a release gate.

The synthetic cases cover `artifact-literal`, `conversational-paraphrase`, `terminology-recovery`, `query-echo`, and `budgeted-agent-output`. They include a target term after the former 200-character preview boundary, three later Backscroll query echoes, progressive query refinement, a target absent diagnostic, a deliberate command failure, an exact-path decoy, and a payload too small to carry its target.

## Run

Use the repository script so every invocation builds the current source as a `dev` binary. Dev identity prevents autoupdate during the evaluation.

```bash
# Existing operator-backed baseline
scripts/eval.sh --dataset legacy --verbose

# Self-contained, isolated baseline
scripts/eval.sh --dataset synthetic --verbose

# Quick subset; filtering by dataset happens before the limit
scripts/eval.sh --dataset synthetic --limit 3
```

Synthetic mode creates a temporary `HOME`, XDG/config root, `*.inputs.toml` manifest, and SQLite database. The manifest indexes only the tracked JSONL fixtures. The work directory is removed when the process exits. The report records the source SHA, exact fixture root, and SHA-256 of the fixture tree.

## Measurements

Each ordinary case executes two searches:

1. `--robot --fields full --max-tokens 0` records reference rank. The target must match the exact fixture filepath and, when supplied, the complete `expected_match` content. Content is not truncated before verification.
2. `--robot --fields minimal --max-tokens <case budget>` records whether that exact target identity survives in the emitted bounded payload.

Cases with `refined_text` repeat both measurements for the refined query. Stdout and stderr are captured separately, and nonzero exits are reported independently from retrieval misses. Expected-absent and expected-error cases are diagnostics and do not inflate recall denominators.

The report prints per-cohort totals and, with `--verbose`, per-case execution status, reference rank, bounded presence, refined rank, exit code, and stderr evidence.

Exit codes:

- `0`: the legacy quality gate passed, or the synthetic observational run completed without unexpected execution failures.
- `1`: legacy recall@5 was below 80%.
- `2`: invalid metadata, setup failure, or an unexpected command failure.

## Query fields

Legacy records continue to work without new fields. Synthetic records use:

- `dataset` and `cohort` for selection and reporting;
- `expected_file` for stable, exact target identity;
- `expected_match` for full-content ground truth;
- `max_tokens` for bounded robot output;
- optional `refined_text`, `expect_absent`, or `expect_error` for diagnostics.

This evaluation measures current behavior. It does not choose a ranking policy, enable embeddings, change strict search defaults, or define query-echo exclusion. Its evidence is intended to inform those later product decisions.
