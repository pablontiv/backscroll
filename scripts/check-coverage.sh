#!/usr/bin/env bash
set -euo pipefail

THRESHOLD="${COVERAGE_THRESHOLD:-85}"

go test -coverprofile=coverage.out ./... 1>/dev/null

total=$(go tool cover -func=coverage.out | grep "^total:" | awk '{print $3}' | tr -d '%')

echo "Total coverage: ${total}%  (threshold: ${THRESHOLD}%)"

if awk "BEGIN { exit !($total >= $THRESHOLD) }"; then
    echo "Coverage gate: PASS"
else
    echo "Coverage gate: FAIL (${total}% < ${THRESHOLD}%)"
    exit 1
fi
