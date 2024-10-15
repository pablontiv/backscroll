# Contributing to Backscroll

## Development Setup

```bash
# Clone and enter the repo
git clone https://github.com/pablontiv/backscroll.git
cd backscroll

# Set up git hooks
git config core.hooksPath .githooks

# Verify environment
just check
just test
```

Requires Go 1.22+ and [just](https://github.com/casey/just).

## Just Recipes

Run `just --list` to see all available recipes. Key ones:

| Recipe | What it does |
|--------|-------------|
| `just check` | `gofmt` check + `go vet` |
| `just test` | Run all tests |
| `just fmt` | Auto-format code |
| `just audit` | `go mod verify` |
| `just build` | Build binary |
| `just coverage-summary` | Test coverage report |

## Workflow

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes
4. Run `just check` and `just ci`
5. Commit using [Conventional Commits](https://www.conventionalcommits.org/)
6. Open a Pull Request
7. Require a passing `candidate` check for the pull request's current head SHA

After merge, install and verify the resulting stable release. Never install or
verify a development (`dev`) build.

## Dagu issue flow

The repository adopts `@a4s/omp-dagu@0.3.101` from
`pablontiv/a4s@04846df5940d65b8e230eafadff6621ae9186122`, rendered by
`bootstrap-backscroll-v0-3-101` for Dagu `2.20.1`. The exact bootstrap artifacts
are versioned under `.workspace/dagu/dags/`: `feature.yaml` delivers issues,
`poc.yaml` proves feasibility, `discovery.yaml` investigates repositories, and
`planning.yaml` imports prepared backlog units for independent review, operator
approval, and issue materialization.

Adoption preserves `BASE_BRANCH=main`,
`PRIMARY=/Users/Shared/harness/backscroll`, `GATE=just check && just ci`,
`REQUIRED_CHECK=candidate`, and
`TEST_PATHS=(^|/)tests/|_test\.go$|^scripts/test-.*\.sh$`.
`REBASE_RESOLVER` and `VALIDATION` are empty. Existing Go behavior and CI remain
unchanged.

From the clean primary clone on `main`, obtain the coordinating session ID with
`dagu_session_id`, then enqueue through the loaded bundle's admission boundary:

```sh
dagu enqueue --run-id "backscroll-issue-$ISSUE" \
  .workspace/dagu/dags/feature.yaml -- \
  ISSUE="$ISSUE" \
  PR_NUMBER="" OMP_SESSION_ID="$OMP_SESSION_ID"
```

The coordinator completes guarded `triage` through the designated
`triage-reviewer` and owns the human-task transitions. At `implement`, leave the
delivery uncommitted in the assigned worktree: Dagu captures the candidate,
rebases it, and runs `just check && just ci`. Independent QA and review bind
their verdicts to that candidate before guarded publication and auto-merge.
The scheduled reconciler owns `await_ci` and `await_merge`, including CI
recovery, merge observation, and return to implementation or rebase when needed.
Dagu verifies integration, updates the primary clone, and cleans up owned
worktrees and branches at close.

A failed or aborted run is terminal. Inspect its logs and resource evidence,
preserve retained work, and fix the cause before enqueueing a replacement with
the same `ISSUE`, branch, and coordinating session ID plus a suffixed run ID.
Set `PR_NUMBER` when an open pull request exists. Dagu validates and transfers
ownership of the retained branch and worktree while preserving HEAD and files.
For fresh-clone recovery, fetch the durable checkpoint with
`git fetch origin refs/dagu/checkpoints/issue-$ISSUE` and follow the bundle
skill's checkpoint recovery procedure.

## Releasing

Releases are fully automated via CI. On push to `main`, CI analyzes conventional commit prefixes, calculates the next semver version, builds multi-platform binaries (Linux, macOS, Windows), and creates a GitHub Release. No manual release steps needed.

## Commit Convention

```
type(scope): description
```

| Type | When to use |
|------|-------------|
| `feat` | New user-facing functionality |
| `fix` | Bug fix |
| `refactor` | Internal restructuring, no behavior change |
| `perf` | Performance improvement |
| `test` | Adding or updating tests |
| `docs` | Documentation only |
| `chore` | Build, CI, dependency updates |

Breaking changes use `!` suffix: `feat!: remove deprecated flag`

## Code Style

- **Formatting**: `gofmt` (enforced in CI and pre-commit hook)
- **Linting**: `go vet`
- **Testing**: stdlib `testing` package; unit tests co-located, integration tests in `cmd/backscroll/main_test.go`
- **Coverage gate**: ≥85% enforced by CI

## Git Hooks

Hooks live in `.githooks/` and are activated with `git config core.hooksPath .githooks`.

| Hook | What it does |
|------|-------------|
| `pre-commit` | `gofmt` check, `go vet`, gitleaks secret scan |
| `commit-msg` | Validates conventional commit format |
| `pre-push` | Validates documentation and runs CI |

## Quality Gates

All PRs must pass:
- `gofmt --check`
- `go vet ./...`
- `go test ./...`
- Coverage ≥85%

## Reporting Issues

- **Bugs**: Use the bug report template
- **Features**: Use the feature request template
- **Security**: See [SECURITY.md](SECURITY.md) for responsible disclosure
