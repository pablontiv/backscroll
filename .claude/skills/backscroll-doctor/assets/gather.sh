#!/usr/bin/env bash
# backscroll-doctor: gather raw diagnostic signal from backscroll's OWN indexed
# usage history. Synthesis + verification is the caller's job — this only mines.
#
# Usage: gather.sh <errors|gaps|usage|all>
set -uo pipefail

BS="${BACKSCROLL_BIN:-backscroll}"
# Pi reasoning blobs and harness chatter are pure noise — strip them.
NOISE='encrypted_content|pi-drive:observation|system-reminder|task-notification'
PER_QUERY="${BACKSCROLL_DOCTOR_LIMIT:-40}"

# emit LABEL MODE QUERY...  (MODE: tool = --content-type tool, text = prose)
emit() {
  local label="$1" mode="$2"; shift 2
  echo "### ${label}"
  local ctflag=()
  [ "$mode" = "tool" ] && ctflag=(--content-type tool)
  for q in "$@"; do
    "$BS" search "$q" --all-projects "${ctflag[@]}" --json 2>/dev/null \
      | jq -r --arg q "$q" '.[]? | "[\($q)] \(.source_path)\n\(.snippet)\n---"' 2>/dev/null \
      | rg -v "$NOISE" 2>/dev/null \
      | head -n "$PER_QUERY" || true
  done
  echo
}

errors() {
  emit "ERRORS (failed tool outputs)" tool \
    "database is locked" "SQLITE_BUSY" "auto-sync failed" "panic:" \
    "no such column" "no such table" "unknown command" "unknown flag" \
    "unknown shorthand flag" "fts5: syntax error" "malformed" "no results found"
}

gaps() {
  # Prose friction: read snippets and infer wishes/workarounds.
  emit "GAPS (prose friction & workarounds)" text \
    "backscroll" "had to grep" "fell back to" "would be nice" \
    "no way to" "doesn't support" "instead of backscroll"
}

usage() {
  # Invocation patterns: which subcommands/flags dominate, retries, tail-gap.
  emit "USAGE (invocation patterns)" tool \
    "backscroll search" "backscroll list" "backscroll read" \
    "backscroll status" "backscroll rebuild" "backscroll validate"
}

case "${1:-all}" in
  errors) errors ;;
  gaps)   gaps ;;
  usage)  usage ;;
  all)    errors; gaps; usage ;;
  *) echo "usage: gather.sh <errors|gaps|usage|all>" >&2; exit 2 ;;
esac
