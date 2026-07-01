---
name: backscroll-doctor
description: "Trigger: backscroll doctor, backscroll self-diagnostic, self-audit backscroll, find backscroll bugs gaps enhancements from usage. Mine backscroll's own indexed history for bugs/gaps/enhancements, verifying each finding against source before reporting."
license: Apache-2.0
metadata:
  author: pablontiv
  version: "1.0"
---

# Backscroll Doctor

Self-audit the `backscroll` CLI by mining its OWN indexed usage history for bugs, gaps, and enhancement opportunities — then verify every finding before reporting it.

## Activation Contract

Run when asked to diagnose, self-audit, or find bugs/gaps/enhancements in backscroll inferred from real usage (not from an external spec).

## Execution Steps

1. **Preflight**: `command -v backscroll && backscroll status`. The index must be non-empty; note its size (files/messages) as the sample.
2. **Gather signal across FOUR angles.** For a large index, fan out one subagent per angle to keep context clean; for a quick check, run inline.
   - **Errors/bugs** — failed tool outputs: `assets/gather.sh errors`.
   - **Gaps/wishes** — prose friction and workarounds: `assets/gather.sh gaps`.
   - **Usage friction** — invocation patterns (pipes to jq/rg, retried flags, `--tail` then `search`): `assets/gather.sh usage`.
   - **Known backlog** — read `docs/roadmap/`, CLAUDE.md "Key Design Decisions", `git log`, and in-repo `TODO`/`FIXME`. Never re-propose already-planned or already-dropped work.
3. **Filter noise**: ignore Pi `encrypted_content`/`pi-drive:observation` blobs, `system-reminder`, `task-notification` (`gather.sh` strips these).
4. **VERIFY before reporting (mandatory — non-skippable).** A snippet is a lead, not a fact. Confirm each claim against the live tool and source:
   - Missing flag/command? Check `backscroll <cmd> --help` and actually run it.
   - Code bug? Confirm the exact `internal/**` `file:line`.
   - Discard false positives explicitly. Subagents routinely hallucinate that existing flags (e.g. `--source-path`) are missing — do not trust, verify.
5. **Cross-check** each surviving finding against the known backlog (angle 4) so nothing already planned is reported as new.

## Output Contract

Group findings: 🔴 Verified bugs · 🟡 Real gaps · 🟢 Usage-driven enhancements · ❌ Discarded false positives.
Per finding: one-line symptom · evidence (quoted snippet + `source_path`, or `file:line`) · verified-in-code (yes/no) · severity/value.
Present usage counts as approximate signals, not exact metrics. Lead with the highest-confidence, cheapest-to-fix verified bug.

## References

- `assets/gather.sh` — categorized query batches (`errors|gaps|usage|all`) over the live index.
- `docs/roadmap/`, `CLAUDE.md` — known/deferred/dropped work to exclude.
- `.claude/skills/backscroll/SKILL.md` — the base retrieval recipe (commands, flags, noise rules).
