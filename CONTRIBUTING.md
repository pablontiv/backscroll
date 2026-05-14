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
| `pre-push` | Validates docs, rebuilds binary |

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
