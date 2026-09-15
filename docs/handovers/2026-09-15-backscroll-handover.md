# Backscroll stow handover — 2026-09-15

Backscroll is current through `origin/main` commit `474cfa8`; three Dependabot pull requests await reconciliation, while two enhancements and four local follow-ups remain unscheduled or queued.

## Open PRs

All currently open pull requests were opened by Dependabot on 2026-09-14 and update GitHub Actions dependencies:

- [chore(deps): bump actions/setup-python from 5 to 7](https://github.com/pablontiv/backscroll/pull/91)
- [chore(deps): bump actions/checkout from 4 to 7](https://github.com/pablontiv/backscroll/pull/92)
- [chore(deps): bump actions/setup-go from 5 to 7](https://github.com/pablontiv/backscroll/pull/93)

## Queued Dependabot reconciliation

`bs-dependabot-pr91-93-reconcile-r1` is queued for the next work turn. Verify CI is green on the exact final head of each open pull request, then merge under the standing posture or escalate if the evidence is insufficient.

## Recently merged / closed

The prior migration handover is superseded. [docs(inputs): align manifests and project guidance](https://github.com/pablontiv/backscroll/pull/69) and [fix(search): exclude query echoes from unfiltered recall](https://github.com/pablontiv/backscroll/pull/70) merged on 2026-09-10, and [the query-echo pollution issue](https://github.com/pablontiv/backscroll/issues/64) closed the same day. Nothing from that handover remains open.

The subsequent echo-exclusion hardening chain also merged:

- [Pull request 86](https://github.com/pablontiv/backscroll/pull/86)
- [Pull request 87](https://github.com/pablontiv/backscroll/pull/87)
- [Pull request 88](https://github.com/pablontiv/backscroll/pull/88)
- [Pull request 89](https://github.com/pablontiv/backscroll/pull/89)
- [fix(storage): unify direct-search echo detection behind one serialized-text chokepoint](https://github.com/pablontiv/backscroll/pull/90)

Together, these changes addressed the recurring bug class tracked by [the query-echo pollution issue](https://github.com/pablontiv/backscroll/issues/64). The current `origin/main` head is `474cfa8`, from the final pull request in that chain.

## Open issues

- [Expose Backscroll through a shared stateless MCP 2026-07-28 server](https://github.com/pablontiv/backscroll/issues/48) — enhancement; not yet scheduled.
- [Expose long-running synchronization through MCP Tasks](https://github.com/pablontiv/backscroll/issues/49) — enhancement; not yet scheduled.

## Decisions

- On 2026-09-10, the captain granted standing merge authority: a green, independently reviewed pull request may merge on its exact final head without asking again. This handover pull request is explicitly excluded and must remain a draft awaiting captain review.
- On 2026-09-10, the escalation threshold was set: only architecture or breaking-change decisions, or evidence gaps, should reach the captain.
- After the captain requested an architecture analysis, [the serialized-text chokepoint hardening](https://github.com/pablontiv/backscroll/pull/90) shipped as the recommended design. It closed the scattered-recognizer bug class for Claude, Codex, and OpenCode. Pi remains open under `bs-echo-pi-result-unmarked-r1`.

## Risks and next steps

### Risks

- `bs-echo-pi-result-unmarked-r1`: the Pi harness is the remaining one of four indexed harnesses exposed to the original pollution symptom tracked by [the query-echo pollution issue](https://github.com/pablontiv/backscroll/issues/64). A design decision on result-pairing strategy is required before implementation.
- `bs-echo-requeue-perf-index-r1`: the SQL prefilter introduced by [the serialized-text chokepoint hardening](https://github.com/pablontiv/backscroll/pull/90) has a measured approximately 21% warm-path performance regression on large indexes. A partial-index mitigation is queued as a non-gating, non-urgent follow-up.
- `bs-agents-md-backpass-extraction-r1`: a prior evaluation found a real ownership-boundary bug: the proposed skill-extraction path defaulted to this Firstmate home's skills directory instead of the project's. The item is queued pending separate authorization and must not be reused uncorrected or treated as authorized.
- `bs-agents-md-directsearch-pointer-r1`: a trivial stale-pointer documentation correction is queued locally and remains undispatched.

### Next steps

1. Reconcile the three open Dependabot pull requests under `bs-dependabot-pr91-93-reconcile-r1`, verifying CI on each exact final head before merging or escalating.
2. Continue proactive bug discovery after the dependency queue is reconciled.
