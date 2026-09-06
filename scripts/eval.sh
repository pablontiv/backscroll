#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

BACKSCROLL_BIN="$WORK_DIR/backscroll"
EVAL_BIN="$WORK_DIR/recall-eval"
SOURCE_SHA="$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || printf unknown)"

(cd "$REPO_ROOT" && go build -o "$BACKSCROLL_BIN" ./cmd/backscroll)
(cd "$REPO_ROOT" && go build -o "$EVAL_BIN" ./scripts/recall-eval)
(cd "$REPO_ROOT" && "$EVAL_BIN" \
  --backscroll "$BACKSCROLL_BIN" \
  --eval-set "$REPO_ROOT/docs/eval/queries.toml" \
  --fixture-root "$REPO_ROOT/docs/eval/fixtures/recall" \
  --source-sha "$SOURCE_SHA" \
  "$@")
