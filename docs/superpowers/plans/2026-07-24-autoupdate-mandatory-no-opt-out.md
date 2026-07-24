# Autoupdate Mandatory, No Opt-Out — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `BACKSCROLL_AUTOUPDATE_DISABLE` runtime opt-out so autoupdate is mandatory for released binaries, while dev builds stay exempt by identity.

**Architecture:** Delete the third argument from the `autoupdate.New(...)` call so no env var can disable a released binary (picokit contract: only `version=="dev"` exempts). Replace the tests that asserted the opt-out with a hermetic wiring test. Realign the two live scripts and living docs. Touch no historical records.

**Tech Stack:** Go (stdlib `testing`), `github.com/pablontiv/picokit/autoupdate` v0.5.4, bash scripts.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-24-autoupdate-mandatory-no-opt-out-design.md`.
- No change to `picokit`. The library already implements dev-build exemption.
- Verification is hermetic: no network dependence. `just ci` (build + scrubbed-`HOME` tests + coverage ≥85%) is the release-blocking gate and runs offline.
- Living surfaces only. **Never edit** these historical records that mention the env var: `docs/roadmap/T020-bump-picokit-to-v0-4-0.md`, `docs/roadmap/O15-picokit-integration/T002-wire-picokit-autoupdate.md`, `docs/superpowers/plans/2026-07-02-m1-slice-a2a3-recall-skill-evalset.md`.
- Conventional commits; no AI attribution. The pre-push hook runs `just ci` when any `*.go` changes and requires a same-push docs update — so all tasks land in one push.

---

### Task 1: Remove the env-var opt-out from the call site and tests

**Files:**
- Modify: `cmd/backscroll/main.go:23`
- Modify: `cmd/backscroll/main_test.go:2-15` (imports), `:989-1058` (the four autoupdate tests)

**Interfaces:**
- Consumes: `autoupdate.New(repo, binary string, envDisable ...string) *Updater` and the exported field `Updater.EnvDisable string` (picokit v0.5.4).
- Produces: nothing later tasks import.

- [ ] **Step 1: Add the hermetic wiring test (failing)**

Add the `autoupdate` import to the import block in `cmd/backscroll/main_test.go` (after the `hashfile` line):

```go
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/pablontiv/picokit/autoupdate"
	"github.com/pablontiv/picokit/hashfile"
	_ "modernc.org/sqlite"
```

Then replace the body of `TestMain_AutoupdateSkipsOnEnv` (lines 1009–1022) — its premise (an env var disables autoupdate) no longer exists — with a hermetic wiring test:

```go
func TestMain_AutoupdateHasNoEnvOptOut(t *testing.T) {
	// A released binary must have no environment opt-out: the only exemption is
	// version == "dev", which an end user cannot set. Constructing the updater
	// exactly as run() does and asserting EnvDisable == "" proves this with no
	// network call.
	u := autoupdate.New("pablontiv/backscroll", "backscroll")
	if u.EnvDisable != "" {
		t.Errorf("released binary must have no env opt-out; EnvDisable = %q", u.EnvDisable)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/backscroll/ -run TestMain_AutoupdateHasNoEnvOptOut -v`
Expected: FAIL — `EnvDisable = "BACKSCROLL_AUTOUPDATE_DISABLE"` (the call site still passes the third arg).

- [ ] **Step 3: Remove the third argument at the call site**

In `cmd/backscroll/main.go:23`, change:

```go
	u := autoupdate.New("pablontiv/backscroll", "backscroll", "BACKSCROLL_AUTOUPDATE_DISABLE")
```

to:

```go
	u := autoupdate.New("pablontiv/backscroll", "backscroll")
```

- [ ] **Step 4: Strip the now-dead env-var lines from the two remaining tests**

In `TestMain_AutoupdateConstructorParams` (was line 989), delete lines 994–995 (the `Setenv`/`Unsetenv` pair) and fix the stale comment so it reads:

```go
func TestMain_AutoupdateConstructorParams(t *testing.T) {
	// Verify run() initializes the updater with the correct repo and binary by
	// exercising --version. In tests version == "dev", so the updater performs
	// no network calls (picokit dev-build exemption).
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("--version")
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	if !strings.Contains(out, "backscroll") {
		t.Errorf("expected version output, got: %s", out)
	}
}
```

In `TestMain_AutoupdateFetchRunsInGoroutine` (was line 1040), delete its `Setenv`/`Unsetenv` pair (lines 1044–1045). The dev-build exemption already keeps it network-free; leave the rest of the test unchanged.

Leave `TestMain_AutoupdateSkipsOnDevVersion` (line 1024) untouched — it already tests the real exemption and uses no env var.

- [ ] **Step 5: Run the autoupdate tests to verify they pass**

Run: `go test ./cmd/backscroll/ -run TestMain_Autoupdate -v`
Expected: PASS — `HasNoEnvOptOut`, `ConstructorParams`, `SkipsOnDevVersion`, `FetchRunsInGoroutine`. No `SkipsOnEnv` remains.

- [ ] **Step 6: Confirm no live Go reference to the env var remains**

Run: `grep -rn "BACKSCROLL_AUTOUPDATE_DISABLE" --include="*.go" .`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add cmd/backscroll/main.go cmd/backscroll/main_test.go
git commit -m "feat(autoupdate)!: remove BACKSCROLL_AUTOUPDATE_DISABLE runtime opt-out

Autoupdate is a product invariant and nobody pins. picokit already exempts dev
builds by identity (version==\"dev\"), so the env-var opt-out was a contradictory
user-facing switch. Drop the third arg to autoupdate.New; replace the opt-out
tests with a hermetic EnvDisable=='' wiring assertion.

Claude-Session: https://claude.ai/code/session_01XsmeJSWsTTZPSzfk5kgXo7"
```

---

### Task 2: Fix the two live scripts that consume the env var

**Files:**
- Modify: `scripts/test-autoupdate-smoke.sh`
- Modify: `scripts/eval.sh:35,43,151` (and a usage note near the top)

**Interfaces:**
- Consumes: nothing from Task 1 at runtime.
- Produces: nothing.

- [ ] **Step 1: Repoint the smoke test at the dev-build invariant**

Replace the whole of `scripts/test-autoupdate-smoke.sh` with:

```bash
#!/bin/bash
set -e
# A dev build (version == "dev", produced by `just build` / plain `go build`)
# is exempt from autoupdate by identity — it makes no network call. This asserts
# that exemption without any env var; the runtime opt-out no longer exists.
./backscroll --version
echo "autoupdate smoke: ok (dev build exempt)"
```

- [ ] **Step 2: Remove the dead env-var prefixes in eval.sh**

In `scripts/eval.sh`, delete the `BACKSCROLL_AUTOUPDATE_DISABLE=1 ` prefix from each of the three invocations (lines 35, 43, 151), leaving the commands otherwise identical. For example line 35 becomes:

```bash
status_json=$("$BACKSCROLL_BIN" status --json 2>/dev/null || true)
```

line 43:

```bash
robot_sample=$("$BACKSCROLL_BIN" search "test" --robot --limit 1 2>&1 | head -3 || true)
```

line 151:

```bash
  robot_output=$("$BACKSCROLL_BIN" search "$text" $flags_str --robot --fields minimal --max-tokens 2000 2>&1 || true)
```

- [ ] **Step 3: Document the dev-build requirement at the top of eval.sh**

In `scripts/eval.sh`, add one line to the usage comment block (after the `# Exit:` line near the top):

```bash
# Note: point BACKSCROLL_BIN at a dev build (`just build`); a dev build neither
# fetches nor waits on autoupdate, so the eval loop stays fast.
```

- [ ] **Step 4: Verify the smoke test passes against a dev build**

Run: `just build && bash scripts/test-autoupdate-smoke.sh`
Expected: version line, then `autoupdate smoke: ok (dev build exempt)`; command returns promptly (no 10s hang, no HTTP).

- [ ] **Step 5: Verify no live script reference to the env var remains**

Run: `grep -rn "BACKSCROLL_AUTOUPDATE_DISABLE" scripts/`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add scripts/test-autoupdate-smoke.sh scripts/eval.sh
git commit -m "chore(scripts): drop removed autoupdate opt-out; use dev build

Both scripts consumed BACKSCROLL_AUTOUPDATE_DISABLE. The smoke test now asserts
the dev-build exemption directly; eval.sh drops the dead prefixes and documents
that BACKSCROLL_BIN should be a dev build to stay fast.

Claude-Session: https://claude.ai/code/session_01XsmeJSWsTTZPSzfk5kgXo7"
```

---

### Task 3: Realign living docs and close the grep gate

**Files:**
- Modify: `CLAUDE.md:108`
- Modify: `.claude/skills/backscroll/SKILL.md:147-149`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing.

- [ ] **Step 1: State the dev-build exemption in CLAUDE.md**

CLAUDE.md line 108 does not name the env var, so this is an addition, not a removal. Change:

```
- **Autoupdate**: `picokit/autoupdate` fetches and stages the latest GitHub release in the background; `run()` waits up to 10s after the command completes so short-lived commands don't kill the download before it finishes.
```

to:

```
- **Autoupdate**: `picokit/autoupdate` fetches and stages the latest GitHub release in the background; `run()` waits up to 10s after the command completes so short-lived commands don't kill the download before it finishes. Autoupdate is mandatory: there is no runtime opt-out (`autoupdate.New` is called with no `envDisable`). Dev builds are exempt by identity — a plain `go build` yields `version="dev"`, which picokit never fetches or applies — so validate against a dev build, not an env var.
```

- [ ] **Step 2: Fix the SKILL.md troubleshooting guidance**

In `.claude/skills/backscroll/SKILL.md`, replace the "Database locked" code block (lines 147–149) that reads:

```bash
BACKSCROLL_AUTOUPDATE_DISABLE=1 backscroll status
```

with:

```bash
backscroll status
```

- [ ] **Step 3: Run the tree-wide grep gate**

Run: `grep -rn "BACKSCROLL_AUTOUPDATE_DISABLE" . --include="*.go" --include="*.md" --include="*.sh" .claude/`
Expected: matches ONLY in `docs/roadmap/T020-bump-picokit-to-v0-4-0.md`, `docs/roadmap/O15-picokit-integration/T002-wire-picokit-autoupdate.md`, `docs/superpowers/plans/2026-07-02-m1-slice-a2a3-recall-skill-evalset.md`, and `docs/superpowers/specs/2026-07-24-autoupdate-mandatory-no-opt-out-design.md`. No live code, living doc, skill, or script.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md .claude/skills/backscroll/SKILL.md
git commit -m "docs(autoupdate): state dev-build exemption; drop env var from skill

CLAUDE.md now records that autoupdate has no runtime opt-out and dev builds are
exempt by identity. The backscroll skill's troubleshooting no longer sets the
removed env var. Historical roadmap/plan records left untouched.

Claude-Session: https://claude.ai/code/session_01XsmeJSWsTTZPSzfk5kgXo7"
```

---

### Task 4: Green gate and push

**Files:** none.

- [ ] **Step 1: Run the release-blocking gate offline**

Run: `just ci`
Expected: build succeeds, all tests pass under scrubbed `HOME`, aggregate coverage ≥85%.

- [ ] **Step 2: Update the memory record**

Update the `autoupdate-reverts-local-builds` engram memory (via `mem_update`) to state: autoupdate has no runtime opt-out; validate against a dev build (`go build` → `version="dev"`, exempt by identity); there is no env-var pin. (This is a memory operation, not a repo file.)

- [ ] **Step 3: Push**

```bash
git push
```

Expected: pre-push hook runs `just ci` (green) and the docs-update check passes (living docs changed alongside `*.go`); CI computes semver from the `feat!` commit and publishes a major release.

---

## Notes for the implementer

- The `feat(autoupdate)!:` bang in Task 1 is deliberate: removing a documented env var is a user-visible breaking change and drives a major version bump per the repo's release flow.
- Do not add a replacement flag, build tag, or config key. The design explicitly declines any opt-out; the dev-build exemption is the only escape and it is intrinsic.
- If a subagent reports a test "green", diff the test body — a passing suite proves nothing if `TestMain_AutoupdateSkipsOnEnv` was left in place asserting removed behavior.
