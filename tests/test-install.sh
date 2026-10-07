#!/usr/bin/env bash
# Hermetic regression tests for install.sh.
set -uo pipefail

PASS=0
FAIL=0
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$ROOT/install.sh"
INPUTS_DIR="$ROOT/inputs"
FIXTURE_DIR="$(mktemp -d)"
trap 'rm -rf "$FIXTURE_DIR"' EXIT

cat >"$FIXTURE_DIR/backscroll" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = "--version" ]; then
  echo "backscroll fixture"
  exit 0
fi
printf '%s\n' "$*" >>"${BACKSCROLL_TEST_CALL_LOG:?}"
[ "${1:-}" = presets ] && [ "${2:-}" = install ]
shift 2
config_home="${XDG_CONFIG_HOME:-$HOME/.config}"
overwrite=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --config-home) config_home="$2"; shift 2 ;;
    --overwrite) overwrite=1; shift ;;
    *) exit 2 ;;
  esac
done
destination="$config_home/backscroll/inputs"
mkdir -p "$destination"
for name in claude.inputs.toml pi.inputs.toml decisions.inputs.toml opencode.inputs.toml codex.inputs.toml; do
  if [ -e "$destination/$name" ] && [ "$overwrite" != 1 ]; then
    continue
  fi
  cp "${BACKSCROLL_INPUTS_SOURCE_DIR:?}/$name" "$destination/$name"
done
SH
chmod +x "$FIXTURE_DIR/backscroll"
tar -czf "$FIXTURE_DIR/asset.tar.gz" -C "$FIXTURE_DIR" backscroll
export FIXTURE_TARBALL="$FIXTURE_DIR/asset.tar.gz"

pass() {
  PASS=$((PASS + 1))
  echo "  PASS: $1"
}

fail() {
  FAIL=$((FAIL + 1))
  echo "  FAIL: $1 - $2"
}

make_testable() {
  local output
  output="$(mktemp)"
  sed -e 's/^set -euo pipefail$/set -uo pipefail/' -e 's/^main "\$@"$//' "$SCRIPT" >"$output"
  echo "$output"
}

run_main() {
  local testable="$1" os="$2" arch="$3" install_dir="$4" home="$5" config_dir="$6" log="$7"
  BACKSCROLL_INSTALL_DIR="$install_dir" \
  BACKSCROLL_CONFIG_DIR="$config_dir" \
  BACKSCROLL_INPUTS_SOURCE_DIR="$INPUTS_DIR" \
  BACKSCROLL_TEST_CALL_LOG="$log" \
  HOME="$home" \
  bash -c "
    source '$testable'
    uname() {
      case \"\$1\" in
        -s) echo '$os' ;;
        -m) echo '$arch' ;;
      esac
    }
    curl() {
      local outfile='' previous='' argument
      for argument in \"\$@\"; do
        if [ \"\$previous\" = -o ]; then outfile=\"\$argument\"; fi
        previous=\"\$argument\"
      done
      if [[ \"\$*\" == *api.github.com* ]]; then
        echo '{\"tag_name\": \"v0.2.3\"}'
      elif [[ \"\$*\" == *releases/download/* ]] && [ -n \"\$outfile\" ]; then
        cp \"\$FIXTURE_TARBALL\" \"\$outfile\"
      else
        return 90
      fi
    }
    main
  "
}

presets_match() {
  local destination="$1" name
  for name in claude.inputs.toml pi.inputs.toml decisions.inputs.toml opencode.inputs.toml codex.inputs.toml; do
    cmp -s "$INPUTS_DIR/$name" "$destination/$name" || return 1
  done
}

echo "=== install.sh tests ==="

if bash -n "$SCRIPT"; then pass "install.sh has valid Bash syntax"; else fail "syntax" "bash -n failed"; fi

TESTABLE="$(make_testable)"
output="$(bash -c "source '$TESTABLE'; error 'test message'" 2>&1)" && rc=0 || rc=$?
if [ "$rc" -eq 1 ] && grep -q 'Error: test message' <<<"$output"; then pass "error reports and exits"; else fail "error" "$output"; fi
rm -f "$TESTABLE"

for platform in "Linux x86_64 linux_amd64" "Darwin arm64 darwin_arm64"; do
  read -r os arch asset <<<"$platform"
  TESTABLE="$(make_testable)"
  install_dir="$(mktemp -d)"; home="$(mktemp -d)"; config="$(mktemp -d)"; log="$(mktemp)"
  output="$(run_main "$TESTABLE" "$os" "$arch" "$install_dir" "$home" "$config" "$log" 2>&1)" && rc=0 || rc=$?
  if [ "$rc" -eq 0 ] && grep -q "backscroll_0.2.3_${asset}.tar.gz" <<<"$output"; then pass "$os $arch selects the release asset"; else fail "$os asset" "$output"; fi
  rm -rf "$TESTABLE" "$install_dir" "$home" "$config" "$log"
done

TESTABLE="$(make_testable)"
output="$(bash -c "source '$TESTABLE'; uname(){ [ \"\$1\" = -s ] && echo FreeBSD || echo x86_64; }; main" 2>&1)" && rc=0 || rc=$?
if [ "$rc" -ne 0 ]; then pass "unsupported systems fail"; else fail "unsupported system" "$output"; fi
rm -f "$TESTABLE"

TESTABLE="$(make_testable)"
install_dir="$(mktemp -d)"; home="$(mktemp -d)"; config="$(mktemp -d)"; log="$(mktemp)"
output="$(run_main "$TESTABLE" Linux x86_64 "$install_dir" "$home" "$config" "$log" 2>&1)" && rc=0 || rc=$?
if [ "$rc" -eq 0 ] && [ -x "$install_dir/backscroll" ]; then pass "the bootstrap installs the binary"; else fail "binary install" "$output"; fi
if grep -qxF "presets install --config-home $config" "$log" && presets_match "$config/backscroll/inputs"; then
  pass "the bootstrap invokes native presets with the explicit config home"
else
  fail "native preset command" "log=$(cat "$log")"
fi
rm -rf "$TESTABLE" "$install_dir" "$home" "$config" "$log"

TESTABLE="$(make_testable)"
install_dir="$(mktemp -d)"; home="$(mktemp -d)"; xdg="$(mktemp -d)"; log="$(mktemp)"
output="$(env -u BACKSCROLL_CONFIG_DIR BACKSCROLL_INSTALL_DIR="$install_dir" BACKSCROLL_INPUTS_SOURCE_DIR="$INPUTS_DIR" BACKSCROLL_TEST_CALL_LOG="$log" HOME="$home" XDG_CONFIG_HOME="$xdg" FIXTURE_TARBALL="$FIXTURE_TARBALL" bash -c "
  source '$TESTABLE'
  uname(){ [ \"\$1\" = -s ] && echo Linux || echo x86_64; }
  curl(){ local out='' prev='' arg; for arg in \"\$@\"; do [ \"\$prev\" = -o ] && out=\"\$arg\"; prev=\"\$arg\"; done; if [[ \"\$*\" == *api.github.com* ]]; then echo '{\"tag_name\": \"v0.2.3\"}'; else cp \"\$FIXTURE_TARBALL\" \"\$out\"; fi; }
  main
" 2>&1)" && rc=0 || rc=$?
if [ "$rc" -eq 0 ] && grep -qxF "presets install" "$log" && presets_match "$xdg/backscroll/inputs" && [ ! -e "$home/.config/backscroll" ]; then
  pass "the native command selects XDG_CONFIG_HOME when no override exists"
else
  fail "XDG config" "$output; log=$(cat "$log")"
fi
rm -rf "$TESTABLE" "$install_dir" "$home" "$xdg" "$log"

TESTABLE="$(make_testable)"
install_dir="$(mktemp -d)"; home="$(mktemp -d)"; config="$(mktemp -d)"; log="$(mktemp)"
mkdir -p "$config/backscroll/inputs"
printf 'operator content\n' >"$config/backscroll/inputs/claude.inputs.toml"
output="$(run_main "$TESTABLE" Linux x86_64 "$install_dir" "$home" "$config" "$log" 2>&1)" && rc=0 || rc=$?
if [ "$rc" -eq 0 ] && grep -qxF 'operator content' "$config/backscroll/inputs/claude.inputs.toml"; then pass "presets are preserved by default"; else fail "preset preservation" "$output"; fi
rm -rf "$install_dir"
mkdir "$install_dir"
BACKSCROLL_FORCE_INPUTS=1 output="$(BACKSCROLL_FORCE_INPUTS=1 run_main "$TESTABLE" Linux x86_64 "$install_dir" "$home" "$config" "$log" 2>&1)" && rc=0 || rc=$?
if [ "$rc" -eq 0 ] && presets_match "$config/backscroll/inputs" && tail -1 "$log" | grep -q -- '--overwrite'; then pass "the explicit force option maps to --overwrite"; else fail "preset overwrite" "$output; log=$(cat "$log")"; fi
rm -rf "$TESTABLE" "$install_dir" "$home" "$config" "$log"

echo
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
