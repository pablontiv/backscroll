---
name: backscroll
description: "Trigger: starting work on a feature/bug with potential prior history. Explicit: prior sessions, we already did this, ya lo hicimos, what error did Y give, where did I run X, what did we decide about Z. Automatic recall for code features, testing, fixes, refactoring. Uses --project first (implicit from cwd), --all-projects if needed, --content-type tool for execution queries. Agent-grade output: --robot --fields minimal under declared token budget."
user-invocable: true
allowed-tools:
  - Bash
---

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

## 3) Canonical Input Location

Manifests are loaded only from:

```
<config_dir>/backscroll/inputs/*.inputs.toml
```

where `<config_dir>` is OS config directory, or `BACKSCROLL_CONFIG_DIR`.

`backscroll.toml` is app config only (DB/embedding), not the ingestion source.

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

## References

- **CLI documentation**: `backscroll search --help`, `backscroll list --help`, `backscroll read --help`
- **v1.4.0+ improvements**: Split FTS index (Slice 1) — `tool_fts` with trigram tokenizer for exact command/error matching; `messages_fts` with porter tokenizer for prose. Switched by `--content-type`.
- **Deployable version check**: `backscroll version` or `backscroll status` shows deployed build.
- **Diagnostic skill**: `backscroll-doctor` self-audits the index for bugs, gaps, enhancements.
