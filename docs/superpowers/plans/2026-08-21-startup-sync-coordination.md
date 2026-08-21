# Startup Sync Coordination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove quadratic startup tagging and guarantee one cross-process startup synchronizer per canonical database while compatible read-safe followers query the last committed SQLite snapshot.

**Architecture:** Replace full-session text concatenation with a per-message tagging accumulator. Add a focused `internal/startuplock` package backed by `gofrs/flock`, then make Cobra's root startup policy classify commands, acquire ownership before preparation, retain ownership through mutations, and route busy read-safe commands to read-only WAL snapshots with stderr freshness warnings.

**Tech Stack:** Go 1.26.2, Cobra, modernc.org/sqlite WAL, `github.com/gofrs/flock` v0.13.0, stdlib `testing` and subprocess helpers, Rootline for ADR validation.

**Spec:** `docs/superpowers/specs/2026-08-21-startup-sync-coordination-design.md`

## Global Constraints

- Pin `github.com/gofrs/flock` at v0.13.0.
- The lock sidecar is `<canonical-db>.startup-sync.lock`, created with mode `0600`, never removed, and supported only on a trusted local filesystem.
- Read-safe snapshot commands are `search`, `list`, `patterns`, `status`, and `validate`; `config` is read-safe metadata; `annotate`, `purge`, `rebuild`, and `recover` are mutations.
- Mutation followers wait at most 5 seconds, retrying at approximately 50 ms; read-safe followers never wait.
- Successful concurrent reads warn on stderr with code `sync_in_progress`; existing text, JSON, and robot stdout shapes remain unchanged.
- Only compatible read-only snapshots may be served; a follower never creates or migrates the database.
- Mutation owners retain the lock through their handlers; `recover` retains it through replacement, verification, and post-install sync.
- No progress UI, hashing/discovery optimization, daemon, MCP server, MCP Tasks, FTS change, or schema migration.
- Tests must scrub `HOME` and `BACKSCROLL_CONFIG_DIR` and must not depend on machine-local inputs.
- Every production change follows red-green-refactor and each task ends with its own focused commit.

## File Map

### New files

- `internal/startuplock/startuplock.go` — canonical sidecar identity, safe open, immediate/context acquisition, idempotent release.
- `internal/startuplock/startuplock_test.go` — deterministic unit coverage for path identity, permissions, contention, cancellation, and cleanup.
- `internal/startuplock/process_test.go` — real subprocess contention and crash-recovery coverage.
- `cmd/backscroll/startup_commands.go` — explicit Cobra startup classes and common mutation-handler lease wrapper.
- `cmd/backscroll/startup_commands_test.go` — exhaustive command inventory and lease-wrapper tests.
- `cmd/backscroll/startup_coordination.go` — owner/follower state machine, warning values, bounded mutation acquisition.
- `cmd/backscroll/startup_coordination_test.go` — injected policy-level owner/follower tests.
- `cmd/backscroll/startup_coordination_process_test.go` — real multi-process single-flight, stale-read, timeout, and crash tests.

### Modified files

- `internal/tagging/tagging.go` — zero-value `Accumulator`, with `Tag` implemented through it.
- `internal/tagging/tagging_test.go` — accumulator equivalence and benchmark coverage.
- `cmd/backscroll/sync_helpers.go` — feed messages to the accumulator and remove `sessionText`.
- `cmd/backscroll/main_test.go` — preserve existing sync/tag assertions and add the focused no-join regression where the current fixtures live.
- `internal/compat/types.go` — add `CodeSyncInProgress`.
- `cmd/backscroll/index_policy.go` — make `indexDataRead` truly read-only and non-migrating.
- `cmd/backscroll/index_policy_test.go` — prove read-only access and refusal to migrate.
- `cmd/backscroll/startup_policy.go` — accept command class, invoke coordination, carry warnings/lease, render warnings, release on pre-handler failures.
- `cmd/backscroll/startup_policy_test.go` — update injected policy signatures and preserve recovery/error contracts.
- `go.mod`, `go.sum` — pinned `gofrs/flock` dependency.
- `docs/sync.md` — owner/follower pipeline and `sync_in_progress` behavior.
- `CLAUDE.md` — implemented package list, module/package layout, pipeline, and advisory-lock key decision.

---

### Task 1: Replace Full-Session Tagging With an Incremental Accumulator

**Files:**
- Modify: `internal/tagging/tagging.go`
- Modify: `internal/tagging/tagging_test.go`
- Modify: `cmd/backscroll/sync_helpers.go:120-151`
- Test: `cmd/backscroll/main_test.go`

**Interfaces:**
- Consumes: existing `patterns map[string]*regexp.Regexp` and `Tag(string) []string`.
- Produces: zero-value `tagging.Accumulator`; `func (a *Accumulator) Add(content string)`; `func (a *Accumulator) Tags() []string`; unchanged `func Tag(content string) []string`.

- [ ] **Step 1: Write accumulator equivalence tests before changing production code**

Add imports for `strings` and write table-driven equivalence plus zero-value tests in `internal/tagging/tagging_test.go`:

```go
func TestAccumulatorMatchesJoinedSessionTagSet(t *testing.T) {
    cases := [][]string{
        nil,
        {"BUG in parser", "Document the README"},
        {"clean", "up does not cross the newline"},
        {"Implement feature", "add tests", "configure CI"},
        {"", "Random text", "PANIC"},
    }
    for _, messages := range cases {
        var acc Accumulator
        for _, message := range messages {
            acc.Add(message)
        }
        got := acc.Tags()
        want := Tag(strings.Join(messages, "\n"))
        sort.Strings(got)
        sort.Strings(want)
        if !reflect.DeepEqual(got, want) {
            t.Fatalf("messages=%q tags=%v want=%v", messages, got, want)
        }
    }
}

func TestAccumulatorTagsReturnsIndependentSlice(t *testing.T) {
    var acc Accumulator
    acc.Add("fix bug and add tests")
    first := acc.Tags()
    first[0] = "mutated"
    second := acc.Tags()
    if slices.Contains(second, "mutated") {
        t.Fatalf("Tags returned aliased state: %v", second)
    }
}
```

Add `reflect`, `slices`, and `strings` to the test imports.

- [ ] **Step 2: Run the focused test and verify red**

Run:

```bash
go test ./internal/tagging -run 'TestAccumulator' -count=1
```

Expected: compilation fails because `Accumulator` is undefined.

- [ ] **Step 3: Implement the minimal accumulator and route `Tag` through it**

Replace the body-level matching logic in `internal/tagging/tagging.go` with:

```go
type Accumulator struct {
    matched map[string]struct{}
}

func (a *Accumulator) Add(content string) {
    lowerContent := strings.ToLower(content)
    for category, pattern := range patterns {
        if pattern.MatchString(lowerContent) {
            if a.matched == nil {
                a.matched = make(map[string]struct{}, len(patterns))
            }
            a.matched[category] = struct{}{}
        }
    }
}

func (a *Accumulator) Tags() []string {
    tags := make([]string, 0, len(a.matched))
    for category := range a.matched {
        tags = append(tags, category)
    }
    return tags
}

func Tag(content string) []string {
    var accumulator Accumulator
    accumulator.Add(content)
    return accumulator.Tags()
}
```

Retain the exported `Tag` comment and add GoDoc to `Accumulator`, `Add`, and `Tags`.

- [ ] **Step 4: Remove quadratic concatenation from `maybeAutoSync`**

In `cmd/backscroll/sync_helpers.go`, replace `var sessionText string` with `var sessionTags tagging.Accumulator`; inside the message loop call:

```go
sessionTags.Add(msg.Content)
```

Delete:

```go
sessionText += msg.Content + "\n"
sessionTags := tagging.Tag(sessionText)
```

and store:

```go
Tags: sessionTags.Tags(),
```

Do not add a `strings.Builder`; the session-sized duplicate must disappear entirely.

- [ ] **Step 5: Add benchmark evidence**

Add to `internal/tagging/tagging_test.go`:

```go
func BenchmarkAccumulatorScaling(b *testing.B) {
    base := strings.Repeat("ordinary prose with a final bug marker ", 256)
    for _, records := range []int{128, 256, 512} {
        b.Run(fmt.Sprintf("records_%d", records), func(b *testing.B) {
            messages := make([]string, records)
            for i := range messages {
                messages[i] = base
            }
            b.ReportAllocs()
            b.SetBytes(int64(records * len(base)))
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                var acc Accumulator
                for _, message := range messages {
                    acc.Add(message)
                }
                _ = acc.Tags()
            }
        })
    }
}
```

Add `fmt` to imports. Run:

```bash
go test ./internal/tagging ./cmd/backscroll -run 'Test(Tag|Accumulator)|Test.*AutoSync' -count=1
go test ./internal/tagging -run '^$' -bench BenchmarkAccumulatorScaling -benchmem -count=3
```

Expected: tests pass; benchmark bytes/sec remains in the same order of magnitude and `B/op` does not grow quadratically when records double. Save the three benchmark outputs in the eventual PR description, not in a tracked generated file.

- [ ] **Step 6: Commit the independently working linear-tagging slice**

```bash
git add internal/tagging/tagging.go internal/tagging/tagging_test.go cmd/backscroll/sync_helpers.go cmd/backscroll/main_test.go
git commit -m "perf(sync): make startup tagging linear"
```

---

### Task 2: Add the Portable Advisory Lock Primitive

**Files:**
- Create: `internal/startuplock/startuplock.go`
- Create: `internal/startuplock/startuplock_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: `github.com/gofrs/flock` v0.13.0.
- Produces: `type Lease`; `func TryAcquire(databasePath string) (*Lease, bool, error)`; `func Acquire(ctx context.Context, databasePath string, retryDelay time.Duration) (*Lease, error)`; `func (l *Lease) Release() error`; sentinel `ErrUnsafeSidecar`.

- [ ] **Step 1: Pin the dependency**

Run:

```bash
go get github.com/gofrs/flock@v0.13.0
go mod tidy
```

Verify:

```bash
go list -m github.com/gofrs/flock
```

Expected: exactly `github.com/gofrs/flock v0.13.0`.

- [ ] **Step 2: Write failing path, permission, and contention tests**

Create `internal/startuplock/startuplock_test.go` in package `startuplock` with focused tests using two independent lock objects:

```go
func TestTryAcquireCanonicalAliasesContend(t *testing.T) {
    dir := t.TempDir()
    realDB := filepath.Join(dir, "index.db")
    if err := os.WriteFile(realDB, nil, 0o600); err != nil {
        t.Fatal(err)
    }
    alias := filepath.Join(dir, "alias.db")
    if err := os.Symlink(realDB, alias); err != nil {
        t.Fatal(err)
    }

    first, locked, err := TryAcquire(realDB)
    if err != nil || !locked {
        t.Fatalf("first acquire locked=%v err=%v", locked, err)
    }
    defer first.Release()

    second, locked, err := TryAcquire(alias)
    if err != nil {
        t.Fatalf("alias acquire: %v", err)
    }
    if locked || second != nil {
        t.Fatalf("alias bypassed canonical lock: lease=%v locked=%v", second, locked)
    }
}

func TestSidecarCreatedPrivateAndPersists(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "index.db")
    lease, locked, err := TryAcquire(dbPath)
    if err != nil || !locked {
        t.Fatalf("acquire locked=%v err=%v", locked, err)
    }
    path, err := sidecarPath(dbPath)
    if err != nil {
        t.Fatal(err)
    }
    info, err := os.Stat(path)
    if err != nil {
        t.Fatal(err)
    }
    if got := info.Mode().Perm(); got != 0o600 {
        t.Fatalf("permissions=%#o want 0600", got)
    }
    if err := lease.Release(); err != nil {
        t.Fatal(err)
    }
    if _, err := os.Stat(path); err != nil {
        t.Fatalf("sidecar removed after release: %v", err)
    }
}
```

Add these exact tests in the same file:

```go
func TestTryAcquireRejectsUnsafeSidecars(t *testing.T) {
    for _, kind := range []string{"directory", "symlink"} {
        t.Run(kind, func(t *testing.T) {
            dir := t.TempDir()
            dbPath := filepath.Join(dir, "index.db")
            path := dbPath + ".startup-sync.lock"
            if kind == "directory" {
                if err := os.Mkdir(path, 0o700); err != nil { t.Fatal(err) }
            } else {
                target := filepath.Join(dir, "target")
                if err := os.WriteFile(target, nil, 0o600); err != nil { t.Fatal(err) }
                if err := os.Symlink(target, path); err != nil { t.Fatal(err) }
            }
            _, _, err := TryAcquire(dbPath)
            if !errors.Is(err, ErrUnsafeSidecar) {
                t.Fatalf("error=%v want ErrUnsafeSidecar", err)
            }
        })
    }
}

func TestAcquireHonorsCanceledContext(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "index.db")
    owner, locked, err := TryAcquire(dbPath)
    if err != nil || !locked { t.Fatalf("owner locked=%v err=%v", locked, err) }
    defer owner.Release()
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    _, err = Acquire(ctx, dbPath, time.Millisecond)
    if !errors.Is(err, context.Canceled) { t.Fatalf("error=%v", err) }
}

func TestLeaseReleaseIsIdempotent(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "index.db")
    lease, locked, err := TryAcquire(dbPath)
    if err != nil || !locked { t.Fatalf("locked=%v err=%v", locked, err) }
    if err := lease.Release(); err != nil { t.Fatal(err) }
    if err := lease.Release(); err != nil { t.Fatal(err) }
    next, locked, err := TryAcquire(dbPath)
    if err != nil || !locked { t.Fatalf("reacquire locked=%v err=%v", locked, err) }
    defer next.Release()
}
```

Add a separate missing-parent test that expects an error containing `resolve database parent` and verifies no directory was created.

- [ ] **Step 3: Run tests and verify red**

```bash
go test ./internal/startuplock -count=1
```

Expected: compilation fails because the package implementation does not exist.

- [ ] **Step 4: Implement canonicalization and safe sidecar validation**

Create `internal/startuplock/startuplock.go` with this structure:

```go
package startuplock

import (
    "context"
    "errors"
    "fmt"
    "io/fs"
    "os"
    "path/filepath"
    "sync"
    "time"

    "github.com/gofrs/flock"
)

var ErrUnsafeSidecar = errors.New("unsafe startup lock sidecar")

type Lease struct {
    mu       sync.Mutex
    lock     *flock.Flock
    released bool
}

func canonicalDatabasePath(path string) (string, error) {
    abs, err := filepath.Abs(path)
    if err != nil {
        return "", fmt.Errorf("canonicalize database path %s: %w", path, err)
    }
    if _, err := os.Stat(abs); err == nil {
        resolved, err := filepath.EvalSymlinks(abs)
        if err != nil {
            return "", fmt.Errorf("resolve database path %s: %w", abs, err)
        }
        return resolved, nil
    } else if !errors.Is(err, fs.ErrNotExist) {
        return "", fmt.Errorf("stat database path %s: %w", abs, err)
    }
    parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
    if err != nil {
        return "", fmt.Errorf("resolve database parent %s: %w", filepath.Dir(abs), err)
    }
    return filepath.Join(parent, filepath.Base(abs)), nil
}

func sidecarPath(databasePath string) (string, error) {
    canonical, err := canonicalDatabasePath(databasePath)
    if err != nil {
        return "", err
    }
    return canonical + ".startup-sync.lock", nil
}

func validateExistingSidecar(path string) error {
    info, err := os.Lstat(path)
    if errors.Is(err, fs.ErrNotExist) {
        return nil
    }
    if err != nil {
        return fmt.Errorf("inspect startup lock %s: %w", path, err)
    }
    if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
        return fmt.Errorf("%w: %s is not a regular file", ErrUnsafeSidecar, path)
    }
    return nil
}
```

The trusted-parent assumption from the spec bounds the unavoidable check/open race. Do not delete the sidecar anywhere.

- [ ] **Step 5: Implement immediate/context acquisition and idempotent release**

Use one helper to build the configured flock:

```go
func newFileLock(databasePath string) (*flock.Flock, error) {
    path, err := sidecarPath(databasePath)
    if err != nil {
        return nil, err
    }
    if err := validateExistingSidecar(path); err != nil {
        return nil, err
    }
    return flock.New(path, flock.SetPermissions(0o600)), nil
}

func TryAcquire(databasePath string) (*Lease, bool, error) {
    fileLock, err := newFileLock(databasePath)
    if err != nil {
        return nil, false, err
    }
    locked, err := fileLock.TryLock()
    if err != nil {
        return nil, false, errors.Join(err, fileLock.Close())
    }
    if !locked {
        return nil, false, fileLock.Close()
    }
    return &Lease{lock: fileLock}, true, nil
}

func Acquire(ctx context.Context, databasePath string, retryDelay time.Duration) (*Lease, error) {
    if retryDelay <= 0 {
        return nil, fmt.Errorf("startup lock retry delay must be positive")
    }
    fileLock, err := newFileLock(databasePath)
    if err != nil {
        return nil, err
    }
    locked, err := fileLock.TryLockContext(ctx, retryDelay)
    if err != nil {
        return nil, errors.Join(err, fileLock.Close())
    }
    if !locked {
        return nil, errors.Join(ctx.Err(), fileLock.Close())
    }
    return &Lease{lock: fileLock}, nil
}

func (l *Lease) Release() error {
    if l == nil {
        return nil
    }
    l.mu.Lock()
    defer l.mu.Unlock()
    if l.released {
        return nil
    }
    if err := l.lock.Close(); err != nil {
        return err
    }
    l.released = true
    return nil
}
```

The pinned v0.13.0 implementation closes its unused handle after failed nonblocking acquisition; the wrapper still calls `Close` on every error/contended return and tests successful reacquisition to prove no handle remains.

- [ ] **Step 6: Run focused tests and audit the module change**

```bash
gofmt -w internal/startuplock
go test ./internal/startuplock -count=1
go mod verify
git diff --check
```

Expected: all pass.

- [ ] **Step 7: Commit the lock primitive**

```bash
git add internal/startuplock/startuplock.go internal/startuplock/startuplock_test.go go.mod go.sum
git commit -m "feat(sync): add cross-process startup lock"
```

---

### Task 3: Prove Lock Contention and Crash Recovery With Real Processes

**Files:**
- Create: `internal/startuplock/process_test.go`

**Interfaces:**
- Consumes: Task 2 `TryAcquire`, `Lease.Release`.
- Produces: subprocess helper contract `BACKSCROLL_LOCK_HELPER=hold`; regression proof that process death releases ownership.

- [ ] **Step 1: Write the subprocess helper and crash test**

Create `internal/startuplock/process_test.go`:

```go
func TestLockHelperProcess(t *testing.T) {
    if os.Getenv("BACKSCROLL_LOCK_HELPER") != "hold" {
        return
    }
    dbPath := os.Getenv("BACKSCROLL_LOCK_DB")
    lease, locked, err := TryAcquire(dbPath)
    if err != nil || !locked {
        fmt.Fprintf(os.Stderr, "acquire locked=%v err=%v\n", locked, err)
        os.Exit(2)
    }
    fmt.Fprintln(os.Stdout, "locked")
    _ = os.Stdout.Sync()
    select {}
    runtime.KeepAlive(lease)
}

func TestProcessDeathReleasesLock(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "index.db")
    cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$", "-test.v")
    cmd.Env = append(os.Environ(),
        "BACKSCROLL_LOCK_HELPER=hold",
        "BACKSCROLL_LOCK_DB="+dbPath,
    )
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        t.Fatal(err)
    }
    cmd.Stderr = os.Stderr
    if err := cmd.Start(); err != nil {
        t.Fatal(err)
    }
    reader := bufio.NewReader(stdout)
    line, err := reader.ReadString('\n')
    if err != nil || strings.TrimSpace(line) != "locked" {
        t.Fatalf("helper readiness line=%q err=%v", line, err)
    }

    if lease, locked, err := TryAcquire(dbPath); err != nil || locked || lease != nil {
        t.Fatalf("parent bypassed helper lock: lease=%v locked=%v err=%v", lease, locked, err)
    }
    if err := cmd.Process.Kill(); err != nil {
        t.Fatal(err)
    }
    _ = cmd.Wait()

    deadline := time.Now().Add(5 * time.Second)
    for {
        lease, locked, err := TryAcquire(dbPath)
        if err != nil {
            t.Fatal(err)
        }
        if locked {
            if err := lease.Release(); err != nil {
                t.Fatal(err)
            }
            break
        }
        if time.Now().After(deadline) {
            t.Fatal("lock remained held after helper process death")
        }
        time.Sleep(25 * time.Millisecond)
    }
}
```

Import `bufio`, `fmt`, `os`, `os/exec`, `path/filepath`, `runtime`, `strings`, `testing`, and `time`.

- [ ] **Step 2: Run the crash test with the production lock implementation**

```bash
go test ./internal/startuplock -run 'Test(ProcessDeath|LockHelper)' -count=1 -v
```

Expected: PASS; the parent first observes contention, kills the helper, then reacquires before the five-second deadline.

- [ ] **Step 3: Add a second-process nonblocking contention assertion**

Extend the helper before its `hold` branch:

```go
mode := os.Getenv("BACKSCROLL_LOCK_HELPER")
if mode != "hold" && mode != "try" { return }
lease, locked, err := TryAcquire(os.Getenv("BACKSCROLL_LOCK_DB"))
if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(2) }
if mode == "try" {
    if locked { _ = lease.Release(); fmt.Fprintln(os.Stdout, "acquired") } else { fmt.Fprintln(os.Stdout, "busy") }
    return
}
```

Start the existing `hold` child, then run a second child with `BACKSCROLL_LOCK_HELPER=try` via `cmd.Output()`. Assert it exits within one second and returns exactly `busy\n`. This proves contention is cross-process rather than only between flock objects in one process.

- [ ] **Step 4: Verify native and cross-compiled package behavior**

```bash
go test ./internal/startuplock -count=5
GOOS=windows GOARCH=amd64 go test -c ./internal/startuplock -o "$(mktemp -d)/startuplock.test.exe"
```

Expected: native tests pass five times and Windows compilation succeeds without leaving a binary in the repository.

- [ ] **Step 5: Commit the crash-recovery proof**

```bash
git add internal/startuplock/process_test.go
git commit -m "test(sync): verify startup lock crash recovery"
```

---

### Task 4: Classify Commands and Add Lease Lifecycle Wrapping

**Files:**
- Create: `cmd/backscroll/startup_commands.go`
- Create: `cmd/backscroll/startup_commands_test.go`
- Modify: `cmd/backscroll/startup_policy.go`
- Modify: `cmd/backscroll/startup_policy_test.go`

**Interfaces:**
- Consumes: Task 2 `(*startuplock.Lease).Release() error` through a private `startupLease` interface.
- Produces: `startupCommandClass`; `startupSnapshotRead`, `startupMetadataRead`, `startupMutation`; `registerStartupCommand`; `startupCommandClassFor`; `startupResult.Lease`; `startupResult.release(error) error`; policy signature `func(context.Context, io.Writer, startupCommandClass) startupResult`.

- [ ] **Step 1: Write the exhaustive Cobra classification test**

Create `cmd/backscroll/startup_commands_test.go` with the approved inventory:

```go
func TestEveryOperationalCommandHasApprovedStartupClass(t *testing.T) {
    root := buildRootCmd(io.Discard, io.Discard)
    want := map[string]startupCommandClass{
        "search": startupSnapshotRead,
        "list": startupSnapshotRead,
        "patterns": startupSnapshotRead,
        "status": startupSnapshotRead,
        "validate": startupSnapshotRead,
        "config": startupMetadataRead,
        "annotate": startupMutation,
        "purge": startupMutation,
        "rebuild": startupMutation,
        "recover": startupMutation,
    }
    if len(root.Commands()) != len(want) {
        t.Fatalf("operational command count=%d want=%d", len(root.Commands()), len(want))
    }
    for _, command := range root.Commands() {
        got, explicit := startupCommandClassFor(command)
        if !explicit {
            t.Errorf("%s has no explicit startup class", command.Name())
            continue
        }
        if got != want[command.Name()] {
            t.Errorf("%s class=%q want=%q", command.Name(), got, want[command.Name()])
        }
    }
}
```

Also add a fake lease test proving a mutation wrapper releases on both success and handler error, while read-safe registration does not retain a lease wrapper.

- [ ] **Step 2: Run the new test and verify red**

```bash
go test ./cmd/backscroll -run 'TestEveryOperationalCommandHasApprovedStartupClass|TestMutation.*Lease' -count=1
```

Expected: compilation fails because startup classes are undefined.

- [ ] **Step 3: Implement classes and common registration**

Create `cmd/backscroll/startup_commands.go`:

```go
package main

import (
    "errors"

    "github.com/spf13/cobra"
)

type startupCommandClass string

const (
    startupSnapshotRead startupCommandClass = "snapshot-read"
    startupMetadataRead startupCommandClass = "metadata-read"
    startupMutation     startupCommandClass = "mutation"
    startupClassKey                         = "backscroll.io/startup-class"
)

func startupCommandClassFor(cmd *cobra.Command) (startupCommandClass, bool) {
    if cmd.Annotations == nil {
        return startupMutation, false
    }
    class := startupCommandClass(cmd.Annotations[startupClassKey])
    switch class {
    case startupSnapshotRead, startupMetadataRead, startupMutation:
        return class, true
    default:
        return startupMutation, false
    }
}

func registerStartupCommand(root *cobra.Command, class startupCommandClass, cmd *cobra.Command) {
    if cmd.Annotations == nil {
        cmd.Annotations = make(map[string]string)
    }
    cmd.Annotations[startupClassKey] = string(class)
    if class == startupMutation {
        cmd.RunE = wrapMutationRunE(cmd.RunE)
    }
    root.AddCommand(cmd)
}

func wrapMutationRunE(runE func(*cobra.Command, []string) error) func(*cobra.Command, []string) (retErr error) {
    return func(cmd *cobra.Command, args []string) (retErr error) {
        defer func() { retErr = startupResultFrom(cmd).release(retErr) }()
        return runE(cmd, args)
    }
}
```

Keep imports minimal; `errors` belongs in `startup_policy.go` if only `startupResult.release` uses it.

- [ ] **Step 4: Extend startup result and policy signature**

In `cmd/backscroll/startup_policy.go` add:

```go
type startupLease interface {
    Release() error
}

type startupWarning struct {
    Code    compat.Code
    Summary string
}

type startupResult struct {
    Config  *config.Config
    Failure *startupFailure
    Warning *startupWarning
    Lease   startupLease
}

func (r startupResult) release(retErr error) error {
    if r.Lease == nil {
        return retErr
    }
    if releaseErr := r.Lease.Release(); releaseErr != nil {
        return errors.Join(retErr, fmt.Errorf("release startup lock: %w", releaseErr))
    }
    return retErr
}
```

Change:

```go
type startupPolicyFunc func(context.Context, io.Writer, startupCommandClass) startupResult
```

and pass the explicit/default-safe class from `PersistentPreRunE` into the policy. Update all injected policy lambdas in `startup_policy_test.go`, `recover_test.go`, and `compat_diagnostics_test.go` to accept the third argument. Assertions should verify the expected class for at least one command from each category.

- [ ] **Step 5: Register every command through the common helper**

Replace the single variadic `root.AddCommand(...)` call with ten explicit calls:

```go
registerStartupCommand(root, startupSnapshotRead, newSearchCmd(stdout, stderr))
registerStartupCommand(root, startupSnapshotRead, newListCmd(stdout, stderr))
registerStartupCommand(root, startupSnapshotRead, newPatternsCmd(stdout, stderr))
registerStartupCommand(root, startupMutation, newRebuildCmd(stdout, stderr))
registerStartupCommand(root, startupMutation, newPurgeCmd(stdout, stderr))
registerStartupCommand(root, startupSnapshotRead, newValidateCmd(stdout, stderr))
registerStartupCommand(root, startupSnapshotRead, newStatusCmd(stdout, stderr))
registerStartupCommand(root, startupMetadataRead, newConfigCmd(stdout, stderr))
registerStartupCommand(root, startupMutation, newAnnotateCmd(stdout, stderr))
registerStartupCommand(root, startupMutation, newRecoverCmd(stdout, stderr))
```

Do not change help/version behavior; Cobra still skips `PersistentPreRunE` for metadata rendering.

- [ ] **Step 6: Run command-policy tests**

```bash
gofmt -w cmd/backscroll/startup_commands.go cmd/backscroll/startup_commands_test.go cmd/backscroll/startup_policy.go cmd/backscroll/startup_policy_test.go cmd/backscroll/recover_test.go cmd/backscroll/compat_diagnostics_test.go
go test ./cmd/backscroll -run 'TestEveryOperationalCommand|TestMutation|TestInvalidOperational|TestFailedStartup|TestRecover|TestMetadataCommands' -count=1
```

Expected: PASS; no command executes startup for invalid argv/help/version.

- [ ] **Step 7: Commit explicit command ownership semantics**

```bash
git add cmd/backscroll/startup_commands.go cmd/backscroll/startup_commands_test.go cmd/backscroll/startup_policy.go cmd/backscroll/startup_policy_test.go cmd/backscroll/recover_test.go cmd/backscroll/compat_diagnostics_test.go
git commit -m "refactor(cli): classify startup command ownership"
```

---

### Task 5: Make Query Handlers Truly Read-Only

**Files:**
- Modify: `cmd/backscroll/index_policy.go:26-61`
- Modify: `cmd/backscroll/index_policy_test.go`
- Modify: `internal/compat/types.go`

**Interfaces:**
- Consumes: `storage.OpenReadOnly`, `compat.InspectIndex`, existing `indexDataRead`/`indexMutation`.
- Produces: `compat.CodeSyncInProgress`; read path returning a read-only `*storage.Database` only when `MigrationPlan.Steps` is empty.

- [ ] **Step 1: Add failing read-only and no-migration tests**

Add to `cmd/backscroll/index_policy_test.go`:

```go
func TestPrepareIndexDataReadReturnsReadOnlyConnection(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "index.db")
    writer, err := storage.Open(dbPath)
    if err != nil {
        t.Fatal(err)
    }
    if err := writer.Close(); err != nil {
        t.Fatal(err)
    }

    db, diag, err := prepareIndex(context.Background(), &config.Config{DatabasePath: dbPath}, indexDataRead)
    if err != nil || diag != nil {
        t.Fatalf("prepare read db=%v diag=%+v err=%v", db, diag, err)
    }
    defer db.Close()
    if _, err := db.DB().Exec(`CREATE TABLE must_not_exist (id INTEGER)`); err == nil {
        t.Fatal("indexDataRead returned a write-capable connection")
    }
}

func TestPrepareIndexDataReadDoesNotApplyPendingMigration(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "index.db")
    writer, err := storage.Open(dbPath)
    if err != nil {
        t.Fatal(err)
    }
    if _, err := writer.DB().Exec(`DELETE FROM schema_migrations WHERE version = 13`); err != nil {
        t.Fatal(err)
    }
    if err := writer.Close(); err != nil {
        t.Fatal(err)
    }

    db, diag, err := prepareIndex(context.Background(), &config.Config{DatabasePath: dbPath}, indexDataRead)
    if db != nil {
        _ = db.Close()
        t.Fatal("read preparation returned DB requiring migration")
    }
    if err == nil && diag == nil {
        t.Fatal("read preparation accepted pending migration")
    }

    inspect, err := storage.OpenReadOnly(dbPath)
    if err != nil {
        t.Fatal(err)
    }
    defer inspect.Close()
    var count int
    if err := inspect.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 13`).Scan(&count); err != nil {
        t.Fatal(err)
    }
    if count != 0 {
        t.Fatalf("read path applied migration 13 count=%d", count)
    }
}
```

- [ ] **Step 2: Run focused tests and verify red**

```bash
go test ./cmd/backscroll -run 'TestPrepareIndexDataRead' -count=1
```

Expected: the write-capability test fails because both classes currently call `OpenCompatible`.

- [ ] **Step 3: Implement non-migrating read preparation**

Split `prepareIndex` by class. For `indexDataRead`:

```go
func prepareReadOnlyIndex(ctx context.Context, path string) (*storage.Database, *compat.Diagnostic, error) {
    db, err := storage.OpenReadOnly(path)
    if err != nil {
        return nil, nil, err
    }
    plan, diag, err := compat.InspectIndex(ctx, db.DB())
    if err != nil || diag != nil {
        return nil, diag, closeIndexDB(db, err)
    }
    if len(plan.Steps) > 0 {
        err := fmt.Errorf("index requires %d migration steps", len(plan.Steps))
        return nil, &compat.Diagnostic{Code: compat.CodeIndexStale, Summary: err.Error()}, closeIndexDB(db, err)
    }
    return db, nil, nil
}
```

Preserve existing canonical continuation behavior in `prepareIndex` for ordinary handler failures. `indexMutation` continues to use `storage.OpenCompatible`.

Add to `internal/compat/types.go`:

```go
CodeSyncInProgress Code = "sync_in_progress"
```

- [ ] **Step 4: Run index and live-WAL tests**

```bash
gofmt -w cmd/backscroll/index_policy.go cmd/backscroll/index_policy_test.go internal/compat/types.go
go test ./cmd/backscroll -run 'TestPrepareIndexDataRead|TestLiveWAL|TestStaleIndex|TestMachineModes' -count=1
go test ./internal/storage -run 'TestOpenReadOnly|TestOpenReadOnlyLiveWAL' -count=1
```

Expected: read connections reject writes, pending migration remains unapplied, and existing live-WAL behavior passes.

- [ ] **Step 5: Commit the read-only boundary**

```bash
git add cmd/backscroll/index_policy.go cmd/backscroll/index_policy_test.go internal/compat/types.go
git commit -m "fix(storage): keep query handlers read-only"
```

---

### Task 6: Integrate Owner/Follower Coordination Into Startup Policy

**Files:**
- Create: `cmd/backscroll/startup_coordination.go`
- Create: `cmd/backscroll/startup_coordination_test.go`
- Modify: `cmd/backscroll/startup_policy.go`
- Modify: `cmd/backscroll/startup_policy_test.go`

**Interfaces:**
- Consumes: Task 2 `startuplock.TryAcquire/Acquire`, Task 4 command classes and lease lifecycle, Task 5 read-only preparation and `CodeSyncInProgress`.
- Produces: `coordinateStartup(context.Context, *config.Config, io.Writer, startupCommandClass) startupResult`; `defaultStartupMutationWait = 5*time.Second`; injectable `startupMutationWait`; `startupLockRetry = 50*time.Millisecond`; stderr warning rendering.

- [ ] **Step 1: Write injected owner/follower tests**

Create `cmd/backscroll/startup_coordination_test.go`. Use private function variables so tests do not need real OS contention:

```go
type fakeStartupLease struct {
    releases int
    err      error
}

func (f *fakeStartupLease) Release() error {
    f.releases++
    return f.err
}
```

Add tests for these exact cases:

1. immediate owner + snapshot read calls sync once and releases before result;
2. immediate owner + mutation calls sync once and returns retained lease;
3. busy snapshot read opens a seeded compatible DB read-only, skips sync, returns warning;
4. busy metadata `config` succeeds even when the DB does not exist and never calls read preparation;
5. busy mutation acquires within its wait and becomes owner;
6. busy mutation deadline returns `CodeSyncInProgress`, no continuation, no sync;
7. lock I/O error returns `startupStageSyncLock`, not a contention warning;
8. owner prepare/sync error keeps the lease only for recover's controlled path and otherwise releases before refusal.

A representative busy-read assertion:

```go
result := coordinateStartup(context.Background(), cfg, io.Discard, startupSnapshotRead)
if result.Failure != nil {
    t.Fatalf("busy snapshot failed: %+v", result.Failure)
}
if result.Warning == nil || result.Warning.Code != compat.CodeSyncInProgress {
    t.Fatalf("warning=%+v want sync_in_progress", result.Warning)
}
if syncCalls != 0 {
    t.Fatalf("follower sync calls=%d want 0", syncCalls)
}
```

Save and restore every injected package variable with `t.Cleanup`; do not call `t.Parallel` in tests that replace globals.

- [ ] **Step 2: Run the tests and verify red**

```bash
go test ./cmd/backscroll -run 'TestCoordinateStartup' -count=1
```

Expected: compilation fails because the coordinator and lock injection points are undefined.

- [ ] **Step 3: Implement the coordinator dependencies and constants**

Create `cmd/backscroll/startup_coordination.go` with:

```go
const (
    defaultStartupMutationWait = 5 * time.Second
    startupLockRetry           = 50 * time.Millisecond
)

var startupMutationWait = defaultStartupMutationWait

var (
    startupTryAcquire = func(path string) (startupLease, bool, error) {
        return startuplock.TryAcquire(path)
    }
    startupAcquire = func(ctx context.Context, path string, delay time.Duration) (startupLease, error) {
        return startuplock.Acquire(ctx, path, delay)
    }
)
```

Add `startupStageSyncLock` to startup stages. Implement helpers that create typed failures without recovery continuation for contention:

```go
func syncInProgressFailure(summary string, cause error) *startupFailure {
    return &startupFailure{
        Stage: startupStageSyncLock,
        Cause: cause,
        Diagnostic: compat.Diagnostic{
            Code:    compat.CodeSyncInProgress,
            Summary: summary,
        },
    }
}
```

- [ ] **Step 4: Implement immediate owner, busy reads, and bounded mutations**

Use this control structure:

```go
func coordinateStartup(ctx context.Context, cfg *config.Config, progress io.Writer, class startupCommandClass) startupResult {
    lease, acquired, err := startupTryAcquire(cfg.DatabasePath)
    if err != nil {
        return startupLockFailure(cfg, err)
    }
    if acquired {
        return runOwnedStartup(ctx, cfg, progress, class, lease)
    }

    switch class {
    case startupSnapshotRead:
        return concurrentSnapshotResult(ctx, cfg)
    case startupMetadataRead:
        return startupResult{
            Config: cfg,
            Warning: &startupWarning{
                Code: compat.CodeSyncInProgress,
                Summary: "startup sync active; continuing without a new startup sync",
            },
        }
    default:
        waitCtx, cancel := context.WithTimeout(ctx, startupMutationWait)
        defer cancel()
        lease, err := startupAcquire(waitCtx, cfg.DatabasePath, startupLockRetry)
        if err != nil {
            if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
                return startupResult{Config: cfg, Failure: syncInProgressFailure(
                    "startup sync remains active after 5s; retry the command", err,
                )}
            }
            if errors.Is(err, context.Canceled) || ctx.Err() != nil {
                return startupResult{Config: cfg, Failure: syncInProgressFailure(
                    "startup lock wait canceled; retry the command", err,
                )}
            }
            return startupLockFailure(cfg, err)
        }
        return runOwnedStartup(ctx, cfg, progress, startupMutation, lease)
    }
}
```

`concurrentSnapshotResult` calls `prepareIndex(..., indexDataRead)`, closes the inspection connection, and converts any absent/incompatible/pending-migration result into `sync_in_progress` without a recovery continuation. On success it returns summary `startup sync active; using last committed index snapshot`.

`runOwnedStartup` performs the existing prepare-close-sync sequence. It releases the lease before returning successful read-safe results. It retains the lease on successful mutations and mutation startup failures so root can either pass it to `recover` or release before refusal. Combine release errors with the primary error.

- [ ] **Step 5: Route default startup through coordination after pure validation**

In `defaultStartupPolicy`, keep input-dir, legacy-source, config-load, and active-manifest validation unchanged. Replace its direct prepare/sync block with:

```go
return coordinateStartup(ctx, cfg, progress, class)
```

The policy receives `class` from Task 4.

- [ ] **Step 6: Render warnings and release rejected leases in `PersistentPreRunE`**

After storing `startupResult` in command context:

```go
if result.Warning != nil {
    if _, err := fmt.Fprintf(stderr, "warning: %s: %s\n", result.Warning.Code, result.Warning.Summary); err != nil {
        return result.release(err)
    }
}
```

On failure:

- if command is recover, failure is recoverable, and class is mutation, return nil with the lease retained;
- otherwise call `result.release(failure)` before rendering/refusing so no pre-handler path leaks ownership.

Do not pass warnings through `startupProgressWriter`; warnings must reach stderr in JSON and robot modes too.

- [ ] **Step 7: Add stdout-contract and cleanup tests**

In `startup_policy_test.go`, add table cases for text/JSON/robot successful warnings. For JSON and robot, assert:

```go
if strings.Contains(stdout.String(), "sync_in_progress") {
    t.Fatalf("warning contaminated machine stdout: %q", stdout.String())
}
if !strings.Contains(stderr.String(), "warning: sync_in_progress:") {
    t.Fatalf("stderr missing freshness warning: %q", stderr.String())
}
```

Add fake-lease tests proving:

- read owner released before handler;
- mutation lease released after handler success and error;
- blocked non-recover command releases before diagnostic;
- recover continuation holds until its handler returns;
- release error is joined rather than replacing handler/startup error.

- [ ] **Step 8: Run policy and recovery tests**

```bash
gofmt -w cmd/backscroll/startup_coordination.go cmd/backscroll/startup_coordination_test.go cmd/backscroll/startup_policy.go cmd/backscroll/startup_policy_test.go
go test ./cmd/backscroll -run 'TestCoordinateStartup|Test.*Startup|Test.*Warning|TestRecover|TestMachineModes|TestHumanDiagnostic' -count=1
```

Expected: PASS; successful machine stdout remains parseable and only stderr carries freshness warnings.

- [ ] **Step 9: Commit the coordinated startup state machine**

```bash
git add cmd/backscroll/startup_coordination.go cmd/backscroll/startup_coordination_test.go cmd/backscroll/startup_policy.go cmd/backscroll/startup_policy_test.go
git commit -m "feat(cli): coordinate startup sync across processes"
```

---

### Task 7: Prove End-to-End Single-Flight and Snapshot Reads Across Processes

**Files:**
- Create: `cmd/backscroll/startup_coordination_process_test.go`

**Interfaces:**
- Consumes: production Cobra root, production advisory lock, `startupSync` injection, storage seed helpers, Task 6 warning/timeout contracts.
- Produces: subprocess helper contract `BACKSCROLL_COORDINATION_HELPER=1` plus barrier/counter environment variables.

- [ ] **Step 1: Create a hermetic child-process helper**

In `cmd/backscroll/startup_coordination_process_test.go`, add a test that becomes a helper only when the environment flag is set:

```go
func TestStartupCoordinationHelperProcess(t *testing.T) {
    if os.Getenv("BACKSCROLL_COORDINATION_HELPER") != "1" {
        return
    }
    counter := os.Getenv("BACKSCROLL_SYNC_COUNTER")
    ready := os.Getenv("BACKSCROLL_SYNC_READY")
    release := os.Getenv("BACKSCROLL_SYNC_RELEASE")
    block := os.Getenv("BACKSCROLL_SYNC_BLOCK") == "1"
    if wait := os.Getenv("BACKSCROLL_MUTATION_WAIT"); wait != "" {
        parsed, err := time.ParseDuration(wait)
        if err != nil {
            t.Fatal(err)
        }
        startupMutationWait = parsed
    }
    startupSync = func(*config.Config, io.Writer) error {
        file, err := os.OpenFile(counter, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
        if err != nil {
            return err
        }
        if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
            _ = file.Close()
            return err
        }
        if err := file.Close(); err != nil {
            return err
        }
        if block {
            if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
                return err
            }
            for {
                if _, err := os.Stat(release); err == nil {
                    break
                }
                time.Sleep(10 * time.Millisecond)
            }
        }
        return nil
    }

    argvJSON := os.Getenv("BACKSCROLL_HELPER_ARGV")
    var argv []string
    if err := json.Unmarshal([]byte(argvJSON), &argv); err != nil {
        t.Fatal(err)
    }
    root := buildRootCmd(os.Stdout, os.Stderr)
    root.SetArgs(argv)
    if err := root.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

`startupMutationWait` is the package variable initialized from `const defaultStartupMutationWait = 5*time.Second` in Task 6. Production never reads an environment variable; only this subprocess test helper assigns the variable inside its isolated child process.

- [ ] **Step 2: Add parent helpers for seed, spawn, and barriers**

Use `setIndexPolicyEnv` to scrub HOME/config and seed a compatible DB with one searchable row and UUID `seed-u`. Add helpers:

```go
func startCoordinationChild(t *testing.T, argv []string, extraEnv ...string) *exec.Cmd {
    t.Helper()
    encoded, err := json.Marshal(argv)
    if err != nil {
        t.Fatal(err)
    }
    cmd := exec.Command(os.Args[0], "-test.run=^TestStartupCoordinationHelperProcess$", "-test.v")
    cmd.Env = append(os.Environ(),
        "BACKSCROLL_COORDINATION_HELPER=1",
        "BACKSCROLL_HELPER_ARGV="+string(encoded),
    )
    cmd.Env = append(cmd.Env, extraEnv...)
    return cmd
}
```

Capture stdout/stderr separately for followers. Poll readiness with a deadline; never sleep for an unbounded fixed duration.

- [ ] **Step 3: Test one owner and nonblocking snapshot followers**

Test flow:

1. Start owner `status --json` with `BACKSCROLL_SYNC_BLOCK=1`.
2. Wait for the ready file proving it entered injected sync while holding the real OS lock.
3. Start `search <seed text> --all-projects --json` and `status --json` followers without blocking.
4. Require both to exit within two seconds.
5. Parse their stdout as valid JSON and require the seeded row/status.
6. Require stderr to contain `warning: sync_in_progress:`.
7. Read the counter and require exactly one PID: followers never entered sync.
8. Create the release file and require owner exit success.

Use a timer/select around `cmd.Wait` so a regression cannot hang the suite.

- [ ] **Step 4: Test bounded mutation and no handler side effect**

While a blocking owner holds the lock, run:

```text
annotate --uuid seed-u --kind correction --label should-not-write
```

with helper-only `BACKSCROLL_MUTATION_WAIT=150ms`. Assert:

- nonzero exit;
- stderr or machine diagnostic carries `sync_in_progress` and retry guidance;
- sync counter remains one line;
- direct read-only SQL shows zero matching annotation rows.

Do not use malformed argv; early validation must pass so the test reaches lock coordination.

- [ ] **Step 5: Test killed owner recovery**

Start another blocking owner, wait for ready, kill it, and wait for process exit. Then start a normal `status --json` child that does not block its injected sync. Require:

- exit success within five seconds;
- counter gains one new owner PID;
- no stale warning for the replacement owner.

This is the CLI-level acceptance test that a crashed synchronizer cannot leave a permanent lock.

- [ ] **Step 6: Test metadata follower with missing DB**

Point `BACKSCROLL_DATABASE_PATH` at a missing file. In the parent test process, call `startuplock.TryAcquire(dbPath)` directly and retain the returned lease while running a child with `config --json`; this holds the canonical OS lock without creating the database. Assert valid config JSON, stderr warning text `continuing without a new startup sync`, and that the database path remains absent. Release the parent lease with `t.Cleanup`.

- [ ] **Step 7: Run the process tests repeatedly and under race detection**

```bash
gofmt -w cmd/backscroll/startup_coordination_process_test.go
go test ./cmd/backscroll -run 'TestStartupCoordination(Process|SingleFlight|Mutation|Crash|Metadata)' -count=5 -v
go test -race ./cmd/backscroll -run 'TestStartupCoordination' -count=1
```

Expected: all pass without leaked child processes or lock files being deleted.

- [ ] **Step 8: Commit the end-to-end concurrency proof**

```bash
git add cmd/backscroll/startup_coordination_process_test.go
git commit -m "test(cli): prove startup sync single-flight"
```

---

### Task 8: Update Living Contracts and Run Release Gates

**Files:**
- Modify: `docs/sync.md`
- Modify: `CLAUDE.md`
- Verify: `docs/adr/0003-coordinar-sync-entre-procesos.md`
- Verify: `docs/superpowers/specs/2026-08-21-startup-sync-coordination-design.md`

**Interfaces:**
- Consumes: completed behavior from Tasks 1-7.
- Produces: living documentation matching command classes, sidecar lifecycle, stale-warning semantics, and new package layout.

- [ ] **Step 1: Update `docs/sync.md` with the coordinated pipeline**

Replace the single-process startup description with a diagram equivalent to:

```text
validated invocation
  -> try <canonical-db>.startup-sync.lock
     -> owner: prepare/migrate -> incremental sync -> command
     -> busy snapshot read: compatible OpenReadOnly -> stderr warning -> query WAL snapshot
     -> busy config: stderr warning -> print validated config
     -> busy mutation: wait <=5s -> owner or retryable sync_in_progress
```

Document that the empty `0600` sidecar persists, is never deleted, and only its OS lock represents ownership. State that WAL/locking support is local-host only.

- [ ] **Step 2: Update every required `CLAUDE.md` inventory**

Make all pre-push-gated sections consistent:

- add `internal/startuplock` to the Implemented list;
- add `startuplock/` to Module Layout;
- add the package row to Package Layout;
- update Core Pipeline to show owner/follower coordination;
- replace the strict single-process mandatory-sync wording with the approved concurrent exception;
- add a key decision describing `gofrs/flock`, persistent sidecar, 5-second mutation wait, and read-only WAL followers;
- replace migration count only if implementation added a migration (this plan must not add one).

- [ ] **Step 3: Run documentation and ADR validation**

```bash
rootline validate docs/adr/0003-coordinar-sync-entre-procesos.md --strict
git diff --check
just check
```

Expected: Rootline valid with zero warnings/errors; formatting and vet pass.

- [ ] **Step 4: Run focused acceptance verification**

```bash
go test ./internal/tagging -run 'Test(Tag|Accumulator)' -count=1
go test ./internal/tagging -run '^$' -bench BenchmarkAccumulatorScaling -benchmem -count=3
go test ./internal/startuplock -count=5
go test ./cmd/backscroll -run 'TestStartupCoordination|TestPrepareIndexDataRead|TestEveryOperationalCommand' -count=3
GOOS=windows GOARCH=amd64 go test -c ./internal/startuplock -o "$(mktemp -d)/startuplock.test.exe"
```

Expected: all tests/compilation pass. Preserve benchmark output for the PR description.

- [ ] **Step 5: Run complete release gates from a scrubbed environment**

```bash
go test -race ./...
just test
just ci
```

Expected: all pass and aggregate statement coverage remains at least 85%. If coverage drops below the gate, add behavior-focused tests to the new coordination package and policy branches; do not add assertion-free coverage calls.

- [ ] **Step 6: Review the complete branch against issue #46 and the spec**

Check each acceptance criterion explicitly:

```bash
git diff --check origin/main...HEAD
git log --oneline origin/main..HEAD
git status --short
```

Expected: clean worktree; no unrelated files; commits correspond to linear tagging, lock primitive, crash proof, classification, read-only boundary, coordination, process proof, and docs.

Verify manually in the diff that:

- `sessionText +=` no longer exists;
- sidecar removal does not exist;
- successful warning output targets stderr;
- mutation timeout is exactly five seconds in production;
- `config` follower does not open/create the DB;
- snapshot handlers use `OpenReadOnly`;
- recover owns the lock through post-install sync;
- no hashing/progress/MCP implementation entered the branch.

- [ ] **Step 7: Commit living documentation**

```bash
git add docs/sync.md CLAUDE.md
git commit -m "docs: document coordinated startup sync"
```

- [ ] **Step 8: Request independent final review before integration**

Use `superpowers:requesting-code-review` against `origin/main...HEAD`. Resolve every Critical or Important finding with a fresh red-green cycle, rerun `just ci`, and only then use `superpowers:finishing-a-development-branch` to choose PR/integration handling.
