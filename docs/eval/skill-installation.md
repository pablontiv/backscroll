# Skill installation correction (#62)

## Narrow question

Can native installation leave Claude, Agents, and OpenCode resolving the same
product-owned Backscroll skill, without losing an existing destination's contents,
and does the installed recipe retrieve a known indexed fixture using robot
`result_N_filepath` rather than the scout's incorrect `result_N_source_path` oracle?

## Basis and scope

Starting evidence: issue #62 and its isolated reproduction comment (2026-09-10).
At baseline, `.githooks/pre-push` replaced only Claude copies with `rm -rf` + `cp`;
`.githooks/post-merge` independently repeated that behavior. Binary installers do
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
| --- | --- | --- | --- |
| Claude | no | no | **no** |
| Agents | no | **yes** | yes |
| OpenCode | no | **yes** | yes |

The real dev binary returned `result_0_filepath=<temporary fixture>/recall.md`
for `installationoraclecobalt`, with no stderr. This corrects the oracle without
claiming that retrieval was defective.

`python3 tests/test_skill_install.py` then exited 1 with two failures before
production changes: native pre-push destroyed the Claude sentinel, and the
explicit preservation-safe installation interface did not yet exist. These
executable tests are committed separately in `1eb5b18` as the versioned E2E RED. Hook build
and rootline are stubbed only to isolate installation from unrelated side effects;
Git, filesystem operations, the hook itself and PoC retrieval are real.

## Fresh production implementation and GREEN

No PoC installer code was promoted into production. The production entrypoint
is `scripts/install-skills.py`; both Git hooks now leave skills unchanged. The
explicit installer verifies a clean ordinary source clone, emits a digest-bound
read-only inventory, preserves previous objects by rename, and links all three
supported destinations to the same canonical tree. Receipts precede mutation and
append-only events support restoration even after process exit mid-install.
The canonical skill itself now distinguishes robot `filepath`/`content` keys from
JSON `source_path`/`snippet`.

Validated locally on macOS:

- `just check`: passed (gofmt/Go vet).
- `just test`: passed (all Go packages).
- `just test-skills`: passed **19 tests** against a dev build, including
  the original E2E tests, all three destinations, byte/mode/link backups, repeated
  install, source updates, stale approvals, source provenance, symlink ancestors,
  drift refusal, partial rollback, process-exit recovery, append-only receipts,
  uninstall/restore and custom OpenCode config root.
- Installed-recipe E2E executes the first canonical cwd-inferred search command
  read through **each** installed link, using a real dev binary and an isolated
  synthetic Claude input with a registered project. Every target returns the exact
  fixture `result_0_filepath`; no all-projects fallback is needed.
- CI includes Linux/macOS filesystem + installed-recipe E2E without agent-runtime
  dependencies. Local success is not a claim that remote CI already passed.

## Native runtime observations and limits

`python3 tests/skill_runtime_probe.py` runs installed tools with a scrubbed HOME,
config/cache/data roots, an empty fixture repository, and a macOS sandbox that
prohibits network access and filesystem writes outside the fixture. No model
turn, real credentials, real-home replacement, or runtime installation is used.

| Surface | Observation |
| --- | --- |
| Codex 0.153.4 / Agents | Native app-server `skills/list` reports exactly one `backscroll`, enabled with user scope, resolving to `<stable-product>/.claude/skills/backscroll/SKILL.md`. |
| OpenCode 1.18.19 | Native `debug skill --pure` reports the OpenCode lexical `SKILL.md` path and corrected recipe content. The same observation holds with Claude/Agents links temporarily absent, so compatible-path discovery cannot hide a broken OpenCode entry. |
| Claude Code 2.1.267 | Native `plugin validate` succeeds on the real canonical skills parent with no errors. Validation of the installed parent explicitly warns that its symlink was **not** read. This is validation evidence, **not an observed interactive session discovery**. |

Claude's current official [discovery documentation](https://code.claude.com/docs/en/skills)
explicitly supports symlinked personal skill folders. A future authorized operator
rollout must still check `/skills` in the actual session; this task does not claim
that interactive check. Native probes are optional and version-sensitive. Codex
also warned that helper aliases are not created under a temporary directory; that
did not prevent `skills/list` from discovering the skill.

The first Claude target-validation attempt incorrectly passed the individual
skill directory (interpreted as a plugin with no manifest); correcting the command
to the canonical **parent skills directory**, per CLI help/documentation, passed.
This is an experiment invocation correction, not a product defect.

## Remaining operator work

This implements the product-owned mechanism, not the real installed-directory
rollout. Issue #62 must remain open for fresh real preimage inventory, stable-root
selection, digest-bound approval, interactive discovery, and restoration evidence
on the operator's actual destinations. Unsupported copies are not automatically
removed. No source is vendored into another skills repository. ADR0008 records the
proposed ownership decision for review; no merge, release or deployment occurred.
