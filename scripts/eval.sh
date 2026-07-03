#!/usr/bin/env bash
set -euo pipefail

# Backscroll Evaluation Runner — M1 Slice A2+A3
# Computes recall@5 over the eval-set (docs/eval/queries.toml)
# Usage: scripts/eval.sh [--verbose] [--limit N]
# Exit: 0 if recall@5 >= 80%, 1 otherwise (gated, not required CI)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
EVAL_TOML="$REPO_ROOT/docs/eval/queries.toml"

VERBOSE=0
LIMIT=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --verbose) VERBOSE=1; shift ;;
    --limit) LIMIT="$2"; shift 2 ;;
    *) echo "Usage: $0 [--verbose] [--limit N]"; exit 1 ;;
  esac
done

# Check preflight: backscroll installed + index populated
if ! command -v backscroll &>/dev/null; then
  echo "❌ backscroll not found in PATH"
  exit 1
fi

status_json=$(BACKSCROLL_AUTOUPDATE_DISABLE=1 backscroll status --json 2>/dev/null || true)
indexed_files=$(echo "$status_json" | jq '.index.files_indexed // 0' 2>/dev/null || echo 0)
if [ "$indexed_files" -lt 1 ]; then
  echo "❌ Index appears empty (files_indexed=$indexed_files). Run 'backscroll rebuild' first."
  exit 1
fi

# Preflight: verify --robot format is NOT double-wrapped
robot_sample=$(BACKSCROLL_AUTOUPDATE_DISABLE=1 backscroll search "test" --robot --limit 1 2>&1 | head -3 || true)
if echo "$robot_sample" | grep -E "^result_0=result_0_" >/dev/null; then
  echo "❌ BLOCKER: --robot output is double-wrapped (bug in backscroll CLI)"
  echo "   Expected format: result_0_source=value"
  echo "   Actual format:   result_0=result_0_source=value"
  echo "   Fix: Apply Task 0 (fix robot-format double-wrapping in cmd/backscroll/search.go)"
  exit 1
fi

echo "Backscroll Evaluation — Recall@5 Metric"
echo "========================================"
echo "Index: $indexed_files files, $(echo "$status_json" | jq '.index.messages_indexed // 0' 2>/dev/null || echo '?') messages"
echo "Eval-set: $EVAL_TOML"
echo ""

# Parse queries from TOML
# Simple inline parser: extract [[query]] blocks and field lines
# LIMITATION: quoted paths with special chars (e.g., spaces, commas) unsupported in flags array.
# Workaround: escape manually or keep flag paths simple (common case: no special chars needed).
declare -a query_ids
declare -a query_texts
declare -a query_flags_str

query_count=0
current_id=""
current_text=""
current_flags=""

while IFS= read -r line; do
  # Skip comments and empty lines
  [[ "$line" =~ ^[[:space:]]*# ]] && continue
  [[ -z "$line" || "$line" =~ ^[[:space:]]*$ ]] && continue

  if [[ "$line" =~ ^\[\[query\]\]$ ]]; then
    # Save previous query if exists
    if [[ -n "$current_id" ]]; then
      query_ids+=("$current_id")
      query_texts+=("$current_text")
      query_flags_str+=("$current_flags")
      ((query_count++))
    fi
    current_id=""
    current_text=""
    current_flags=""
  elif [[ "$line" =~ ^id[[:space:]]*=[[:space:]]*\"(.+)\" ]]; then
    current_id="${BASH_REMATCH[1]}"
  elif [[ "$line" =~ ^text[[:space:]]*=[[:space:]]*\"(.+)\" ]]; then
    current_text="${BASH_REMATCH[1]}"
  elif [[ "$line" =~ ^flags[[:space:]]*= ]]; then
    # Extract array: flags = ["--project", "path"] → "--project" "path"
    flags_part="${line#*flags*=}"
    flags_part="${flags_part//[\[\]]/}"
    flags_part="${flags_part//,/ }"
    flags_part=$(echo "$flags_part" | sed 's/"//g')
    current_flags="$flags_part"
  fi
done < "$EVAL_TOML"

# Save last query
if [[ -n "$current_id" ]]; then
  query_ids+=("$current_id")
  query_texts+=("$current_text")
  query_flags_str+=("$current_flags")
  ((query_count++))
fi

if [ "$query_count" -lt 1 ]; then
  echo "❌ No queries found in $EVAL_TOML"
  exit 1
fi

echo "Loaded $query_count queries from eval-set"
if [ "$LIMIT" -gt 0 ] && [ "$LIMIT" -lt "$query_count" ]; then
  query_count="$LIMIT"
  echo "Limiting to first $LIMIT queries"
fi
echo ""

# Execute queries and compute recall@5
results_found=0
results_at_rank_5=0
declare -a result_details

for ((i = 0; i < query_count; i++)); do
  id="${query_ids[$i]}"
  text="${query_texts[$i]}"
  flags_str="${query_flags_str[$i]}"

  # Build command: backscroll search --robot --fields minimal + flags
  # If no --all-projects in flags, add it by default
  if [[ ! "$flags_str" =~ --all-projects ]]; then
    flags_str="--all-projects $flags_str"
  fi

  if [ "$VERBOSE" -eq 1 ]; then
    echo "[$((i+1))/$query_count] $id"
    echo "  Query: $text"
    echo "  Flags: $flags_str"
  fi

  # Execute search with robot format
  robot_output=$(BACKSCROLL_AUTOUPDATE_DISABLE=1 backscroll search "$text" $flags_str --robot --fields minimal --max-tokens 2000 2>&1 || true)

  # Extract rank from robot output: result_0_rank=<N>
  rank=$(echo "$robot_output" | grep "^result_0_rank=" | head -1 | cut -d= -f2)

  if [[ -n "$rank" ]] && [[ "$rank" =~ ^[0-9]+$ ]]; then
    ((results_found++))
    if [ "$rank" -le 5 ]; then
      ((results_at_rank_5++))
    fi
    if [ "$VERBOSE" -eq 1 ]; then
      echo "  ✓ Found at rank $rank"
    fi
    result_details+=("$id: rank=$rank")
  else
    if [ "$VERBOSE" -eq 1 ]; then
      echo "  ✗ No result (rank not found in output)"
    fi
    result_details+=("$id: NO RESULT")
  fi
done

echo ""
echo "Results"
echo "======="

if [ "$results_found" -gt 0 ]; then
  recall_at_5=$(awk "BEGIN {printf \"%.1f\", 100 * $results_at_rank_5 / $query_count}")
else
  recall_at_5="0"
fi

echo "Queries evaluated: $query_count"
echo "Results found: $results_found"
echo "Results at rank ≤5: $results_at_rank_5"
echo "Recall@5: $recall_at_5%"
echo ""

if [ "$VERBOSE" -eq 1 ]; then
  echo "Per-query results:"
  for detail in "${result_details[@]}"; do
    echo "  $detail"
  done
  echo ""
fi

# Exit code: 0 if recall >= 80%, 1 otherwise
if (( $(echo "$recall_at_5 >= 80" | bc -l) )); then
  echo "✓ Recall@5 target met (≥80%)"
  exit 0
else
  echo "✗ Recall@5 below target (<80%)"
  exit 1
fi
