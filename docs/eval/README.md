# Backscroll Eval-Set — M1 Milestone Success Metric

## Purpose

The eval-set provides the yardstick for M1 Episodic Recall v1: **agents recover prior work unprompted, with measurable recall@5**.

This ~20-query set was mined from real indexed sessions across backscroll, rootline, and pinata projects. Each query represents a real use case: feature decisions, error investigations, tool recovery, and architectural choices that agents need to recall before starting work.

## What We Measure

**Recall@5 with Ground-Truth Matching**: percentage of eval queries returning the **correct target result** at rank ≤5.

- Target: **≥80%** after each slice in Track A, B, C.
- Metric computed by `scripts/eval.sh` — simple, reproducible, no human judgment.
- Not a required CI gate (eval runs locally or on-demand), but a standing regression check.
- **Ground-truth approach**: Each query includes an `expected_match` — a distinctive substring from the correct target's filepath or content. A query succeeds iff that substring appears in any result at rank 0–4. This prevents vacuous evaluation (BM25 always returns something; we need to verify it's the RIGHT thing).

## The Query Set

**File**: `docs/eval/queries.toml`

**Structure**: 20 queries, each with:
- `id`: stable query identifier (e.g., `q1_split_fts_decision`)
- `text`: search string ("RRF merge reciprocal rank fusion")
- `flags`: backscroll CLI flags (`--all-projects`, `--content-type tool`, etc.)
- `expected_match`: distinctive substring from the correct target (e.g., "home-shared-backscroll" from filepath, or a unique session ID). Verifies we found the RIGHT result, not just any result.
- `description`: what the query is about (feature, bug, design)
- `rationale`: why this query matters (agent use case)
- `expected_rank`: human-predicted rank where the correct result should appear

**Coverage**:
- **Decision recovery** (4): RRF, split FTS, migration v7, cwd bucketing — agents need to know if decisions are already made.
- **Error investigation** (3): SQLITE_BUSY, coverage floors, tool errors — debugging and diagnosis.
- **Feature work** (5): OpenCode reader, trigram tokenizer, declarative engine retirement, Pi reasoning, embeddings — implementation context.
- **Tool recovery** (4): go test, backscroll search invocation, command failures, project identity — execution patterns and workarounds.
- **Self-diagnostic** (1): backscroll-doctor skill — agents need to know the diagnostic surface.

## Running the Eval

### Preflight

```bash
# Confirm backscroll is installed and index is populated
backscroll status
```

### Execute

```bash
# Run full eval-set
scripts/eval.sh

# Run with verbose output (per-query results)
scripts/eval.sh --verbose

# Run only first 5 queries (quick smoke test)
scripts/eval.sh --limit 5
```

### Output

```
Backscroll Evaluation — Recall@5 Metric
========================================
Index: 1719 files, 192507 messages
Eval-set: docs/eval/queries.toml

Loaded 20 queries from eval-set

Results
=======
Queries evaluated: 20
Results found: 18
Results at rank ≤5: 16
Recall@5: 80.0%

✓ Recall@5 target met (≥80%)
```

Exit code: 0 (success) if recall@5 ≥ 80%, else 1 (gate failed).

### Interpretation

- **Recall@5 ≥ 80%**: Most queries return useful results in the top 5. Agents can rely on backscroll for recall.
- **Recall@5 60–80%**: Some queries miss the top 5; scoring or content may be improving. Check `--verbose` output for which queries fail.
- **Recall@5 < 60%**: Significant ranking issue or missing content. Investigate failures with `backscroll search --verbose` and `backscroll status`.

## Eval-Set Evolution

**After each slice (A2→B1→B2→C1→B3→C3):**
1. Run `scripts/eval.sh --verbose` and log baseline recall@5.
2. If recall drops, investigate:
   - New content added by slice (new tool calls, reasoning)? Queries may need refinement.
   - Ranking changed? Run `backscroll search <query> --robot --fields full` and inspect scores.
3. Document regressions in the PR or commit message.

**After M1 completion:**
- Grow eval-set to ~50 queries (M2 decision).
- Establish as standing regression gate (not required pre-push, but recommended).

## Query Mining Methodology

Queries were extracted from:
1. **Real indexed sessions** — backscroll, rootline, and pinata project histories.
2. **Developer friction points** — where agents asked "what did we do about X?" or "where did we solve Y?"
3. **Architecture decisions** — choices that appear in CLAUDE.md, roadmap, and git history.
4. **Error recovery** — common bugs and investigation patterns.
5. **Cross-project patterns** — behaviors that span multiple projects.

Each query was verified to return a meaningful result on the live index (as of 2026-07-02 snapshot).

## Notes

- Queries use `--all-projects` by default unless project-scoped.
- `--content-type tool` is used for execution-shaped queries (commands, errors, paths) — these hit the trigram `tool_fts` index.
- `--max-tokens 2000` is the standard budget; most queries fit comfortably.
- Tool queries with <3 character terms (e.g., "go", "ls") will not match the trigram tokenizer; these queries are excluded from the eval-set.

## References

- Spec: `docs/superpowers/specs/2026-07-02-backscroll-north-star-milestones-design.md` (Track A, A3)
- Plan: `docs/superpowers/plans/2026-07-02-m1-slice-a2a3-recall-skill-evalset.md`
- Skill: `.claude/skills/backscroll/SKILL.md`
