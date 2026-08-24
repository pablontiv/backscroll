# Recover Remediation Startup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `recover` retain exclusive startup-lock ownership while bypassing ordinary compatible-open and pre-handler sync so remediation remains reachable when normal index preparation fails.

**Architecture:** Add one explicit `startupRemediation` command class beside snapshot, metadata, and mutation classes. It follows mutation lock acquisition/timeout and handler-held lease semantics, but `runOwnedStartup` returns directly after lock acquisition without calling `prepareIndex` or `maybeAutoSync`. Recovery's existing handler remains responsible for read-only planning, atomic replacement, verification, and post-install sync.

**Tech Stack:** Go 1.26.2, Cobra, `gofrs/flock`, SQLite recovery package, stdlib testing, subprocess tests, Just

**Spec:** `docs/superpowers/specs/2026-08-24-semantic-lineage-closure-design.md`

**Dependency:** Execute after `docs/superpowers/plans/2026-08-24-semantic-lineage-closure.md`, so the ALTER-built fixture is semantically recognized by recovery adapters.

## Global Constraints

- `recover` keeps the canonical database startup lock for the complete handler.
- Remediation uses the same five-second mutation wait and busy-owner timeout as mutations.
- Remediation skips ordinary `OpenCompatible` preparation and mandatory pre-handler sync.
- Invalid CLI/config/manifest input still fails before recovery planning.
- `recover --dry-run` performs no input-data mutation.
- Apply mode keeps the lease through atomic replacement, verification, and post-install sync.
- Search, list, patterns, status, validate, config, annotate, purge, and rebuild behavior is unchanged.
- Remove the old mechanism that carries an ordinary startup failure into the recover handler.
- Do not add a new command, flag, lock file, retry loop, or persisted state.
- Follow RED → GREEN → REFACTOR; run focused tests before race and repository gates.

---

## File map

| Path | Responsibility |
|---|---|
| `cmd/backscroll/startup_commands.go` | Declare remediation class and shared lease-retaining class predicate. |
| `cmd/backscroll/startup_commands_test.go` | Explicit command classification and lease-release behavior. |
| `cmd/backscroll/startup_coordination.go` | Mutation-equivalent lock acquisition with remediation preparation bypass. |
| `cmd/backscroll/startup_coordination_test.go` | Immediate/busy owner bypass, timeout, and lease retention. |
| `cmd/backscroll/startup_policy.go` | Register recover as remediation and remove startup-failure pass-through exception. |
| `cmd/backscroll/startup_policy_test.go` | Validate preflight blocking and delete obsolete controlled-failure path tests. |
| `cmd/backscroll/recover.go` | Consume successful remediation startup only; retain post-install sync. |
| `cmd/backscroll/recover_test.go` | Handler error and lease release without startup-failure aggregation. |
| `cmd/backscroll/compat_diagnostics_test.go` | Real ALTER-built dry-run reaches planner and preserves SQLite files. |
| `cmd/backscroll/startup_coordination_process_test.go` | Cross-process remediation contention and lock retention. |
| `docs/sync.md` | Document remediation owner path. |
| `CLAUDE.md` | Update startup command classes and recover flow. |

### Task 1: Add an explicit lease-retaining remediation class

**Files:**
- Modify: `cmd/backscroll/startup_commands.go:5-43`
- Modify: `cmd/backscroll/startup_commands_test.go:10-128`
- Modify: `cmd/backscroll/startup_coordination.go:140-153`
- Modify: `cmd/backscroll/startup_policy.go:180-224`
- Modify: `cmd/backscroll/startup_policy_test.go:93-145`

**Interfaces:**
- Consumes: existing Cobra annotations and `startupResult.release`.
- Produces:

```go
const startupRemediation startupCommandClass = "remediation"
func startupClassRetainsLease(startupCommandClass) bool
```

- [ ] **Step 1: Write the command-class RED test**

Change `TestEveryOperationalCommandHasApprovedStartupClass` expectations:

```go
"recover": startupRemediation,
```

Change `TestEveryOperationalCommandRunsStartupBeforeHandler` similarly:

```go
{argv: []string{"recover", "--from", "missing.db", "--dry-run"}, wantClass: startupRemediation},
```

Run:

```bash
go test ./cmd/backscroll -run '^(TestEveryOperationalCommandHasApprovedStartupClass|TestEveryOperationalCommandRunsStartupBeforeHandler)$'
```

Expected: FAIL to compile with `undefined: startupRemediation`.

- [ ] **Step 2: Add remediation to the explicit class set**

In `startup_commands.go`:

```go
const (
    startupSnapshotRead startupCommandClass = "snapshot-read"
    startupMetadataRead startupCommandClass = "metadata-read"
    startupMutation     startupCommandClass = "mutation"
    startupRemediation  startupCommandClass = "remediation"
    startupClassKey                         = "backscroll.io/startup-class"
)
```

Accept it in `startupCommandClassFor`:

```go
case startupSnapshotRead, startupMetadataRead, startupMutation, startupRemediation:
```

Add:

```go
func startupClassRetainsLease(class startupCommandClass) bool {
    return class == startupMutation || class == startupRemediation
}
```

Use the predicate in registration:

```go
if startupClassRetainsLease(class) {
    cmd.RunE = wrapLeaseRetainingRunE(cmd.RunE)
}
```

Rename `wrapMutationRunE` to `wrapLeaseRetainingRunE`; its body remains the same.

- [ ] **Step 3: Register recover as remediation**

In `buildRootCmdWithStartup`:

```go
registerStartupCommand(root, startupRemediation, newRecoverCmd(stdout, stderr))
```

Do not change another command's class.

- [ ] **Step 4: Preserve existing startup behavior while the new class is introduced**

In `runOwnedStartup` and `ownedStartupFailureResult`, replace `class == startupMutation` with `startupClassRetainsLease(class)`. Do not bypass prepare/sync yet.

Temporarily update the existing recover-on-recoverable-failure condition to use the same predicate:

```go
if cmd.Name() == "recover" && failure.Recoverable && startupClassRetainsLease(class) {
    return nil
}
```

Task 3 deletes this compatibility bridge after Task 2 gives remediation its direct owner path. This keeps the complete `cmd/backscroll` suite green between commits.

- [ ] **Step 5: Generalize lease-release registration tests**

Rename `TestMutationRegistrationReleasesStartupLease` to `TestLeaseRetainingRegistrationReleasesStartupLease` and table-drive both classes and handler outcomes:

```go
handlerErr := errors.New("handler failed")
for _, class := range []startupCommandClass{startupMutation, startupRemediation} {
    for _, tc := range []struct{name string; err error}{{name: "success"}, {name: "handler error", err: handlerErr}} {
        t.Run(string(class)+"/"+tc.name, func(t *testing.T) {
            lease := &fakeStartupLease{}
            cmd := &cobra.Command{Use: "operation", RunE: func(*cobra.Command, []string) error { return tc.err }}
            root := &cobra.Command{Use: "root"}
            registerStartupCommand(root, class, cmd)
            cmd.SetContext(context.WithValue(context.Background(), startupContextKey{}, startupResult{Lease: lease}))
            err := cmd.RunE(cmd, nil)
            if !errors.Is(err, tc.err) { t.Fatalf("error=%v want %v", err, tc.err) }
            if lease.releases != 1 { t.Fatalf("releases=%d want 1", lease.releases) }
        })
    }
}
```

For the release-error join test, loop over the same two classes, use `fakeStartupLease{err: releaseErr}`, and assert `errors.Is(err, handlerErr)`, `errors.Is(err, releaseErr)`, and exactly one release. Keep the read-safe no-release test unchanged.

- [ ] **Step 6: Run focused and complete command tests to GREEN**

```bash
go test ./cmd/backscroll -run '^(TestEveryOperationalCommandHasApprovedStartupClass|TestEveryOperationalCommandRunsStartupBeforeHandler|TestLeaseRetainingRegistration|TestReadSafeRegistration|TestUnknownStartupCommandClass)'
go test ./cmd/backscroll
```

Expected: PASS. Recovery still performs ordinary preparation at this intermediate commit; only its explicit class and lease semantics have changed.

- [ ] **Step 7: Commit the explicit command contract**

```bash
git add cmd/backscroll/startup_commands.go cmd/backscroll/startup_commands_test.go \
  cmd/backscroll/startup_coordination.go cmd/backscroll/startup_policy.go cmd/backscroll/startup_policy_test.go
git commit -m "refactor(cli): classify recover as remediation"
```

### Task 2: Bypass prepare and sync after remediation acquires the lock

**Files:**
- Modify: `cmd/backscroll/startup_coordination.go:31-153`
- Modify: `cmd/backscroll/startup_coordination_test.go:17-260`

**Interfaces:**
- Consumes: `startupRemediation` and `startupClassRetainsLease` from Task 1.
- Produces: owned remediation `startupResult{Config: cfg, Lease: lease}` with zero prepare/sync calls.

- [ ] **Step 1: Write immediate-owner bypass test**

Add:

```go
func TestCoordinateStartupImmediateRemediationRetainsLeaseWithoutPrepareOrSync(t *testing.T) {
    restoreStartupCoordinatorGlobals(t)
    lease := &fakeStartupLease{}
    startupTryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
    startupPrepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
        t.Fatal("remediation must not prepare the index")
        return nil, nil, nil
    }
    startupSync = func(*config.Config, io.Writer) error {
        t.Fatal("remediation must not run pre-handler sync")
        return nil
    }

    cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}
    result := coordinateStartup(context.Background(), cfg, io.Discard, startupRemediation)
    if result.Failure != nil { t.Fatalf("failure=%+v", result.Failure) }
    if result.Config != cfg || result.Lease != lease { t.Fatalf("result=%+v want cfg and retained lease", result) }
    if lease.releases != 0 { t.Fatalf("lease releases=%d want 0", lease.releases) }
}
```

- [ ] **Step 2: Write busy-owner remediation test**

Add:

```go
func TestCoordinateStartupBusyRemediationAcquiresAndBypassesPrepareSync(t *testing.T) {
    restoreStartupCoordinatorGlobals(t)
    lease := &fakeStartupLease{}
    startupTryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
    startupAcquire = func(ctx context.Context, _ string, delay time.Duration) (startupLease, error) {
        if delay != startupLockRetry { t.Fatalf("delay=%v", delay) }
        if _, ok := ctx.Deadline(); !ok { t.Fatal("missing deadline") }
        return lease, nil
    }
    startupPrepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
        t.Fatal("remediation must not prepare")
        return nil, nil, nil
    }
    startupSync = func(*config.Config, io.Writer) error { t.Fatal("remediation must not sync"); return nil }
    result := coordinateStartup(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupRemediation)
    if result.Failure != nil || result.Lease != lease { t.Fatalf("result=%+v", result) }
}
```

Run:

```bash
go test ./cmd/backscroll -run '^TestCoordinateStartup.*Remediation'
```

Expected: both tests FAIL because `runOwnedStartup` prepares and syncs; the busy path also changes the class to `startupMutation`.

- [ ] **Step 3: Preserve the requested class after waiting**

In `coordinateStartup`, change:

```go
return runOwnedStartup(ctx, cfg, progress, startupMutation, lease)
```

to:

```go
return runOwnedStartup(ctx, cfg, progress, class, lease)
```

The default switch branch continues to provide mutation/remediation wait semantics. Snapshot and metadata branches remain unchanged.

- [ ] **Step 4: Return remediation ownership before ordinary preparation**

At the beginning of `runOwnedStartup`, before diagnostics timing or `startupPrepareIndex`:

```go
if class == startupRemediation {
    return startupResult{Config: cfg, Lease: lease}
}
```

Task 1 already generalized the normal post-sync and failure retention paths through `startupClassRetainsLease`; do not add a second class check.

- [ ] **Step 5: Table-drive contention timeout for mutation and remediation**

Change the busy deadline test to run:

```go
for _, class := range []startupCommandClass{startupMutation, startupRemediation} {
    t.Run(string(class), func(t *testing.T) {
        restoreStartupCoordinatorGlobals(t)
        startupMutationWait = time.Millisecond
        startupTryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
        startupAcquire = func(ctx context.Context, _ string, _ time.Duration) (startupLease, error) { <-ctx.Done(); return nil, ctx.Err() }
        startupPrepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) { t.Fatal("prepare after timeout"); return nil, nil, nil }
        startupSync = func(*config.Config, io.Writer) error { t.Fatal("sync after timeout"); return nil }
        result := coordinateStartup(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, class)
        failure := result.startupFailure()
        if failure == nil || failure.Diagnostic.Code != compat.CodeSyncInProgress { t.Fatalf("failure=%+v", failure) }
        if len(failure.Diagnostic.Continuation) != 0 || result.Lease != nil { t.Fatalf("result=%+v", result) }
    })
}
```

- [ ] **Step 6: Run coordination tests**

```bash
go test ./cmd/backscroll -run '^TestCoordinateStartup' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the remediation owner path**

```bash
git add cmd/backscroll/startup_coordination.go cmd/backscroll/startup_coordination_test.go
git commit -m "fix(cli): bypass ordinary startup for recovery"
```

### Task 3: Remove startup-failure pass-through from the recover handler

**Files:**
- Modify: `cmd/backscroll/startup_policy.go:96-111,192-216`
- Modify: `cmd/backscroll/startup_policy_test.go:149-518`
- Modify: `cmd/backscroll/recover.go:30-72`
- Modify: `cmd/backscroll/recover_test.go:160-275`
- Modify: `cmd/backscroll/compat_diagnostics_test.go:540-600`

**Interfaces:**
- Consumes: successful remediation startup results from Task 2.
- Produces: recovery errors and post-install sync errors without aggregation with an ordinary startup failure.

- [ ] **Step 1: Write a policy test proving remediation does not bypass genuine startup failures**

Add:

```go
func TestRemediationCommandDoesNotIgnorePolicyFailure(t *testing.T) {
    policyErr := errors.New("configuration cannot be interpreted")
    root := buildRootCmdWithStartup(io.Discard, io.Discard, func(context.Context, io.Writer, startupCommandClass) startupResult {
        return startupResult{Failure: &startupFailure{
            Stage: startupStageConfigLoad,
            Cause: policyErr,
            Diagnostic: compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: policyErr.Error()},
        }}
    })
    root.SetArgs([]string{"recover", "--from", "stranded.db", "--dry-run"})
    err := root.Execute()
    if !errors.Is(err, policyErr) { t.Fatalf("error=%v want policy failure", err) }
}
```

This should already pass; it locks the retained preflight boundary before deleting the special case.

- [ ] **Step 2: Delete the old recover-on-failed-startup exception**

Remove from `PersistentPreRunE`:

```go
if cmd.Name() == "recover" && failure.Recoverable && startupClassRetainsLease(class) {
    return nil
}
```

Every startup failure now follows the ordinary refusal path. Real remediation never creates prepare/sync failure because it bypasses those phases.

- [ ] **Step 3: Simplify the recover handler**

Delete `optionalStartupFailureError` from `startup_policy.go`.

In `recover.go`, delete:

```go
startupFailure := optionalStartupFailureError(startup.startupFailure())
```

Return direct wrapped errors:

```go
return fmt.Errorf("load config for recovery: %w", err)
return fmt.Errorf("recovery failed: %w", err)
return fmt.Errorf("post-recovery sync: %w", err)
```

Do not join an absent startup failure. Keep backup-path and installed-path stderr reporting unchanged.

- [ ] **Step 4: Remove tests for the obsolete aggregate path**

Delete or replace tests whose sole contract is that recover continues with a failed ordinary startup result:

- `TestFailedStartupAllowsOnlyRecoverWithInjectedPolicy`
- `TestRecoverAloneContinuesAfterStartupFailure`
- `TestRecoverableStartupFailuresPermitControlledRecovery`
- `TestDiagnosticOnlyStartupFailurePlusRecoveryFailurePreservesBothCauses`
- `TestDiagnosticOnlyStartupFailurePlusPostInstallSyncFailurePreservesBothCauses`

Keep tests for invalid invocation, nonrecoverable preflight failure, normal recovery errors, backup reporting, post-install sync, and machine diagnostics.

Update recover unit tests that inject `startupResult.Failure`: inject only `Config` and expected handler failures. Assert no typed `startupFailure` is present in returned errors.

- [ ] **Step 5: Replace injected continuation success with the real default policy**

Rewrite `TestRecoveryContinuationExecutesInConfiguredSamePathContextWithEmptyWAL` as `TestRecoverDryRunBypassesPreparationForAlterBuiltLineage`.

Use existing helpers:

```go
dbPath := newFixtureIndexDB(t, "v13-development-alter-built.sql")
setIndexPolicyEnv(t, dbPath, t.TempDir())
```

Create the empty WAL and call `snapshotSQLiteFiles` as the current test does. Execute the real command:

```go
stdout, stderr, err := runCmd("recover", "--from", dbPath, "--dry-run")
if err != nil {
    t.Fatalf("recover dry-run failed: %v\nstdout=%q stderr=%q", err, stdout, stderr)
}
```

Assert:

```go
if !strings.Contains(stdout, "recovery dry run") { t.Fatalf("stdout=%q", stdout) }
for _, forbidden := range []string{"migration_failed", "unsupported_lineage", "f6a081b9", "50016 diagnostic"} {
    if strings.Contains(stdout+stderr, forbidden) { t.Fatalf("output retained %q: stdout=%q stderr=%q", forbidden, stdout, stderr) }
}
```

Retain exact database/WAL metadata immutability assertions, including:

```go
assertSQLiteFilesUnchanged(t, dbPath, before)
walAfter, err := os.Stat(walPath)
if err != nil { t.Fatalf("stat WAL after dry-run: %v", err) }
if walAfter.Size() != 0 || walAfter.Mode() != walBefore.Mode() || !walAfter.ModTime().Equal(walBefore.ModTime()) {
    t.Fatalf("empty WAL metadata changed: before=%+v after=%+v", walBefore, walAfter)
}
```

- [ ] **Step 6: Run all command tests after removing the compatibility bridge**

```bash
go test ./cmd/backscroll -run '^(TestRemediationCommandDoesNotIgnorePolicyFailure|TestInvalidOperationalCommandsSkipStartup|TestRecover|TestSuccessfulStartup|TestRecoverDryRunBypassesPreparationForAlterBuiltLineage)' -count=1
go test ./cmd/backscroll
```

Expected: PASS with a recovery dry-run report and no repeated startup diagnostic. The complete package must be green before commit.

- [ ] **Step 7: Commit removal of the circular failure channel**

```bash
git add cmd/backscroll/startup_policy.go cmd/backscroll/startup_policy_test.go \
  cmd/backscroll/recover.go cmd/backscroll/recover_test.go cmd/backscroll/compat_diagnostics_test.go
git commit -m "refactor(recovery): remove startup failure pass-through"
```

### Task 4: Prove remediation contention across processes

**Files:**
- Modify: `cmd/backscroll/startup_coordination_process_test.go`

**Interfaces:**
- Consumes: remediation startup from Tasks 1–3 and existing subprocess barriers.
- Produces: cross-process wait, timeout, retry, and zero-pre-sync evidence.

- [ ] **Step 1: Add a process test proving remediation waits and retains ownership**

Add `TestStartupCoordinationRemediationWaitsForOwner` using the existing child helpers and sync barrier—no new helper protocol is needed:

```go
dir := t.TempDir()
dbPath := filepath.Join(dir, "index.db")
seedStartupCoordinationDB(t, dbPath)
setIndexPolicyEnv(t, dbPath, t.TempDir())
counter := filepath.Join(dir, "sync-counter.txt")
ready := filepath.Join(dir, "owner-ready")
release := filepath.Join(dir, "owner-release")

owner := startCoordinationChild(t, []string{"status", "--json"},
    "BACKSCROLL_SYNC_COUNTER="+counter,
    "BACKSCROLL_SYNC_READY="+ready,
    "BACKSCROLL_SYNC_RELEASE="+release,
    "BACKSCROLL_SYNC_BLOCK=1")
waitForPath(t, ready, 10*time.Second)

blocked := startCoordinationChild(t, []string{"recover", "--from", dbPath, "--dry-run"},
    "BACKSCROLL_MUTATION_WAIT=100ms")
if err := waitForChild(t, blocked, 10*time.Second); err == nil {
    t.Fatal("busy remediation unexpectedly succeeded")
}
assertStderrContains(t, blocked, "sync_in_progress")
if strings.Contains(blocked.stdout.String(), "recovery dry run") {
    t.Fatalf("blocked remediation emitted recovery output: %q", blocked.stdout.String())
}

if err := os.WriteFile(release, []byte("release"), 0o600); err != nil { t.Fatal(err) }
requireChildSuccess(t, owner, 10*time.Second)

retry := startCoordinationChild(t, []string{"recover", "--from", dbPath, "--dry-run"})
requireChildSuccess(t, retry, 10*time.Second)
if !strings.Contains(retry.stdout.String(), "recovery dry run") {
    t.Fatalf("retry stdout=%q", retry.stdout.String())
}
assertCounterLines(t, counter, 1)
```

The final counter assertion proves remediation did not invoke pre-handler sync. Do not assert absence of the persistent sidecar; ADR 0003 requires it to remain.

- [ ] **Step 2: Run subprocess coordination repeatedly**

```bash
go test ./cmd/backscroll -run '^(TestStartupCoordination.*Remediation|TestRecoverDryRunBypassesPreparation)' -count=5
```

Expected: PASS without timing flakes.

- [ ] **Step 3: Commit cross-process remediation evidence**

```bash
git add cmd/backscroll/startup_coordination_process_test.go
git commit -m "test(recovery): prove remediation contention"
```

### Task 5: Update living guidance and run all gates

**Files:**
- Modify: `CLAUDE.md` — startup coordination and core pipeline sections.
- Modify: `docs/sync.md` — owner/follower/remediation behavior.

**Interfaces:**
- Consumes: completed remediation startup behavior.
- Produces: maintainer/user operational contract and final verification evidence.

- [ ] **Step 1: Update command classes in living documentation**

Document:

```text
snapshot-read: search, list, patterns, status, validate
metadata-read: config
mutation: annotate, purge, rebuild
remediation: recover
```

State that remediation acquires/retains the mutation-grade lock but skips ordinary compatible-open and pre-handler sync; apply runs post-install sync under the same lease.

Remove claims that `recover` first performs mandatory startup sync or proceeds by carrying a prior startup failure into its handler.

- [ ] **Step 2: Run focused tests**

```bash
go test ./cmd/backscroll -run 'TestCoordinateStartup|TestEveryOperationalCommand|TestRemediation|TestRecover|TestStartupCoordination.*Remediation' -count=1
```

Expected: PASS.

- [ ] **Step 3: Run race and package tests**

```bash
go test -race ./cmd/backscroll ./internal/recovery ./internal/startuplock
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Verify Windows lock compilation**

```bash
GOOS=windows GOARCH=amd64 go test -c ./internal/startuplock -o /tmp/startuplock-windows.test.exe
```

Expected: compile exits 0.

- [ ] **Step 5: Run repository gates**

```bash
just check
just test
just ci
```

Expected: PASS; aggregate coverage at least 85 percent.

- [ ] **Step 6: Commit documentation**

```bash
git add CLAUDE.md docs/sync.md
git commit -m "docs(recovery): document remediation startup"
```

- [ ] **Step 7: Record review evidence**

PR description must report:

- exact ALTER-built dry-run test output;
- prepare and pre-handler sync call counts of zero for remediation;
- immediate and busy-owner lease behavior;
- subprocess repetition count;
- race, Windows compile, and CI results;
- confirmation that no new flag, command, lock, or persistent state was added.
