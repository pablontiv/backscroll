# Autoupdate: Mandatory, No Runtime Opt-Out

Date: 2026-07-24
Status: Approved design, pending implementation plan

## Problem

Autoupdate is a deliberate product invariant: every backscroll release reaches
users' machines on its own, and there is no legitimate reason for a user to pin.
Yet `cmd/backscroll/main.go:23` wires a runtime opt-out:

```go
u := autoupdate.New("pablontiv/backscroll", "backscroll", "BACKSCROLL_AUTOUPDATE_DISABLE")
```

That env var is a user-facing switch that turns the mandatory update off. It
contradicts the invariant, and nobody uses it for its stated purpose (pinning) —
its only real use has been suppressing the background fetch during local work,
which is the wrong tool for that job.

## Key finding: the correct behavior already exists in the library

`github.com/pablontiv/picokit@v0.5.4/autoupdate` already exempts development
builds intrinsically, with no env var involved:

- `apply.go:19` — `if u.CurrentVersion == "dev" { return }`: a dev build never
  applies a staged update (checked before any staged file is read).
- `updater.go:69` — `if currentVersion == "dev" || os.Getenv(u.EnvDisable) == "1"`:
  a dev build never fetches or stages.
- `updater.go:48` (library contract) — "if omitted or empty, no environment
  variable can disable the updater (only version==\"dev\" does)."
- `apply.go:34` — a staged update is applied only when `isNewer(newestTag, CurrentVersion)`.

A local `go build` produces `version = "dev"` (`main.go:13`); CI injects the real
semver via `-ldflags "-X main.version=..."`. So a dev build is exempt from
autoupdate **by identity**, not by opt-out. The escape hatch a developer or
auditor needs is already what a dev build is.

## Design

A subtraction, not a feature.

1. **Drop the opt-out argument.** `cmd/backscroll/main.go:23` becomes:

   ```go
   u := autoupdate.New("pablontiv/backscroll", "backscroll")
   ```

   Per the library contract, with no `envDisable` no environment variable can
   disable the updater on a released binary. The only exemption is
   `version == "dev"`, which is intrinsic and cannot be set by an end user.

2. **Rewrite the affected tests.** `cmd/backscroll/main_test.go` (~lines 994–1045)
   currently asserts that `BACKSCROLL_AUTOUPDATE_DISABLE=1` disables autoupdate.
   That capability is removed, so those assertions are replaced with the
   invariants that now hold:
   - a `dev`-version updater neither fetches/stages nor applies;
   - a released-version updater has no env-var off switch.

3. **Realign living docs and guidance** in the same change (docs-sync guard
   requires it). Living surfaces only — historical records are out of scope
   (see below):
   - `CLAUDE.md` — the Autoupdate note does not currently name the env var, so
     this is an addition, not a removal: state the dev-build exemption ("dev
     builds are exempt by identity; there is no env-var opt-out").
   - `.claude/skills/backscroll/SKILL.md:148` — validation/troubleshooting
     guidance changes from "set `BACKSCROLL_AUTOUPDATE_DISABLE=1`" to "build and
     run a dev binary (`go build`) — dev builds are exempt by identity."
   - The `autoupdate-reverts-local-builds` memory is updated to say the same:
     validate against a dev build; there is no env-var pin.

### Out of scope: historical records (must NOT be edited)

These name the env var but describe past decisions and completed work; they are
historical and are never rewritten to match the present:
`docs/roadmap/T020-bump-picokit-to-v0-4-0.md`,
`docs/roadmap/O15-picokit-integration/T002-wire-picokit-autoupdate.md`, and
`docs/superpowers/plans/2026-07-02-m1-slice-a2a3-recall-skill-evalset.md`. The
verification grep below expects these mentions to remain.

## What this removes

The env var's only non-pin use was letting a **released** binary skip its
background fetch — the cause of a multi-minute hang when running several
`patterns` invocations in a loop during an audit. After this change a released
binary can no longer be told to skip that fetch. The correct fix for that
scenario is to run a **dev build**, which skips the fetch and is fully exempt.
This matches the audit-tooling guidance already in the docs-northstar skill
("build a local dev binary"). For an ordinary user nothing is lost: the fetch is
backgrounded and mandatory by design, which is the intended behavior.

## Non-goals

- No change to picokit. The library already implements the desired semantics; a
  library change would be needed only if we later wanted a released binary to
  skip the sweep-time fetch without being a dev build, which this design
  explicitly declines in favor of "use a dev build."
- No change to the update cadence, the 10s staging wait, or the fetch mechanism.

## Verification

- `go build -o /tmp/bs ./cmd/backscroll` then run a command with the network
  unreachable/slow: no fetch attempt, no hang (dev exemption).
- Grep the tree for `BACKSCROLL_AUTOUPDATE_DISABLE`: only historical records
  (roadmap task files, the dated M1 plan) and this spec remain; no live code
  path, no living doc, and no agent skill references it.
- `just ci` green (build + scrubbed-HOME tests + coverage ≥85%).
