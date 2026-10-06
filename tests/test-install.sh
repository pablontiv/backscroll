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
echo 'FAKE_BINARY' >"$FIXTURE_DIR/backscroll"
FIXTURE_TARBALL="$FIXTURE_DIR/asset.tar.gz"
tar -czf "$FIXTURE_TARBALL" -C "$FIXTURE_DIR" backscroll
export FIXTURE_TARBALL

pass() {
    ((PASS++))
    echo "  PASS: $1"
}
fail() {
    ((FAIL++))
    echo "  FAIL: $1 — $2"
}

core_presets_match() {
    local destination="$1" preset
    for preset in claude pi codex; do
        cmp -s "$INPUTS_DIR/$preset.inputs.toml" "$destination/$preset.inputs.toml" || return 1
    done
}

# Create a testable version: strip set -e and the main call
make_testable() {
    local tmp
    tmp=$(mktemp)
    sed -e 's/^set -euo pipefail$/set -uo pipefail/' \
        -e 's/^main "\$@"$//' \
        "$SCRIPT" >"$tmp"
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
    export BACKSCROLL_INSTALL_DIR=\$(mktemp -d)
    export BACKSCROLL_CONFIG_DIR=\$(mktemp -d)
    export BACKSCROLL_INPUTS_SOURCE_DIR='$INPUTS_DIR'
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
    export BACKSCROLL_INSTALL_DIR=\$(mktemp -d)
    export BACKSCROLL_CONFIG_DIR=\$(mktemp -d)
    export BACKSCROLL_INPUTS_SOURCE_DIR='$INPUTS_DIR'
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

# Test 10: BACKSCROLL_CONFIG_DIR controls input destination and wins over defaults
echo "[input preset install with config override]"
testable=$(make_testable)
CONFIG_DIR=$(mktemp -d)
HOME_DIR=$(mktemp -d)
XDG_DIR=$(mktemp -d)
output=$(BACKSCROLL_CONFIG_DIR="$CONFIG_DIR" BACKSCROLL_INPUTS_SOURCE_DIR="$INPUTS_DIR" \
    HOME="$HOME_DIR" XDG_CONFIG_HOME="$XDG_DIR" bash -c "
    unset BACKSCROLL_FORCE_INPUTS
    source '$testable'
    install_input_presets 'v0.2.3' 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if core_presets_match "$CONFIG_DIR/backscroll/inputs" &&
    [ ! -e "$HOME_DIR/.config/backscroll" ] &&
    [ ! -e "$XDG_DIR/backscroll" ]; then
    pass "BACKSCROLL_CONFIG_DIR installs Pi, Claude, and Codex only under its destination"
else
    fail "config override destination" "unexpected preset destination; output: $output"
fi
rm -rf "$CONFIG_DIR" "$HOME_DIR" "$XDG_DIR"

# Test 11: Without an override, Unix installs under HOME/.config and ignores XDG_CONFIG_HOME
echo "[Unix default config destination]"
testable=$(make_testable)
HOME_DIR=$(mktemp -d)
XDG_DIR=$(mktemp -d)
output=$(HOME="$HOME_DIR" XDG_CONFIG_HOME="$XDG_DIR" BACKSCROLL_INPUTS_SOURCE_DIR="$INPUTS_DIR" bash -c "
    unset BACKSCROLL_CONFIG_DIR BACKSCROLL_FORCE_INPUTS
    source '$testable'
    install_input_presets 'v0.2.3' 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if core_presets_match "$HOME_DIR/.config/backscroll/inputs" &&
    [ ! -e "$XDG_DIR/backscroll" ]; then
    pass "default installs Pi, Claude, and Codex under HOME/.config despite XDG_CONFIG_HOME"
else
    fail "Unix default config destination" "expected presets only under $HOME_DIR/.config; output: $output"
fi
rm -rf "$HOME_DIR" "$XDG_DIR"

# Test 12: Existing core presets are not overwritten by default
echo "[input preset skip existing]"
testable=$(make_testable)
CONFIG_DIR=$(mktemp -d)
mkdir -p "$CONFIG_DIR/backscroll/inputs"
printf '%s\n' "claude user edit" >"$CONFIG_DIR/backscroll/inputs/claude.inputs.toml"
printf '%s\n' "pi user edit" >"$CONFIG_DIR/backscroll/inputs/pi.inputs.toml"
printf '%s\n' "codex user edit" >"$CONFIG_DIR/backscroll/inputs/codex.inputs.toml"
output=$(BACKSCROLL_CONFIG_DIR="$CONFIG_DIR" BACKSCROLL_INPUTS_SOURCE_DIR="$INPUTS_DIR" bash -c "
    unset BACKSCROLL_FORCE_INPUTS
    source '$testable'
    install_input_presets 'v0.2.3' 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if grep -qxF "claude user edit" "$CONFIG_DIR/backscroll/inputs/claude.inputs.toml" &&
    grep -qxF "pi user edit" "$CONFIG_DIR/backscroll/inputs/pi.inputs.toml" &&
    grep -qxF "codex user edit" "$CONFIG_DIR/backscroll/inputs/codex.inputs.toml" &&
    [ "$(printf '%s\n' "$output" | grep -c 'exists, skipping')" -eq 3 ]; then
    pass "existing Pi, Claude, and Codex presets are preserved by default"
else
    fail "input preset preservation" "a core preset changed or was not skipped; output: $output"
fi
rm -rf "$CONFIG_DIR"

# Test 13: BACKSCROLL_FORCE_INPUTS=1 overwrites existing core presets
echo "[input preset force overwrite]"
testable=$(make_testable)
CONFIG_DIR=$(mktemp -d)
mkdir -p "$CONFIG_DIR/backscroll/inputs"
printf '%s\n' "claude user edit" >"$CONFIG_DIR/backscroll/inputs/claude.inputs.toml"
printf '%s\n' "pi user edit" >"$CONFIG_DIR/backscroll/inputs/pi.inputs.toml"
printf '%s\n' "codex user edit" >"$CONFIG_DIR/backscroll/inputs/codex.inputs.toml"
output=$(BACKSCROLL_CONFIG_DIR="$CONFIG_DIR" BACKSCROLL_INPUTS_SOURCE_DIR="$INPUTS_DIR" \
    BACKSCROLL_FORCE_INPUTS=1 bash -c "
    source '$testable'
    install_input_presets 'v0.2.3' 2>&1
") && rc=$? || rc=$?
rm -f "$testable"

if core_presets_match "$CONFIG_DIR/backscroll/inputs"; then
    pass "BACKSCROLL_FORCE_INPUTS=1 overwrites Pi, Claude, and Codex presets"
else
    fail "input preset force" "a core preset was not overwritten; output: $output"
fi
rm -rf "$CONFIG_DIR"

# --- Summary ---
echo ""
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
