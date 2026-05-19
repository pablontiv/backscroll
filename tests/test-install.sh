#!/usr/bin/env bash
# Unit tests for install.sh — validates structure and logic
set -uo pipefail

PASS=0
FAIL=0
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$SCRIPT_DIR/install.sh"
INPUTS_DIR="$SCRIPT_DIR/inputs"

FIXTURE_DIR="$(mktemp -d)"
trap 'rm -rf "$FIXTURE_DIR"' EXIT
echo 'FAKE_BINARY' > "$FIXTURE_DIR/backscroll"
FIXTURE_TARBALL="$FIXTURE_DIR/asset.tar.gz"
tar -czf "$FIXTURE_TARBALL" -C "$FIXTURE_DIR" backscroll
export FIXTURE_TARBALL

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

run_main_linux() {
    local testable install_dir config_dir
    testable="$1"
    install_dir="$2"
    config_dir="$3"
    BACKSCROLL_INSTALL_DIR="$install_dir" \
    BACKSCROLL_CONFIG_DIR="$config_dir" \
    BACKSCROLL_INPUTS_SOURCE_DIR="$INPUTS_DIR" \
    bash -c "
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
            elif [[ \"\$*\" == *releases/download/* ]] && [ -n \"\$outfile\" ]; then
                cp \"\$FIXTURE_TARBALL\" \"\$outfile\"
            elif [ -n \"\$outfile\" ]; then
                echo 'FAKE_BINARY' > \"\$outfile\"
            else
                echo 'FAKE_BINARY'
            fi
        }
        chmod() { :; }
        main 2>&1
    "
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
        local outfile='' prev='' arg
        for arg in \"\$@\"; do
            if [ \"\$prev\" = '-o' ]; then outfile=\"\$arg\"; fi
            prev=\"\$arg\"
        done
        if [[ \"\$*\" == *api.github.com* ]]; then
            echo '{\"tag_name\": \"v0.2.3\"}'
        elif [[ \"\$*\" == *releases/download/* ]] && [ -n \"\$outfile\" ]; then
            cp \"\$FIXTURE_TARBALL\" \"\$outfile\"
        elif [ -n \"\$outfile\" ]; then
            echo 'FAKE_BINARY' > \"\$outfile\"
        else
            echo 'FAKE_BINARY'
        fi
    }
    chmod() { :; }
    export BACKSCROLL_INSTALL_DIR=\$(mktemp -d)
    export BACKSCROLL_CONFIG_DIR=\$(mktemp -d)
    export BACKSCROLL_INPUTS_SOURCE_DIR='$INPUTS_DIR'
    main 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if echo "$output" | grep -q "backscroll_0.2.3_linux_amd64.tar.gz"; then
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
        local outfile='' prev='' arg
        for arg in \"\$@\"; do
            if [ \"\$prev\" = '-o' ]; then outfile=\"\$arg\"; fi
            prev=\"\$arg\"
        done
        if [[ \"\$*\" == *api.github.com* ]]; then
            echo '{\"tag_name\": \"v0.2.3\"}'
        elif [[ \"\$*\" == *releases/download/* ]] && [ -n \"\$outfile\" ]; then
            cp \"\$FIXTURE_TARBALL\" \"\$outfile\"
        elif [ -n \"\$outfile\" ]; then
            echo 'FAKE_BINARY' > \"\$outfile\"
        else
            echo 'FAKE_BINARY'
        fi
    }
    chmod() { :; }
    export BACKSCROLL_INSTALL_DIR=\$(mktemp -d)
    export BACKSCROLL_CONFIG_DIR=\$(mktemp -d)
    export BACKSCROLL_INPUTS_SOURCE_DIR='$INPUTS_DIR'
    main 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if echo "$output" | grep -q "backscroll_0.2.3_darwin_arm64.tar.gz"; then
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
CONFIG_DIR=$(mktemp -d)
output=$(run_main_linux "$testable" "$CUSTOM_DIR" "$CONFIG_DIR") && rc=$? || rc=$?
rm -f "$testable"

if [ -f "$CUSTOM_DIR/backscroll" ]; then
    pass "installs to custom BACKSCROLL_INSTALL_DIR"
else
    fail "custom install dir" "binary not found in $CUSTOM_DIR"
fi
rm -rf "$CUSTOM_DIR" "$CONFIG_DIR"

# Test 8: Version tag extracted from API response
echo "[version extraction]"
testable=$(make_testable)
INSTALL_DIR=$(mktemp -d)
CONFIG_DIR=$(mktemp -d)
output=$(run_main_linux "$testable" "$INSTALL_DIR" "$CONFIG_DIR") && rc=$? || rc=$?
rm -f "$testable"

if echo "$output" | grep -q "v0.2.3"; then
    pass "extracts version v0.2.3 from API response"
else
    fail "version extraction" "output: $output"
fi
rm -rf "$INSTALL_DIR" "$CONFIG_DIR"

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

# Test 10: BACKSCROLL_CONFIG_DIR controls input destination
echo "[input preset install with config override]"
testable=$(make_testable)
INSTALL_DIR=$(mktemp -d)
CONFIG_DIR=$(mktemp -d)
output=$(run_main_linux "$testable" "$INSTALL_DIR" "$CONFIG_DIR") && rc=$? || rc=$?
rm -f "$testable"

if [ -f "$CONFIG_DIR/backscroll/inputs/claude.inputs.toml" ]; then
    pass "installs input presets under BACKSCROLL_CONFIG_DIR/backscroll/inputs"
else
    fail "input preset install" "preset not found in $CONFIG_DIR/backscroll/inputs; output: $output"
fi
rm -rf "$INSTALL_DIR" "$CONFIG_DIR"

# Test 11: Existing input presets are not overwritten by default
echo "[input preset skip existing]"
testable=$(make_testable)
CONFIG_DIR=$(mktemp -d)
mkdir -p "$CONFIG_DIR/backscroll/inputs"
echo "user edit" > "$CONFIG_DIR/backscroll/inputs/claude.inputs.toml"
output=$(BACKSCROLL_CONFIG_DIR="$CONFIG_DIR" BACKSCROLL_INPUTS_SOURCE_DIR="$INPUTS_DIR" bash -c "
    source '$testable'
    install_input_presets 'v0.2.3' 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if grep -q "user edit" "$CONFIG_DIR/backscroll/inputs/claude.inputs.toml" && echo "$output" | grep -q "exists, skipping"; then
    pass "existing input preset is skipped by default"
else
    fail "input preset skip" "file or output did not show skip; output: $output"
fi
rm -rf "$CONFIG_DIR"

# Test 12: BACKSCROLL_FORCE_INPUTS=1 overwrites existing input presets
echo "[input preset force overwrite]"
testable=$(make_testable)
CONFIG_DIR=$(mktemp -d)
mkdir -p "$CONFIG_DIR/backscroll/inputs"
echo "user edit" > "$CONFIG_DIR/backscroll/inputs/claude.inputs.toml"
output=$(BACKSCROLL_CONFIG_DIR="$CONFIG_DIR" BACKSCROLL_INPUTS_SOURCE_DIR="$INPUTS_DIR" BACKSCROLL_FORCE_INPUTS=1 bash -c "
    source '$testable'
    install_input_presets 'v0.2.3' 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if grep -q "id = \"claude\"" "$CONFIG_DIR/backscroll/inputs/claude.inputs.toml"; then
    pass "BACKSCROLL_FORCE_INPUTS=1 overwrites existing preset"
else
    fail "input preset force" "preset was not overwritten; output: $output"
fi
rm -rf "$CONFIG_DIR"

# Test 13: Linux default config dir honors XDG_CONFIG_HOME
echo "[Linux config dir resolution]"
testable=$(make_testable)
XDG_DIR=$(mktemp -d)
output=$(XDG_CONFIG_HOME="$XDG_DIR" HOME="$(mktemp -d)" bash -c "
    source '$testable'
    uname() { echo 'Linux'; }
    get_config_dir
") && rc=$? || rc=$?
rm -f "$testable"

if [ "$output" = "$XDG_DIR" ]; then
    pass "Linux config dir uses XDG_CONFIG_HOME"
else
    fail "Linux config dir" "expected $XDG_DIR, got $output"
fi
rm -rf "$XDG_DIR"

# Test 14: macOS default config dir uses Application Support
echo "[macOS config dir resolution]"
testable=$(make_testable)
HOME_DIR=$(mktemp -d)
output=$(HOME="$HOME_DIR" bash -c "
    source '$testable'
    uname() { echo 'Darwin'; }
    get_config_dir
") && rc=$? || rc=$?
rm -f "$testable"

if [ "$output" = "$HOME_DIR/Library/Application Support" ]; then
    pass "macOS config dir uses ~/Library/Application Support"
else
    fail "macOS config dir" "expected $HOME_DIR/Library/Application Support, got $output"
fi
rm -rf "$HOME_DIR"

# --- Summary ---
echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
