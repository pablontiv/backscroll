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

Requires Rust 1.85+ (edition 2024).

## Workflow

1. Fork the repository
2. Create a feature branch from `master`
3. Make your changes
4. Run `just check` and `just test`
5. Commit using [Conventional Commits](https://www.conventionalcommits.org/)
6. Open a Pull Request

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

- **Formatting**: `cargo fmt` (enforced in CI)
- **Linting**: `clippy` with nursery + pedantic lints, `-D warnings`
- **Testing**: Co-located unit tests + integration tests in `tests/cli.rs`
- **Snapshots**: Update with `cargo insta review`

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
