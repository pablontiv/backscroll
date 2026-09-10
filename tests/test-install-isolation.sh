#!/usr/bin/env bash
# Regression for issue #74: tests/test-install.sh must never leak its
# FAKE_BINARY fixture into a real caller's $HOME/.local/bin. The suite is
# supposed to establish its own BACKSCROLL_INSTALL_DIR/BACKSCROLL_CONFIG_DIR
# overrides before install.sh is sourced or invoked — never after.
#
# This drives the real suite (unmodified) against a synthetic caller HOME
# with no BACKSCROLL_INSTALL_DIR/BACKSCROLL_CONFIG_DIR/BACKSCROLL_INPUTS_SOURCE_DIR
# inherited from this process — i.e. today's ordinary, unwrapped invocation —
# and asserts the synthetic HOME's default binary location is left untouched:
# byte- and mode-preserved when a binary already exists there, still absent
# when it does not. All writes are contained to per-scenario temp directories;
# the real $HOME is never used as a target.
set -uo pipefail

PASS=0
FAIL=0
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SUITE="$SCRIPT_DIR/tests/test-install.sh"

pass() {
    ((PASS++))
    echo "  PASS: $1"
}
fail() {
    ((FAIL++))
    echo "  FAIL: $1 — $2"
}

file_mode() {
    stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1" 2>/dev/null
}

# Runs the real installation suite against a synthetic HOME, with no
# BACKSCROLL_INSTALL_DIR/BACKSCROLL_CONFIG_DIR/BACKSCROLL_INPUTS_SOURCE_DIR
# inherited from this process — reproducing an ordinary caller invocation.
run_suite_against_home() {
    local synthetic_home="$1" log="$2"
    env -u BACKSCROLL_INSTALL_DIR -u BACKSCROLL_CONFIG_DIR -u BACKSCROLL_INPUTS_SOURCE_DIR \
        HOME="$synthetic_home" \
        bash "$SUITE" >"$log" 2>&1
}

echo "=== install.sh test-suite isolation regression (issue #74) ==="

# Scenario: a pre-existing default binary must be byte- and mode-preserved.
echo "[default binary preserved when one already exists]"
SYN_HOME=$(mktemp -d)
LOG=$(mktemp)
mkdir -p "$SYN_HOME/.local/bin"
TARGET="$SYN_HOME/.local/bin/backscroll"
printf '#!/bin/sh\nprintf "ORIGINAL_BINARY\\n"\n' >"$TARGET"
chmod 755 "$TARGET"
ORIGINAL_COPY=$(mktemp)
cp -p "$TARGET" "$ORIGINAL_COPY"

run_suite_against_home "$SYN_HOME" "$LOG" || true

if [ -e "$TARGET" ] && cmp -s "$ORIGINAL_COPY" "$TARGET" && [ "$(file_mode "$TARGET")" = "755" ]; then
    pass "pre-existing default binary is byte- and mode-preserved"
else
    fail "pre-existing default binary preservation" "suite log tail: $(tail -5 "$LOG")"
fi
rm -rf "$SYN_HOME" "$ORIGINAL_COPY" "$LOG"

# Scenario: no default binary must appear when none existed before.
echo "[no default binary appears when absent]"
SYN_HOME=$(mktemp -d)
LOG=$(mktemp)
TARGET="$SYN_HOME/.local/bin/backscroll"

run_suite_against_home "$SYN_HOME" "$LOG" || true

if [ ! -e "$TARGET" ]; then
    pass "no default binary is created when none existed"
else
    fail "default binary should stay absent" "found $(stat -f '%z bytes, mode %Lp' "$TARGET" 2>/dev/null || stat -c '%s bytes, mode %a' "$TARGET")"
fi
rm -rf "$SYN_HOME" "$LOG"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
