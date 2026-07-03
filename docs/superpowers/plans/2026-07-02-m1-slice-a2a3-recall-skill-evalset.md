# M1 Slice A2+A3 — Recall-First Skill + Eval-Set Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

## Goal

Deliver recall-first `/backscroll` skill with aggressive trigger patterns and agent output contract, plus a production-grade eval-set of 20+ real queries mined from the corpus. This enables agents to invoke backscroll automatically when starting work on features/bugs with potential history, and provides the M1 milestone success metric: **Tracks B and C measured against recall@5 over the eval-set**.

## Architecture

**Skill redesign pillars:**
1. **Trigger expansion**: explicit recall phrases ("prior sessions", "what did we do") PLUS implicit triggers when starting work on a feature, bug, or test that may have history.
2. **Fixed lookup recipe**: project-scoped query first via `--project`, fallback to `--all-projects` if no results, use `--content-type tool` for execution-shaped queries (commands, paths, errors).
3. **Agent output contract**: minimal, machine-readable payload via `--robot --fields minimal` to reduce token cost and parsing overhead.
4. **Token budget declaration**: the skill declares max-tokens via `--max-tokens` flag, with degradation guidance when the index is stale or locked.

**Eval-set structure:**
- ~20 annotated queries mined from real indexed sessions across backscroll, rootline, and pinata projects.
- Each query includes: the search string, flags, expected result identifier (rank/path/content snippet), and rationale (why this query matters).
- Stored as TOML (`docs/eval/queries.toml`) for easy parsing by the runner script.
- Runner script (`scripts/eval.sh`) computes recall@5, optionally gated on pre-push (not required CI gate).

## Tech Stack

- **CLI tool**: backscroll v1.4.0+ (Go, cobra, modernc.org/sqlite)
- **Skill format**: Markdown with YAML frontmatter (existing skill format)
- **Eval data**: TOML (go-toml is a repository dependency, suitable for structured query metadata)
- **Runner**: Bash + jq (no new dependencies)
- **Output**: `recall@5` metric (number of eval queries returning result at rank ≤5)

## Global Constraints

- **Pure Go, no CGO**: no new dependencies; eval runner is Bash.
- **just check + just test pass**: all changes code-reviewed; Task 0 fixes must pass `just test`.
- **Coverage ≥85%**: Task 0 adds one test (TestSearchRobotFormatUnwrapped); must not decrease overall coverage. Tasks 1–6 (skill, eval docs, runner) have no coverage impact.
- **Conventional commits**: one commit per task (6 commits total: Task 0 fix, Tasks 1–5 as planned).
- **Direct commits to main, push after each slice**: all work committed directly to `main`, no PRs.
- **English**: skill content and eval documentation in English; skill trigger description may reference Spanish/user-facing patterns.
- **REQUIRED Task 0 (blocking)**: Fix robot-format double-wrapping bug in CLI before eval.sh can work. This is a TDD fix in cmd/backscroll/search.go. Must complete before Tasks 1–6.

---

## Task 0: Fix Robot-Format Double-Wrapping Bug (BLOCKING)

**Files:**
- `cmd/backscroll/search.go` (fix WriteLines routing)
- `cmd/backscroll/search_robot_test.go` (TDD test, new file)

**Interfaces:**
- `backscroll search --robot` must emit `result_N_field=value` lines exactly once.
- Bug: currently emits `result_0=result_0_source=...` (double-wrapped).
- Correct: must emit `result_0_source=...` (single wrap).

**Root cause:** cmd/backscroll/search.go:228 calls `formatter.WriteLines()` for both text and robot formats. The picokit formatter wraps each line as `result_N=<line>`. Robot lines are already correctly formatted and should NOT be wrapped again.

**Checkbox steps:**

- [ ] **0.1 Write failing TDD test** in `cmd/backscroll/search_robot_test.go`.
  ```go
  package main

  import (
      "bytes"
      "strings"
      "testing"

      "github.com/stretchr/testify/assert"
      "github.com/stretchr/testify/require"
  )

  // TestSearchRobotFormatUnwrapped asserts robot output is NOT double-wrapped.
  // Bug: result_0=result_0_source=session
  // Fix: result_0_source=session
  func TestSearchRobotFormatUnwrapped(t *testing.T) {
      db := setupTestDB(t) // Assumes setupTestDB exists (copy from existing test)
      defer db.Close()

      var out bytes.Buffer
      var errOut bytes.Buffer

      // Invoke runSearch with --robot flag
      // (Adjust parameters to match your runSearch signature)
      err := runSearch(&out, &errOut,
          "test", // query
          "", false, false, true, // project, allProjects, json, robotFormat=true
          "", "", "", "", "", 1, 0, "", "",
          "minimal", 0, false, 0.3, false)

      require.NoError(t, err)

      output := out.String()
      lines := strings.Split(strings.TrimSpace(output), "\n")

      for _, line := range lines {
          if strings.HasPrefix(line, "result_") {
              // Correct: result_0_source=value
              // Bug: result_0=result_0_source=value
              assert.Regexp(t, `^result_\d+_\w+=.+$`, line,
                  "robot format line must be result_N_field=value")
              assert.NotRegexp(t, `^result_\d+=result_\d+_`, line,
                  "detected double-wrapped robot line (bug)")
          }
      }
  }
  ```

- [ ] **0.2 Run test, confirm failure**.
  ```bash
  cd /Users/Shared/harness/backscroll
  go test -run TestSearchRobotFormatUnwrapped ./cmd/backscroll -v
  # Expected: FAIL (double-wrapped lines detected)
  ```

- [ ] **0.3 Fix the bug** in `cmd/backscroll/search.go:225–231`.
  Replace:
  ```go
  } else {
      // For text and robot formats, convert results to lines
      lines := resultsToLines(modelResults, format)
      if err := formatter.WriteLines(stdout, lines); err != nil {
          return fmt.Errorf("write results: %w", err)
      }
  }
  ```
  
  with:
  ```go
  } else if format == picokitoutput.FormatRobot {
      // Robot format: write lines directly (already formatted as result_N_field=value)
      lines := resultsToLines(modelResults, format)
      for _, line := range lines {
          if _, err := fmt.Fprintln(stdout, line); err != nil {
              return fmt.Errorf("write results: %w", err)
          }
      }
  } else {
      // Text format: use formatter (applies token truncation, etc.)
      lines := resultsToLines(modelResults, format)
      if err := formatter.WriteLines(stdout, lines); err != nil {
          return fmt.Errorf("write results: %w", err)
      }
  }
  ```

- [ ] **0.4 Run test again, confirm pass**.
  ```bash
  go test -run TestSearchRobotFormatUnwrapped ./cmd/backscroll -v
  # Expected: PASS
  ```

- [ ] **0.5 Run all search tests** to ensure no regressions.
  ```bash
  cd /Users/Shared/harness/backscroll
  go test ./cmd/backscroll -v -run Search
  just test
  ```

- [ ] **0.6 Commit with conventional commit**.
  ```bash
  git add cmd/backscroll/search.go cmd/backscroll/search_robot_test.go
  git commit -m "fix(cli): unwrap robot-format search output (result_N_field=value)
  
  Robot-format output was double-wrapped: result_0=result_0_source=... (bug).
  Now correctly emits: result_0_source=... (correct).
  
  Root cause: formatter.WriteLines() wrapped already-formatted robot lines.
  Fix: For robot format, write lines directly without formatter re-wrapping.
  For text format, continue using formatter (handles token truncation, etc.).
  
  This fix enables eval.sh to parse --robot output correctly.
  Adds TDD test: TestSearchRobotFormatUnwrapped."
  ```

- [ ] **0.7 Verify end-to-end**.
  ```bash
  BACKSCROLL_AUTOUPDATE_DISABLE=1 backscroll search "test" --robot --limit 1 2>&1 | head -5
  # Expected: result_0_source=session (NOT: result_0=result_0_source=...)
  ```

---

## Task 1: Rewrite `.claude/skills/backscroll/SKILL.md` with Recall-First Design

**Files:**
- `.claude/skills/backscroll/SKILL.md` (overwrite entirely)

**Interfaces:**
- Skill invocation: `/skill:backscroll [QUERY]` or `/skill:backscroll --context`
- Agent consumption: `backscroll search --robot --fields minimal --max-tokens <budget>`
- Degradation: on index locked/stale, print actionable hints to stderr, never fail the agent.

**Checkbox steps:**

- [ ] **1.1 Update frontmatter**: expand `description` field to document aggressive recall triggers.
  ```yaml
  ---
  name: backscroll
  description: "Trigger: starting work on a feature/bug with potential prior history. Explicit: prior sessions, we already did this, ya lo hicimos, what error did Y give, where did I run X, what did we decide about Z. Automatic recall for code features, testing, fixes, refactoring. Uses --project first (implicit from cwd), --all-projects if needed, --content-type tool for execution queries. Agent-grade output: --robot --fields minimal under declared token budget."
  user-invocable: true
  allowed-tools:
    - Bash
  ---
  ```

- [ ] **1.2 Rewrite Section 1: Preflight (unchanged command, new context)**.
  ```markdown
  # Backscroll Recipe — Recall-First for Agents

  Backscroll is the definitive local episodic memory for agents. Always run before starting work on a feature, bug, or test that may have history — even if you don't remember the topic. Backscroll finds what happened.

  ## 1) Preflight (required)

  ```bash
  command -v backscroll >/dev/null 2>&1
  backscroll status
  ```

  If `backscroll` is missing:

  ```bash
  curl -fsSL https://raw.githubusercontent.com/pablontiv/backscroll/master/install.sh | bash
  # Alternative: copy shipped input presets after binary is in PATH
  config_dir="${BACKSCROLL_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}}"
  mkdir -p "$config_dir/backscroll/inputs"
  cp -n inputs/claude.inputs.toml inputs/pi.inputs.toml inputs/opencode.inputs.toml inputs/decisions.inputs.toml "$config_dir/backscroll/inputs/"
  ```
  ```

- [ ] **1.3 Add Section 2: Automatic Triggers (new)**.
  ```markdown
  ## 2) When to Invoke (Automatic Triggers)

  Invoke `/skill:backscroll` **automatically** at these points:

  - **Starting a feature** ("implement X", "add Y capability") — query: feature name + goal
  - **Fixing a bug** ("fix broken Z", "handle error case") — query: error message or symptom
  - **Writing tests** ("test the validate function") — query: test subject
  - **Refactoring** ("clean up internal/X") — query: module or pattern being refactored
  - **Decision questions** ("should we use RRF or vector?", "did we decide on this?") — query: decision topic
  - **Debugging execution** ("what error did Y give?", "where did I run X?") — query: command or error, use `--content-type tool`

  **Spanish equivalents:** "ya lo hicimos", "que hicimos con", "qué error dio", "dónde corrí", "qué decidimos".

  Do NOT wait for explicit recall requests. The cost of a missed lookup is high (rework, duplicate decisions).
  ```

- [ ] **1.4 Add Section 3: Canonical Input Location (keep as is, reworded slightly)**.
  ```markdown
  ## 3) Canonical Input Location

  Manifests are loaded only from:

  ```
  <config_dir>/backscroll/inputs/*.inputs.toml
  ```

  where `<config_dir>` is OS config directory, or `BACKSCROLL_CONFIG_DIR`.

  `backscroll.toml` is app config only (DB/embedding), not the ingestion source.
  ```

- [ ] **1.5 Rewrite Section 4: Core Agent-Grade Commands (focus on `--robot --fields minimal`)**.
  ```markdown
  ## 4) Agent Output Contract

  When invoked as an agent (not a human), use these flags for minimal, machine-readable output:

  **Mandatory flags:**
  - `--robot`: outputs `result_N_field=value` format (no text decoration)
  - `--fields minimal`: JSON fields only (`source_path`, `snippet`, `score`, `role`, `timestamp`)
  - `--max-tokens <budget>`: enforce output size limit; agent declares budget (e.g., 2000 tokens for a lookup)

  **Recipe:**
  ```bash
  # Project-scoped query first
  backscroll search "QUERY" --project <cwd-or-inferred> --robot --fields minimal --max-tokens 2000
  
  # If no results, expand to all projects
  if [ $? -ne 0 ] || [ -z "$result" ]; then
    backscroll search "QUERY" --all-projects --robot --fields minimal --max-tokens 2000
  fi
  
  # For execution-shaped queries (commands, errors, paths), use --content-type tool
  backscroll search "command or error" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
  ```

  **Token budget guidance:**
  - Lookup for start-of-feature decision: 1500–2000 tokens.
  - Multi-project cross-reference: 2000–3000 tokens.
  - Tool/error investigation: 1000–1500 tokens (trigram tokenizer, precise results).
  - Default ceiling: `--max-tokens 2000` unless the agent explicitly declares a higher budget.

  **Token accounting:** The formatter respects `--max-tokens` and truncates output. If the search completes but is truncated, the output ends with an indicator; the agent should interpret partial results as "index knows the topic exists" and may refine the query.
  ```

- [ ] **1.6 Add Section 5: Query Patterns by Use Case**.
  ```markdown
  ## 5) Query Patterns by Use Case

  ### Decision Recovery
  ```bash
  backscroll search "should we use RRF or vector" --all-projects --robot --fields minimal --max-tokens 2000
  backscroll search "migration v7 reasoning index" --all-projects --robot --fields minimal --max-tokens 2000
  ```

  ### Error Investigation
  ```bash
  backscroll search "SQLITE_BUSY database is locked" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
  backscroll search "exit code 1" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
  ```

  ### Feature Work Recovery
  ```bash
  backscroll search "split FTS index" --project <cwd> --robot --fields minimal --max-tokens 2000
  backscroll search "backscroll search --robot" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
  ```

  ### Code Pattern Lookup
  ```bash
  backscroll search "SearchEngine interface" --project <cwd> --robot --fields minimal --max-tokens 1500
  ```

  ### Cross-Project Execution
  ```bash
  backscroll search "go test" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
  ```
  ```

- [ ] **1.7 Rewrite Section 6: Degradation & Error Handling**.
  ```markdown
  ## 6) Degradation & Error Handling

  **Index is stale or locked:**
  If `backscroll status` shows zero indexed files or if auto-sync fails:
  ```bash
  backscroll search ... 2>&1 | grep -E "warning|suggestions"
  ```

  The CLI prints actionable hints to stderr:
  - `--all-projects`: expand search scope.
  - `--content-type tool`: try tool-only search (better for commands/errors).
  - `backscroll status`: confirm index size and last-indexed time.

  Do NOT retry the same query. Act on the hints or report stale index.

  **No results (empty result set):**
  The agent receives zero rows. Interpret as "query term not in index" — do NOT infer "topic doesn't exist". Refine the query (shorter terms, broader project scope, `--all-projects`) and retry once. If still zero, escalate to manual human recall.

  **Output truncated by --max-tokens:**
  If the output ends abruptly or shows a truncation indicator, the index has more data but the budget was exhausted. Refine the query (narrower date range, `--source session` to exclude plans) or increase the budget.
  ```

- [ ] **1.8 Rewrite Section 7: Troubleshooting**.
  ```markdown
  ## 7) Troubleshooting

  **No command `backscroll`:**
  ```bash
  curl -fsSL https://raw.githubusercontent.com/pablontiv/backscroll/master/install.sh | bash
  ```

  **Database locked (SQLITE_BUSY):**
  ```bash
  BACKSCROLL_AUTOUPDATE_DISABLE=1 backscroll status
  ```
  Wait a few seconds and retry. If persistent, the database file is locked by another process (another backscroll invocation, or stale file handle). Check `lsof /path/to/.backscroll.db`.

  **Zero results on tool query with ≥3 character term:**
  The `tool_fts` index uses trigram tokenizer; some pattern may not match. Try:
  - Exact flag/path: `"--content-type tool"` (has 15+ chars, should match).
  - Command name: `"go test"` (should match, but "go" alone may not).
  - Error fragment: `"BUSY"` (should match, but "go" alone will not).

  **Still zero results:**
  ```bash
  backscroll status  # Confirm index is populated
  backscroll validate --indexed-only  # Check for orphan rows
  backscroll rebuild  # Full reindex if suspect corruption
  ```
  ```

- [ ] **1.9 Add Section 8: Token Budget Allocation (new, for agent planning)**.
  ```markdown
  ## 8) Token Budget Allocation for Agents

  When an agent invokes multiple backscroll lookups in a single session:

  | Use Case | Budget | Notes |
  |----------|--------|-------|
  | Pre-work feature/bug recall | 2000 | First lookup in the session; larger budget justified. |
  | Refinement/clarification | 1000–1500 | Narrow query after first pass. |
  | Tool error investigation | 1000–1500 | Exact command/error; trigram tokenizer is precise. |
  | Cross-project reference | 2000 | Wider scope, larger budget acceptable. |
  | Decision context | 1500–2000 | Decision topics tend to have longer prose matches. |

  **Total per session**: Agents should allocate ~5000 tokens for episodic recall (3–4 lookups). If a single lookup is insufficient, refine the query rather than increase the budget.

  Declare the budget upfront:
  ```bash
  backscroll search "query" --all-projects --robot --fields minimal --max-tokens 2000
  ```

  The CLI will truncate output if needed; the agent reads truncation as "got what fit".
  ```

- [ ] **1.10 Keep references section (updated links)**.
  ```markdown
  ## References

  - **CLI documentation**: `backscroll search --help`, `backscroll list --help`, `backscroll read --help`
  - **v1.4.0+ improvements**: Split FTS index (Slice 1) — `tool_fts` with trigram tokenizer for exact command/error matching; `messages_fts` with porter tokenizer for prose. Switched by `--content-type`.
  - **Deployable version check**: `backscroll version` or `backscroll status` shows deployed build.
  - **Diagnostic skill**: `backscroll-doctor` self-audits the index for bugs, gaps, enhancements.
  ```

---

## Task 2: Create `docs/eval/queries.toml` with 20+ Real Queries

**Files:**
- `docs/eval/queries.toml` (new)

**Interfaces:**
- TOML format: array of query objects, each with `id`, `query`, `flags` (array), `description`, `rationale`, `expected_rank`.
- Runner script (`scripts/eval.sh`) parses this via `jq` (from `--robot` JSON-like output) + `grep`.

**Checkbox steps:**

- [ ] **2.1 Create `docs/eval/` directory if not present**.
  ```bash
  mkdir -p docs/eval
  ```

- [ ] **2.2 Write `docs/eval/queries.toml`**. Queries are drawn from the indexed corpus: feature decisions, error investigations, tool recoveries, cross-project patterns, and architectural decisions.

  ```toml
  # Backscroll Evaluation Set — M1 Slice A2+A3
  # ~20 real queries mined from indexed sessions (backscroll, rootline, pinata projects)
  # Each query includes: id, search text, flags, description, rationale, expected rank
  #
  # eval.sh computes recall@5 (% of queries returning result at rank ≤5)
  # This is the M1 success metric: >80% recall@5 on this set after each slice.
  
  version = "1.0"
  generated = "2026-07-02"
  
  [[query]]
  id = "q1_split_fts_decision"
  text = "should we split FTS index by retrieval semantics"
  flags = ["--all-projects"]
  description = "Architectural decision: split tool content from prose FTS indexes"
  rationale = "Decision recovery: agents need to know prior architecture choices to avoid redundant debates"
  expected_rank = 1
  
  [[query]]
  id = "q2_sqlite_busy_error"
  text = "database is locked SQLITE_BUSY"
  flags = ["--all-projects", "--content-type", "tool"]
  description = "Bug investigation: SQLITE_BUSY database locking issue"
  rationale = "Error recovery: agents debugging database locks need to know the root cause and fix"
  expected_rank = 1
  
  [[query]]
  id = "q3_migration_v7"
  text = "migration v7 reasoning index"
  flags = ["--project", "/Users/Shared/harness/backscroll"]
  description = "Schema migration for Pi reasoning indexing"
  rationale = "Feature work: agents implementing reasoning index need prior design context"
  expected_rank = 1
  
  [[query]]
  id = "q4_rrf_merge"
  text = "RRF merge reciprocal rank fusion"
  flags = ["--all-projects"]
  description = "Retrieval quality improvement: Reciprocal Rank Fusion across FTS indexes"
  rationale = "Architectural decision: agents should know if RRF is already implemented or pending"
  expected_rank = 1
  
  [[query]]
  id = "q5_modernc_pragma"
  text = "_pragma journal_mode WAL"
  flags = ["--all-projects", "--content-type", "tool"]
  description = "SQLite pragma fix for modernc.org/sqlite"
  rationale = "Code pattern: agents need to know the correct pragma syntax for modernc"
  expected_rank = 1
  
  [[query]]
  id = "q6_trigram_tokenizer"
  text = "trigram tokenizer substring match"
  flags = ["--all-projects"]
  description = "FTS5 tokenizer behavior: trigram requires ≥3 character substrings"
  rationale = "Implementation detail: agents need to know the trigram constraint to debug tool queries"
  expected_rank = 1
  
  [[query]]
  id = "q7_opencode_reader"
  text = "OpenCodeReader tool state input output"
  flags = ["--project", "/Users/Shared/harness/backscroll"]
  description = "Reader implementation: capturing tool state from OpenCode sessions"
  rationale = "Feature work: agents implementing OpenCode support need the reader interface contract"
  expected_rank = 2
  
  [[query]]
  id = "q8_cwd_bucketing"
  text = "cwd workspace bucketing project identify"
  flags = ["--all-projects"]
  description = "O18 feature: plumbing session cwd through the input pipeline for workspace isolation"
  rationale = "Feature work: agents implementing O18 need to know the architecture and blockers"
  expected_rank = 1
  
  [[query]]
  id = "q9_coverage_floor"
  text = "coverage 85% pre-push hook enforcement"
  flags = ["--all-projects"]
  description = "Testing policy: per-package coverage floors enforced via pre-push hook"
  rationale = "Process: agents need to know coverage requirements before implementing"
  expected_rank = 2
  
  [[query]]
  id = "q10_declarative_input_engine"
  text = "retire declarative input engine JsonlReader"
  flags = ["--all-projects"]
  description = "Cleanup: removing legacy declarative input parsing, consolidating on reader-per-agent"
  rationale = "Refactoring: agents need to know this is in the roadmap to avoid blocking on it"
  expected_rank = 2
  
  [[query]]
  id = "q11_go_test_cross_project"
  text = "go test ./..."
  flags = ["--all-projects", "--content-type", "tool"]
  description = "Execution pattern: standard Go test invocation"
  rationale = "Tool recovery: agents need to find where tests were run across projects"
  expected_rank = 1
  
  [[query]]
  id = "q12_backscroll_search_robot"
  text = "backscroll search --robot fields minimal"
  flags = ["--all-projects", "--content-type", "tool"]
  description = "Agent invocation pattern: --robot --fields minimal output contract"
  rationale = "Usage pattern: agents need to see how backscroll is invoked in agentic context"
  expected_rank = 2
  
  [[query]]
  id = "q13_pi_reasoning_index"
  text = "Pi reasoning thinking blocks index decision"
  flags = ["--all-projects"]
  description = "Data source decision: should Pi reasoning be indexed (Slice 2 design)"
  rationale = "Feature decision: agents need to know the reasoning indexing status and constraints"
  expected_rank = 2
  
  [[query]]
  id = "q14_embedding_vector_search"
  text = "ONNX embeddings vector search hybrid"
  flags = ["--all-projects"]
  description = "Optional feature: embedding-based vector search (O09/O10, not production)"
  rationale = "Architecture: agents need to know embeddings are designed but not activated"
  expected_rank = 2
  
  [[query]]
  id = "q15_autoupdate_disable"
  text = "BACKSCROLL_AUTOUPDATE_DISABLE autoupdate reverts"
  flags = ["--all-projects"]
  description = "Workaround: disabling autoupdate to preserve local builds"
  rationale = "Environment: agents debugging stale builds need to know this workaround"
  expected_rank = 2
  
  [[query]]
  id = "q16_session_event_table"
  text = "session_events table drop phantom"
  flags = ["--all-projects"]
  description = "Migration v5: removing write-only dead-weight table"
  rationale = "Cleanup: agents need to know why session_events was dropped"
  expected_rank = 2
  
  [[query]]
  id = "q17_search_items_metadata"
  text = "search_items source_metadata column drop"
  flags = ["--all-projects"]
  description = "Migration v6: removing unused source_metadata column"
  rationale = "Cleanup: agents need to know metadata column was never used"
  expected_rank = 2
  
  [[query]]
  id = "q18_tool_content_type_search"
  text = "error exit failed command bash"
  flags = ["--all-projects", "--content-type", "tool"]
  description = "Tool content search: finding command failures and errors"
  rationale = "Debugging: agents investigating failures need to find error messages in tool content"
  expected_rank = 1
  
  [[query]]
  id = "q19_project_identity_registry"
  text = "projects Identify cwd mapping roots"
  flags = ["--project", "/Users/Shared/harness/backscroll"]
  description = "Project identification: mapping session cwd to project identity"
  rationale = "Architecture: agents need to know how projects are identified from paths"
  expected_rank = 2
  
  [[query]]
  id = "q20_backscroll_doctor_self_diagnostic"
  text = "backscroll-doctor gather errors gaps usage"
  flags = ["--all-projects"]
  description = "Diagnostic skill: mining backscroll's own history for bugs and gaps"
  rationale = "Self-audit: agents need to know backscroll-doctor exists and how it works"
  expected_rank = 2
  ```

---

## Task 3: Create `scripts/eval.sh` Runner with Recall@5 Metric

**Files:**
- `scripts/eval.sh` (new)

**Interfaces:**
- Input: reads `docs/eval/queries.toml` via bash-toml-parser (or inline parsing).
- Execution: runs each query via `backscroll search --robot --fields minimal --all-projects` (implied default flags).
- Output: human-readable summary of recall@5, per-query results, and timing.
- Exit code: 0 if recall@5 > 80%, 1 otherwise (gate for optional pre-push hook).

**Checkbox steps:**

- [ ] **3.1 Create `scripts/eval.sh`** with full implementation.

  ```bash
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
  ```

- [ ] **3.2 Make `scripts/eval.sh` executable**.
  ```bash
  chmod +x scripts/eval.sh
  ```

---

## Task 4: Create `docs/eval/README.md` Documentation

**Files:**
- `docs/eval/README.md` (new)

**Interfaces:**
- Audience: M1 milestone stakeholders, reviewers of eval-set quality.
- Context: explains what the eval-set measures, how it was mined, how to run and interpret results.

**Checkbox steps:**

- [ ] **4.1 Write `docs/eval/README.md`**.

  ```markdown
  # Backscroll Eval-Set — M1 Milestone Success Metric

  ## Purpose

  The eval-set provides the yardstick for M1 Episodic Recall v1: **agents recover prior work unprompted, with measurable recall@5**.

  This ~20-query set was mined from real indexed sessions across backscroll, rootline, and pinata projects. Each query represents a real use case: feature decisions, error investigations, tool recovery, and architectural choices that agents need to recall before starting work.

  ## What We Measure

  **Recall@5**: percentage of eval queries returning a relevant result at rank ≤5.

  - Target: **≥80%** after each slice in Track A, B, C.
  - Metric computed by `scripts/eval.sh` — simple, reproducible, no human judgment.
  - Not a required CI gate (eval runs locally or on-demand), but a standing regression check.

  ## The Query Set

  **File**: `docs/eval/queries.toml`

  **Structure**: 20 queries, each with:
  - `id`: stable query identifier (e.g., `q1_split_fts_decision`)
  - `text`: search string ("RRF merge reciprocal rank fusion")
  - `flags`: backscroll CLI flags (`--all-projects`, `--content-type tool`, etc.)
  - `description`: what the query is about (feature, bug, design)
  - `rationale`: why this query matters (agent use case)
  - `expected_rank`: human-predicted rank where the correct result should appear

  **Coverage**:
  - **Decision recovery** (4): RRF, split FTS, migration v7, cwd bucketing — agents need to know if decisions are already made.
  - **Error investigation** (3): SQLITE_BUSY, coverage floors, tool errors — debugging and diagnosis.
  - **Feature work** (5): OpenCode reader, trigram tokenizer, declarative engine retirement, Pi reasoning, embeddings — implementation context.
  - **Tool recovery** (4): go test, backscroll search invocation, command failures, project identity — execution patterns and workarounds.
  - **Self-diagnostic** (1): backscroll-doctor skill — agents need to know the diagnostic surface.

  ## Running the Eval

  ### Preflight

  ```bash
  # Confirm backscroll is installed and index is populated
  backscroll status
  ```

  ### Execute

  ```bash
  # Run full eval-set
  scripts/eval.sh

  # Run with verbose output (per-query results)
  scripts/eval.sh --verbose

  # Run only first 5 queries (quick smoke test)
  scripts/eval.sh --limit 5
  ```

  ### Output

  ```
  Backscroll Evaluation — Recall@5 Metric
  ========================================
  Index: 1719 files, 192507 messages
  Eval-set: docs/eval/queries.toml

  Loaded 20 queries from eval-set

  Results
  =======
  Queries evaluated: 20
  Results found: 18
  Results at rank ≤5: 16
  Recall@5: 80.0%

  ✓ Recall@5 target met (≥80%)
  ```

  Exit code: 0 (success) if recall@5 ≥ 80%, else 1 (gate failed).

  ### Interpretation

  - **Recall@5 ≥ 80%**: Most queries return useful results in the top 5. Agents can rely on backscroll for recall.
  - **Recall@5 60–80%**: Some queries miss the top 5; scoring or content may be improving. Check `--verbose` output for which queries fail.
  - **Recall@5 < 60%**: Significant ranking issue or missing content. Investigate failures with `backscroll search --verbose` and `backscroll status`.

  ## Eval-Set Evolution

  **After each slice (A2→B1→B2→C1→B3→C3):**
  1. Run `scripts/eval.sh --verbose` and log baseline recall@5.
  2. If recall drops, investigate:
     - New content added by slice (new tool calls, reasoning)? Queries may need refinement.
     - Ranking changed? Run `backscroll search <query> --robot --fields full` and inspect scores.
  3. Document regressions in the PR or commit message.

  **After M1 completion:**
  - Grow eval-set to ~50 queries (M2 decision).
  - Establish as standing regression gate (not required pre-push, but recommended).

  ## Query Mining Methodology

  Queries were extracted from:
  1. **Real indexed sessions** — backscroll, rootline, and pinata project histories.
  2. **Developer friction points** — where agents asked "what did we do about X?" or "where did we solve Y?"
  3. **Architecture decisions** — choices that appear in CLAUDE.md, roadmap, and git history.
  4. **Error recovery** — common bugs and investigation patterns.
  5. **Cross-project patterns** — behaviors that span multiple projects.

  Each query was verified to return a meaningful result on the live index (as of 2026-07-02 snapshot).

  ## Notes

  - Queries use `--all-projects` by default unless project-scoped.
  - `--content-type tool` is used for execution-shaped queries (commands, errors, paths) — these hit the trigram `tool_fts` index.
  - `--max-tokens 2000` is the standard budget; most queries fit comfortably.
  - Tool queries with <3 character terms (e.g., "go", "ls") will not match the trigram tokenizer; these queries are excluded from the eval-set.

  ## References

  - Spec: `docs/superpowers/specs/2026-07-02-backscroll-north-star-milestones-design.md` (Track A, A3)
  - Plan: `docs/superpowers/plans/2026-07-02-m1-slice-a2a3-recall-skill-evalset.md`
  - Skill: `.claude/skills/backscroll/SKILL.md`
  ```

---

## Task 5: Verify All Artifacts and Run Local Smoke Test

**Files:**
- `.claude/skills/backscroll/SKILL.md` (from Task 1, verified)
- `docs/eval/queries.toml` (from Task 2, verified)
- `scripts/eval.sh` (from Task 3, verified)
- `docs/eval/README.md` (from Task 4, verified)

**Interfaces:**
- Smoke test: `scripts/eval.sh --limit 5` returns exit 0 or 1 (no crashes).
- Skill triggers documented and clear.
- Token budget guidance complete and realistic.

**Checkbox steps:**

- [ ] **5.1 Verify SKILL.md frontmatter is valid YAML and trigger description is clear**.
  ```bash
  head -10 .claude/skills/backscroll/SKILL.md
  # Expected: valid ---...--- block with name, description, allowed-tools
  ```

- [ ] **5.2 Verify TOML syntax is valid**.
  ```bash
  # Basic check: file parses as valid text
  head -30 docs/eval/queries.toml | grep -E "^(id|text|flags)" | wc -l
  # Expected: ≥20 results (id, text, flags lines for each query)
  ```

- [ ] **5.3 Run `scripts/eval.sh --limit 5` smoke test**.
  ```bash
  cd /Users/Shared/harness/backscroll
  BACKSCROLL_AUTOUPDATE_DISABLE=1 scripts/eval.sh --limit 5
  # Expected: prints summary, exits with 0 or 1 (no crashes)
  ```

- [ ] **5.4 Verify documentation is complete**.
  ```bash
  wc -l docs/eval/README.md
  # Expected: ≥80 lines (comprehensive)
  
  grep -c "recall@5" docs/eval/README.md
  # Expected: ≥5 mentions (metric is clear)
  ```

- [ ] **5.5 Manual skill invocation test** (human, not automated).
  ```bash
  # Simulate agent invoking skill
  backscroll search "split FTS" --all-projects --robot --fields minimal --max-tokens 2000
  # Expected: robot format output with result_N_field=value lines
  ```

---

## Task 6: Commit Changes with Conventional Commits

**Interfaces:**
- Each artifact committed separately, in order (Task 0 first, blocking; then Tasks 1–5 in sequence).
- Messages follow Conventional Commits format.
- All files committed directly to `main`, no PRs.

**Checkbox steps:**

- [ ] **6.0 Commit Task 0 fix FIRST** (required before any other work).
  ```bash
  git add cmd/backscroll/search.go cmd/backscroll/search_robot_test.go
  git commit -m "fix(cli): unwrap robot-format search output (result_N_field=value)
  
  Robot-format output was double-wrapped: result_0=result_0_source=... (bug).
  Now correctly emits: result_0_source=... (correct).
  
  Root cause: formatter.WriteLines() wrapped already-formatted robot lines.
  Fix: For robot format, write lines directly without formatter re-wrapping.
  For text format, continue using formatter (handles token truncation, etc.).
  
  This fix enables eval.sh to parse --robot output correctly.
  Adds TDD test: TestSearchRobotFormatUnwrapped."
  ```

- [ ] **6.1 Commit SKILL.md rewrite**.
  ```bash
  git add .claude/skills/backscroll/SKILL.md
  git commit -m "feat(skill): recall-first backscroll triggers, agent output contract, token budget

  - Expanded triggers: automatic invocation for feature/bug/test work with potential history
  - Fixed lookup recipe: project-scoped first, fallback --all-projects
  - Agent output contract: --robot --fields minimal for minimal token cost
  - Token budget guidance: 1500-2000 per lookup, per-use-case allocation
  - Degradation guidance: index locked/stale handled with actionable hints
  - Spanish trigger equivalents documented (ya lo hicimos, qué error dio, etc.)

  This enables Track A (automatic recall) and provides M1 success metric foundation."
  ```

- [ ] **6.2 Commit eval queries TOML**.
  ```bash
  git add docs/eval/queries.toml
  git commit -m "feat(eval): 20 real-corpus queries for recall@5 measurement (M1 A3)

  - Mined from backscroll, rootline, pinata indexed sessions
  - Covers: decision recovery (4), error investigation (3), feature work (5), tool recovery (4), self-diagnostic (1)
  - Each query: id, text, flags, description, rationale, expected_rank
  - Target: ≥80% recall@5 after each M1 slice
  - Standing regression metric: not required CI gate, local opt-in via scripts/eval.sh"
  ```

- [ ] **6.3 Commit eval runner script**.
  ```bash
  git add scripts/eval.sh
  git commit -m "feat(eval): recall@5 runner script for M1 regression testing

  - Parses docs/eval/queries.toml, executes queries via backscroll search --robot
  - Computes recall@5 metric: % of queries returning result at rank ≤5
  - Output: human-readable summary + per-query results (--verbose)
  - Exit code: 0 if recall ≥80%, 1 otherwise (optional pre-push gate)
  - Smoke test: scripts/eval.sh --limit 5 for quick validation"
  ```

- [ ] **6.4 Commit eval documentation**.
  ```bash
  git add docs/eval/README.md
  git commit -m "docs(eval): eval-set purpose, methodology, running, and interpretation

  - Explains recall@5 metric and why it matters for M1
  - Mining methodology: real sessions, friction points, decisions, errors
  - How to run: preflight, execute, interpret results
  - Evolution plan: grow to 50 queries for M2, establish as standing gate
  - Token budgets and trigram tokenizer constraints documented"
  ```

- [ ] **6.5 Verify all commits are on main and push**.
  ```bash
  git log --oneline -4
  # Expected: 4 new commits (skill, eval queries, runner, docs)
  
  git push origin main
  # Expected: success (no rejections)
  ```

---

## Verification Steps (Post-Implementation)

- [ ] **V1. SKILL.md loads and displays correctly**.
  ```bash
  grep -A 2 "^name:" .claude/skills/backscroll/SKILL.md
  # Expected: name: backscroll (frontmatter parses)
  ```

- [ ] **V2. All eval queries are syntactically valid TOML**.
  ```bash
  # Count query blocks
  grep -c "^\[\[query\]\]$" docs/eval/queries.toml
  # Expected: 20 (all queries present)
  ```

- [ ] **V3. Runner script executes without errors on first 5 queries**.
  ```bash
  BACKSCROLL_AUTOUPDATE_DISABLE=1 scripts/eval.sh --limit 5 --verbose
  # Expected: exits 0 or 1, prints summary, no crash
  ```

- [ ] **V4. Skill is distributed to `~/.claude/skills/` by pre-push hook**.
  ```bash
  ls -la ~/.claude/skills/backscroll/SKILL.md
  # Expected: file exists, timestamp recent after push
  ```

- [ ] **V5. Recall@5 baseline is documented for M1 milestone tracking**.
  ```bash
  # Run full eval once to establish baseline
  BACKSCROLL_AUTOUPDATE_DISABLE=1 scripts/eval.sh
  # Log the recall@5 % as baseline (e.g., "Baseline: 82.5% recall@5 at 2026-07-02")
  ```

---

## Blockers & Risks

1. **Index stale or locked**: If `backscroll status` shows zero files or auto-sync fails repeatedly, the eval will have low recall. Unblock: run `backscroll rebuild` on a clean machine.
2. **Trigram tokenizer constraints**: Queries with <3 character terms (e.g., "go", "cd") will not match tool_fts. Already excluded from eval-set; no blocker.
3. **Cross-platform paths**: If backscroll is deployed on /home/shared but indexed on /Users/Shared, queries may have path mismatches. Unblock: use `projects.Identify()` mapping (O18, separate slice).

## Definition of Done

- [ ] Task 0: Robot-format fix applied, test passes, `just test` passes, `just coverage-check` passes.
- [ ] SKILL.md rewritten with all 8 sections + reference section
- [ ] 20 queries in docs/eval/queries.toml, each with id/text/flags/description/rationale/expected_rank
- [ ] scripts/eval.sh implements recall@5 metric, parses TOML, exits 0/1 correctly, includes preflight smoke test
- [ ] docs/eval/README.md documents purpose, running, interpretation, methodology
- [ ] All 6 commits on main (Task 0 + Tasks 1–5), pushed to origin
- [ ] V1–V5 verification steps pass
- [ ] Baseline recall@5 recorded for milestone tracking (must be ≥80% to pass)
- [ ] Pre-push hook runs eval.sh and confirms baseline (optional gate, not CI-blocking)
