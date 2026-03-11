#!/usr/bin/env bash
# Unit tests for install.sh — validates structure and logic
set -uo pipefail

PASS=0
FAIL=0
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$SCRIPT_DIR/install.sh"

pass() { ((PASS++)); echo "  PASS: $1"; }
fail() { ((FAIL++)); echo "  FAIL: $1 — $2"; }

# Create a testable version: strip set -e and the main call
make_testable() {
    local tmp
    tmp=$(mktemp)
    sed -e 's/^set -euo pipefail$/set -uo pipefail/' \
        -e 's/^main "\$@"$//' \
        "$SCRIPT" > "$tmp"
    echo "$tmp"
}

echo "=== install.sh tests ==="

# Test 1: script has valid bash syntax
echo "[syntax check]"
if bash -n "$SCRIPT" 2>&1; then
    pass "install.sh has valid bash syntax"
else
    fail "syntax check" "bash -n failed"
fi

# Test 2: error function exits non-zero with message
echo "[error function]"
testable=$(make_testable)
output=$(bash -c "
    source '$testable'
    error 'test message' 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if [ "$rc" -ne 0 ]; then
    if echo "$output" | grep -q "Error: test message"; then
        pass "error prints message and exits 1"
    else
        fail "error message format" "got: $output"
    fi
else
    fail "error should exit non-zero" "got exit 0"
fi

# Test 3: Linux x86_64 platform detection
echo "[Linux x86_64 detection]"
testable=$(make_testable)
output=$(bash -c "
    source '$testable'
    uname() {
        case \"\$1\" in
            -s) echo 'Linux' ;;
            -m) echo 'x86_64' ;;
        esac
    }
    curl() {
        if [[ \"\${2:-}\" == *api.github.com* ]]; then
            echo '{\"tag_name\": \"v0.2.3\"}'
        else
            echo 'FAKE_BINARY'
        fi
    }
    chmod() { :; }
    export BACKSCROLL_INSTALL_DIR=\$(mktemp -d)
    main 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if echo "$output" | grep -q "backscroll-linux-x86_64"; then
    pass "Linux x86_64 selects correct asset"
else
    fail "Linux x86_64 asset" "output: $output"
fi

# Test 4: Darwin arm64 platform detection
echo "[macOS arm64 detection]"
testable=$(make_testable)
output=$(bash -c "
    source '$testable'
    uname() {
        case \"\$1\" in
            -s) echo 'Darwin' ;;
            -m) echo 'arm64' ;;
        esac
    }
    curl() {
        if [[ \"\${2:-}\" == *api.github.com* ]]; then
            echo '{\"tag_name\": \"v0.2.3\"}'
        else
            echo 'FAKE_BINARY'
        fi
    }
    chmod() { :; }
    export BACKSCROLL_INSTALL_DIR=\$(mktemp -d)
    main 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if echo "$output" | grep -q "backscroll-macos-aarch64"; then
    pass "macOS arm64 selects correct asset"
else
    fail "macOS arm64 asset" "output: $output"
fi

# Test 5: Unsupported OS fails
echo "[unsupported OS rejection]"
testable=$(make_testable)
output=$(bash -c "
    source '$testable'
    uname() {
        case \"\$1\" in
            -s) echo 'FreeBSD' ;;
            -m) echo 'x86_64' ;;
        esac
    }
    main 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if [ "$rc" -ne 0 ]; then
    pass "unsupported OS exits non-zero"
else
    fail "unsupported OS" "expected failure, got exit 0"
fi

# Test 6: Unsupported Linux arch fails
echo "[unsupported arch rejection]"
testable=$(make_testable)
output=$(bash -c "
    source '$testable'
    uname() {
        case \"\$1\" in
            -s) echo 'Linux' ;;
            -m) echo 'aarch64' ;;
        esac
    }
    main 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if [ "$rc" -ne 0 ]; then
    pass "unsupported Linux arch exits non-zero"
else
    fail "unsupported arch" "expected failure, got exit 0"
fi

# Test 7: Custom install dir via BACKSCROLL_INSTALL_DIR
echo "[custom install dir]"
testable=$(make_testable)
CUSTOM_DIR=$(mktemp -d)
output=$(BACKSCROLL_INSTALL_DIR="$CUSTOM_DIR" bash -c "
    source '$testable'
    uname() {
        case \"\$1\" in
            -s) echo 'Linux' ;;
            -m) echo 'x86_64' ;;
        esac
    }
    curl() {
        local outfile='' prev='' arg
        for arg in \"\$@\"; do
            if [ \"\$prev\" = '-o' ]; then outfile=\"\$arg\"; fi
            prev=\"\$arg\"
        done
        if [[ \"\$*\" == *api.github.com* ]]; then
            echo '{\"tag_name\": \"v0.2.3\"}'
        elif [ -n \"\$outfile\" ]; then
            echo 'FAKE_BINARY' > \"\$outfile\"
        else
            echo 'FAKE_BINARY'
        fi
    }
    chmod() { :; }
    main 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if [ -f "$CUSTOM_DIR/backscroll" ]; then
    pass "installs to custom BACKSCROLL_INSTALL_DIR"
else
    fail "custom install dir" "binary not found in $CUSTOM_DIR"
fi
rm -rf "$CUSTOM_DIR"

# Test 8: Version tag extracted from API response
echo "[version extraction]"
testable=$(make_testable)
output=$(bash -c "
    source '$testable'
    uname() {
        case \"\$1\" in
            -s) echo 'Linux' ;;
            -m) echo 'x86_64' ;;
        esac
    }
    curl() {
        if [[ \"\${2:-}\" == *api.github.com* ]]; then
            echo '{\"tag_name\": \"v0.2.3\"}'
        else
            echo 'FAKE_BINARY'
        fi
    }
    chmod() { :; }
    export BACKSCROLL_INSTALL_DIR=\$(mktemp -d)
    main 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if echo "$output" | grep -q "v0.2.3"; then
    pass "extracts version v0.2.3 from API response"
else
    fail "version extraction" "output: $output"
fi

# Test 9: Empty tag_name fails
echo "[empty version fails]"
testable=$(make_testable)
output=$(bash -c "
    source '$testable'
    uname() {
        case \"\$1\" in
            -s) echo 'Linux' ;;
            -m) echo 'x86_64' ;;
        esac
    }
    curl() { echo '{}'; }
    main 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if [ "$rc" -ne 0 ]; then
    pass "empty version tag causes failure"
else
    fail "empty version" "expected failure, got exit 0"
fi

# --- Summary ---
echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
