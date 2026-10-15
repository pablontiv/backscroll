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
4. Run `just check` and `just test`
5. Commit using [Conventional Commits](https://www.conventionalcommits.org/)
6. Open a Pull Request
7. Before closing the pull request, require a passing candidate for its current
   head SHA and smoke-test that candidate locally, side by side with the stable
   installation, against a transactionally consistent copy of the local database

Every pull request receives an automatic candidate build. Follow the
[PR candidate artifact runbook](docs/runbooks/pr-candidate.md) to verify and
smoke-test it without exposing the live database or replacing the stable binary.
A new candidate is required after every pull request synchronization.

After merge, install and verify the resulting stable release. Never install or
verify a development (`dev`) build.

## Dagu issue flow

Maintainers deliver issues with the versioned
`.workspace/dagu/dags/feature.yaml`. From the clean primary clone on `main`,
obtain the coordinating session ID with `dagu_session_id`, then enqueue:

```sh
dagu enqueue --run-id "backscroll-issue-$ISSUE" \
  .workspace/dagu/dags/feature.yaml -- \
  ISSUE="$ISSUE" PRIMARY="$PWD" BRANCH="dagu/issue-$ISSUE" \
  PR_NUMBER="" OMP_SESSION_ID="$OMP_SESSION_ID"
```

The coordinator completes guarded `triage` through the designated
`triage-reviewer` and owns the `triage` and `implement` human tasks. The
scheduled reconciler alone advances or pushes back `await_merge`; leave that
task for it rather than completing it manually.

A failed or aborted run is terminal. After fixing the cause, recover a pushed
checkpoint with `git fetch origin && git branch dagu/issue-$ISSUE
origin/dagu/issue-$ISSUE`, then enqueue a new run with a suffixed run ID. Set
`PR_NUMBER` when the branch already has an open pull request. Runs that failed
before a checkpoint can be enqueued again after their cleanup completes.

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
