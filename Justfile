# Justfile for Backscroll
set shell := ["bash", "-c"]

# Default recipe
default: check test

# Run all checks (fmt, clippy, check)
check:
    cargo fmt --all -- --check
    cargo clippy --all-targets --all-features -- -D warnings
    cargo check --all

# Format code
fmt:
    cargo fmt --all

# Run tests with isolated input config so user manifests in ~/.config/backscroll/inputs do not affect the suite.
test:
    config_dir="$(mktemp -d)" && trap 'rm -rf "$config_dir"' EXIT && echo "BACKSCROLL_CONFIG_DIR=$config_dir" && BACKSCROLL_CONFIG_DIR="$config_dir" cargo test --all-features

# Build in release mode
build:
    cargo build --release

# Static build using Zig (Linux musl)
static-build:
    source $HOME/.cargo/env && \
    export PATH=$PATH:$(pwd)/zig-linux-x86_64-0.13.0 && \
    cargo zigbuild --release --target x86_64-unknown-linux-musl

# Code coverage report (LCOV)
coverage:
    source $HOME/.cargo/env && \
    cargo llvm-cov --all-features --workspace --lcov --output-path lcov.info

# Show coverage summary
coverage-summary:
    source $HOME/.cargo/env && \
    cargo llvm-cov --all-features --workspace

# Audit dependencies
audit:
    cargo deny check licenses bans

# Generate CHANGELOG.md from conventional commits
changelog:
    git-cliff --output CHANGELOG.md

