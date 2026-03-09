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

Requires Rust 1.85+ (edition 2024) and [just](https://github.com/casey/just).

## Just Recipes

Run `just --list` to see all available recipes. Key ones:

| Recipe | What it does |
|--------|-------------|
| `just check` | Format check + clippy + cargo check |
| `just test` | Run all tests |
| `just fmt` | Auto-format code |
| `just audit` | Dependency license/ban audit |
| `just build` | Release build |
| `just sync-version` | Sync `Cargo.toml` version with latest git tag |
| `just release-patch` | Full release: check → test → sync → bump patch → commit → tag → push |
| `just release-minor` | Same but bumps minor version |

## Workflow

1. Fork the repository
2. Create a feature branch from `master`
3. Make your changes
4. Run `just check` and `just test`
5. Commit using [Conventional Commits](https://www.conventionalcommits.org/)
6. Open a Pull Request

## Releasing

Releases use `just` recipes that sync with the latest git tag, run quality gates, bump the version, and push:

```bash
just release-patch   # v0.1.14 → v0.1.15
just release-minor   # v0.1.14 → v0.2.0
```

CI also auto-tags on push to master based on conventional commit prefixes.

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

- **Formatting**: `cargo fmt` (enforced in CI and pre-commit hook)
- **Linting**: `clippy` with nursery + pedantic lints, `-D warnings`
- **Testing**: Co-located unit tests + integration tests in `tests/cli.rs`
- **Snapshots**: Update with `cargo insta review`

## Git Hooks

Hooks live in `.githooks/` and are activated with `git config core.hooksPath .githooks`.

| Hook | What it does |
|------|-------------|
| `pre-commit` | cargo fmt check, clippy, gitleaks secret scan |
| `commit-msg` | Validates conventional commit format |
| `pre-push` | Validates docs/epics, checks code-docs drift, rebuilds binary |
| `post-merge` | Rebuilds binary, propagates doc aggregates |

## Quality Gates

All PRs must pass:
- `cargo fmt --all -- --check`
- `cargo clippy --all-targets --all-features -- -D warnings`
- `cargo test --all-features`
- `cargo deny check licenses bans advisories`

## Reporting Issues

- **Bugs**: Use the bug report template
- **Features**: Use the feature request template
- **Security**: See [SECURITY.md](SECURITY.md) for responsible disclosure
