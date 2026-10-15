# CI layout

This document is the source of truth for which checks run at each delivery moment. The required branch-protection check remains `candidate`; it fails closed unless every pull-request dependency finishes with `success`.

## Measured baseline

The baseline is pull request #184, workflow run `38775504959`, plus local race-enabled package profiles from the pre-change tree (`84155f9`). GitHub durations are wall-clock job durations.

| Baseline work | Measured cost |
|---|---:|
| Crossbeam `ci / Test & Build` | 444 s |
| `gitleaks` | 9 s |
| Durability: Ubuntu / macOS / Windows | 25 / 45 / 61 s |
| Skill install: Ubuntu / macOS | 36 / 87 s |
| Candidate metadata / build / static validation / smoke / publish | 30 / 115 / 34 / 8 / 6 s |
| Final `candidate` verification | 5 s |
| End-to-end pull-request critical path | 660 s |
| `go test -race -count=1 ./cmd/backscroll` | 460.51 s wall; package 453.817 s |
| `go test -race -count=1 ./internal/storage` | 277.94 s wall; package 275.383 s |

The command package profile had 474 top-level tests, 59 at or above one second, and 19 at or above five seconds. `TestRequiredOutputFailures` cost 69.64 s; the process-based query-echo family repeatedly compiled private binaries and contributed about 170 s; `TestProjectProofBacklogConvergesAcrossRegistryRebuild` cost 13.78 s. The storage profile had 499 top-level tests; `TestRecallFrequencyUnfilteredSQLMatchesGoScan` alone cost 107.28 s because its recall assertion used the full ingest/mining pipeline and then repeated both scans as a timing loop.

## Optimized local evidence

| Work | Wall time | Change from baseline |
|---|---:|---:|
| Full `cmd/backscroll` race suite | 196.80 s | 2.34x faster |
| Full `internal/storage` race suite | 243.46 s | 1.14x faster |
| Complete plain repository suite | 41.36 s | New merge-gate basis |
| Command concurrency race leg | 4.98 s | New targeted leg |
| Storage concurrency race leg | 28.94 s | New targeted leg |
| Autoupdate + startup-lock race leg | 3.68 s | New targeted leg |
| Sequential local `just ci` | 77.56 s | Aggregate coverage 86.9% |

The complete package race commands remain useful for local diagnostics and demonstrate that parallelization did not introduce races. Merge latency uses the concurrency-focused race legs because the complete plain suite already executes every serial behavior assertion.

## Decision table

| Job or step | Decision and moment | Contract protected | Measured baseline cost |
|---|---|---|---:|
| Crossbeam `go-ci` light profile | **Keep on pull request, push, and schedule.** Run the full package suite without `-race` and enforce aggregate statement coverage at 85%. | Compilation, behavior, and the merge-blocking coverage floor. | 444 s before test optimization |
| Concurrency-focused race matrix | **Keep on pull request, push, and schedule; optimize to tests that exercise concurrent behavior.** Split command coordination/update/cancellation, storage concurrency/transaction/cancellation, and complete `internal/autoupdate` + `internal/startuplock` suites across three runners. The plain coverage job still runs every test. | Goroutine lifecycle, SQLite concurrent-open/transaction behavior, update publication, and startup-lock ownership without repeating every serial behavior assertion under `-race`. | Full-package baselines above; the matrix pays only the slowest concurrency-focused leg |
| Gitleaks | **Keep on pull request, push, and schedule.** | Secret disclosure in tracked changes. | 9 s |
| Native durability matrix | **Move from pull request to push-to-main and schedule.** | Real filesystem durability and recovery behavior on Linux, macOS, and Windows. | 25 / 45 / 61 s in parallel |
| Durability standalone `go build ./...` | **Remove.** The immediately following `go test` compiles the production packages for the native runner. | No additional contract. | About 2–4 s inside each durability job |
| Skill-install matrix | **Move from pull request to push-to-main and schedule.** | Python installer, shipped skill recipe, install script, and shell isolation on Linux and macOS. | 36 / 87 s in parallel |
| PR six-target `candidate-build` and artifact revalidation | **Remove from pull requests; protect at release.** Crossbeam `go-release` builds the same Linux, macOS, and Windows amd64/arm64 release matrix after the main gate, with release checksums, provenance, and attestations. | Release archive portability and supply-chain identity at the moment artifacts can be published. | 205 s serial candidate workflow plus queue gaps |
| Required `candidate` aggregate | **Keep on pull requests and simplify.** Require `ci`, every race matrix leg, and `gitleaks` to equal `success`. | One stable, strict branch-protection status that fails closed. | 5 s |
| Crossbeam release | **Keep on push to main.** Require `ci`, race, gitleaks, durability, and skill-install first. | Versioning, six-platform archives, checksums, provenance, attestations, and GitHub Release publication. | Not on the PR path |
| Weekly schedule | **Add Monday 06:17 UTC.** Run non-release verification, including the moved native and installation matrices. | Detect runner/toolchain/OS drift between releases. | Off the PR path |
| Local install regression work in `just ci` | **Move to push-to-main and schedule via `skill-install`.** | Installer portability before a release is published. | Included in the 36 / 87 s jobs |
| Separate local build in `just ci` | **Remove.** `go test ./...` compiles all packages. | No additional contract. | Duplicated compilation |

## Execution moments

### Pull request

The merge gate starts these independent jobs together:

1. `ci`: Crossbeam light Go CI, including aggregate coverage `>=85%`.
2. `race`: three concurrency-focused matrix legs for command coordination/update/cancellation, storage concurrency/transactions, and the complete autoupdate/startuplock packages.
3. `gitleaks`: tracked-secret scanning.
4. `candidate`: the only required aggregate; it runs with `always()` and accepts only `success` from all three dependencies.

The expected critical path is:

```text
max(ci, command concurrency race, storage concurrency race, runtime race, gitleaks) + candidate (~5 s)
```

On the measured workstation, the complete plain suite took 41.36 seconds and the slowest separately measured race leg took 28.94 seconds. With those jobs running in parallel, the local-equivalent critical path is about 46 seconds including the five-second aggregate. Allowing more than a minute for checkout, Go setup, cache variance, and runner variance gives a conservative **under-two-minute expected pull-request critical path**, versus the measured 660-second baseline. Queue time is excluded.

This removes the former 205-second serial candidate artifact chain and the 25–87-second native/install matrices from the pre-merge path. Coverage remains enforced before merge rather than deferred.

### Push to main

The pull-request quality set runs again on the merged commit. Native durability and skill-install matrices also run. The release job requires all five job groups (`ci`, `race`, `gitleaks`, `durability`, `skill-install`) before Crossbeam may tag, build the six target archives, attest, and publish.

### Weekly schedule

The weekly run executes `ci`, targeted race, gitleaks, native durability, and skill installation. Release is push-only, so a scheduled run cannot tag or publish.

## Local gate

`just ci` mirrors the code-bearing merge gate locally:

1. one scrubbed-HOME `go test -count=1 ./... -coverprofile=coverage.out` run;
2. aggregate coverage enforcement at 85%;
3. the same three concurrency-focused race invocations used by the workflow.

The local gate does not repeat serial behavior assertions under `-race`; the complete plain suite already supplies those assertions. `just coverage-check` remains the advisory per-package floor report. Native OS and skill-install matrices belong to post-merge runners because they depend on runner operating systems rather than additional source-level assertions.

## Test execution design

Process-based query-echo tests share one package binary built lazily by `TestMain`; every process still receives isolated HOME, config, database, and input paths. Read-only output-contract commands share one prepared database and injected startup snapshot per population fixture, then execute their complete success/write-failure matrices in parallel. The project-proof backlog uses one 401-row fixture whose 200, 400, and 401 checkpoints cover the shared cap and terminal remainder without rebuilding a second prefix.

Tests call `t.Parallel()` only when their temporary filesystem/database state and injected dependencies are independent. Process-global environment tests remain serial; startup-coordination subprocesses receive explicit child environments and therefore run in parallel. `startupCoordinator.mutationWait`, `runDependencies.stagingWait`, and `runDependencies.stagingTimer` are the test injection points for bounded startup and autoupdate waits, so tests do not sleep for production defaults.
