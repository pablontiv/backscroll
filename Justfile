# Justfile for Backscroll
set shell := ["bash", "-c"]

# Default recipe
default: check test

# Run fmt + vet + staticcheck
check:
    gofmt -l . | grep -q . && { echo "gofmt: unformatted files"; exit 1; } || true
    go vet ./...

# Format code
fmt:
    gofmt -w .

# Run tests with isolated config dir
test:
    config_dir="$(mktemp -d)" && trap 'rm -rf "$config_dir"' EXIT && \
    BACKSCROLL_CONFIG_DIR="$config_dir" go test ./...

# Isolated skill installer filesystem + installed-recipe E2E (Python 3.9+)
test-skills:
    mkdir -p .local-evidence
    go build -o .local-evidence/backscroll ./cmd/backscroll
    BACKSCROLL_TEST_BINARY="$PWD/.local-evidence/backscroll" python3 tests/test_skill_install.py -v

# Build binary
build:
    go build -o backscroll ./cmd/backscroll

# Coverage summary
coverage-summary:
    go test -cover ./...

# Show coverage per package and total
coverage:
    go test ./... -coverprofile=coverage.out
    go run github.com/pablontiv/picokit/cmd/pkcov report

# Check coverage meets per-package floors
coverage-check: coverage
    go run github.com/pablontiv/picokit/cmd/pkcov check

# Audit dependencies
audit:
    go mod verify

# Install-script bash regression suite (issue #74). Scrubs inherited BACKSCROLL_*
# and XDG_CONFIG_HOME so a developer shell environment cannot mask regressions
# or override test 14's macOS config-dir assertion (install.sh:88 prefers
# XDG_CONFIG_HOME over HOME).
install-tests:
    env -u XDG_CONFIG_HOME -u BACKSCROLL_INSTALL_DIR -u BACKSCROLL_CONFIG_DIR \
        -u BACKSCROLL_INPUTS_SOURCE_DIR \
        bash tests/test-install.sh
    env -u XDG_CONFIG_HOME -u BACKSCROLL_INSTALL_DIR -u BACKSCROLL_CONFIG_DIR \
        -u BACKSCROLL_INPUTS_SOURCE_DIR \
        bash tests/test-install-isolation.sh

# Local mirror of CI gate: build + scrubbed-HOME tests + coverage ≥85% +
# install-script bash regression suite.
ci:
    go build ./...
    config_dir="$(mktemp -d)" && trap 'rm -rf "$config_dir"' EXIT && \
    HOME="$(mktemp -d)" BACKSCROLL_CONFIG_DIR="$config_dir" go test ./... -coverprofile=coverage.out && \
    go tool cover -func=coverage.out | grep total | awk '{print substr($3, 1, length($3)-1)}' | { read cov; echo "Coverage: ${cov}%"; if (( $(echo "$cov < 85" | bc -l) )); then echo "Coverage ${cov}% below 85%"; exit 1; fi; } && \
    just install-tests
