# Skill installation correction (#62)

## Narrow question

Can native installation leave Claude, Agents, and OpenCode resolving the same
product-owned Backscroll skill, without losing an existing destination's contents,
and does the installed recipe retrieve a known indexed fixture using robot
`result_N_filepath` rather than the scout's incorrect `result_N_source_path` oracle?

## Basis and scope

Starting evidence: issue #62 and its isolated reproduction comment (2026-09-10).
The existing `.githooks/pre-push` replaces only Claude copies with `rm -rf` + `cp`;
`.githooks/post-merge` independently repeats that behavior. Binary installers do
not install skills. The product correction must not perform an operator rollout.
Only temporary HOME fixtures may be replaced during this task.

## Activation-day-1 recall

- Command: `backscroll search "skill installation Claude Agents OpenCode" --robot --fields minimal --max-tokens 2000`.
- First call: exit 0, four hits, 0.22 seconds wall time. Relevant hits included the
  previous installation scout and shared-memory product intent; another hit was
  an unrelated query-echo ADR discussion.
- Warning: `sync_in_progress`; one permitted identical retry took 0.23 seconds,
  also exit 0 with the same warning. No additional recall/status/validate calls.
- Effect: confirmed `result_N_filepath` from the actual robot output and recovered
  the previous scout context. The issue's complete body and evidence comment,
  retrieved separately through `gh-axi`, remain the implementation authority.
- Noise/limits: minimal snippets are not a full investigation report; both calls
  read committed snapshots while another sync was active. No absence claim.

## Experiment and RED/GREEN

At baseline `82f8bb8`, `go build -o .local-evidence/backscroll ./cmd/backscroll`
and `python3 tests/skill_install_poc.py .local-evidence/backscroll` succeeded:

| Native pre-push destination | Symlink | Stale recipe retained | Local-note retained |
|---|---|---|---|
| Claude | no | no | **no** |
| Agents | no | **yes** | yes |
| OpenCode | no | **yes** | yes |

The real dev binary returned `result_0_filepath=<temporary fixture>/recall.md`
for `installationoraclecobalt`, with no stderr. This corrects the oracle without
claiming that retrieval was defective.

`python3 tests/test_skill_install.py` then exited 1 with two failures before
production changes: native pre-push destroyed the Claude sentinel, and the
explicit preservation-safe installation interface did not yet exist. These
executable tests are committed separately as the versioned E2E RED. Hook build
and rootline are stubbed only to isolate installation from unrelated side effects;
Git, filesystem operations, the hook itself and PoC retrieval are real.

Implementation and final validation results follow after GREEN.
