---
estado: Completed
tipo: task
---
# T009: Fix stale backscroll skill preflight

**Contribuye a**: the backscroll skill must use valid CLI commands so it works correctly when invoked.

## Problem

BUG-1: `backscroll inputs validate` and `backscroll inputs list --json` were removed as part of O01–O03 but the skill was never updated. Running the skill's preflight fails with "unknown command 'inputs'".

Reproduced in session `f8a57559-b3c7-4469-8ea8-41dc30186d5e`.

## Criterios de Aceptación

- `command -v backscroll && backscroll status` works as preflight.
- No mention of `backscroll inputs` in the skill.
- No `cargo install` in the install instructions (Go binary, not Rust).
- The `--inputs` invocation mode is removed or replaced.
- Section 7 troubleshooting uses valid commands only.
- Both skill copies updated: repo and `~/.claude/skills/backscroll/` (pre-push hook handles sync).

## Scope

File: `.claude/skills/backscroll/SKILL.md`

Changes:
1. **Preflight** — remove `backscroll inputs validate` and `backscroll inputs list --json`; keep `command -v backscroll` and `backscroll status`
2. **Install instructions** — remove `cargo install` line (Go binary now)
3. **Section 3 table** — remove `--inputs` row
4. **Section 5** — remove `inputs list`/`inputs validate` from `--json` note
5. **Section 7** — replace `inputs` troubleshooting with valid commands: `backscroll status`, `backscroll validate`, `backscroll sync`

## Fuentes de verdad

- `.claude/skills/backscroll/SKILL.md`
