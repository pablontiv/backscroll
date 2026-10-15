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

# Race detector for tests that exercise goroutines, cancellation, concurrent
# storage, update publication, and cross-process startup locking.
race:
    go test -race -count=1 ./cmd/backscroll -run '^(Test.*(Concurrent|Coordination|CoordinateStartup|Cancellation|Canceled|Atomic|Goroutine|Staging|Signal|Lock)|TestProductionUpdater.*)$'
    go test -race -count=1 ./internal/storage -run '^Test.*(Concurrent|Cancellation|Canceled|Atomic|TransactionGate|Busy|Lock|LiveWAL|EEXIST)'
    go test -race -count=1 ./internal/autoupdate ./internal/startuplock

# Native skill and preset unit tests plus hermetic CLI E2E tests.
test-skills:
    config_dir="$(mktemp -d)" && home="$(mktemp -d)" && \
    gomodcache="$(go env GOMODCACHE)" && gocache="$(go env GOCACHE)" && \
    trap 'rm -rf "$config_dir" "$home"' EXIT && \
    HOME="$home" BACKSCROLL_CONFIG_DIR="$config_dir" \
    GOMODCACHE="$gomodcache" GOCACHE="$gocache" \
        go test ./internal/skillinstall ./internal/presets ./cmd/backscroll

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

# Local mirror of the merge gate: aggregate coverage ≥85%, followed by the
# concurrency-focused race set. go test compiles every package, so a separate
# build adds no protection; install and durability matrices run after merge.
ci:
    test_root="$(mktemp -d)" && trap 'rm -rf "$test_root"' EXIT && \
    module_cache="$(go env GOMODCACHE)" && build_cache="$(go env GOCACHE)" && \
    mkdir -p "$test_root/home" "$test_root/config" && \
    HOME="$test_root/home" BACKSCROLL_CONFIG_DIR="$test_root/config" GOMODCACHE="$module_cache" GOCACHE="$build_cache" go test -count=1 ./... -coverprofile=coverage.out && \
    go tool cover -func=coverage.out | grep total | awk '{print substr($3, 1, length($3)-1)}' | { read cov; echo "Coverage: ${cov}%"; if (( $(echo "$cov < 85" | bc -l) )); then echo "Coverage ${cov}% below 85%"; exit 1; fi; } && \
    HOME="$test_root/home" BACKSCROLL_CONFIG_DIR="$test_root/config" GOMODCACHE="$module_cache" GOCACHE="$build_cache" just race
