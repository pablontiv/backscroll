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
