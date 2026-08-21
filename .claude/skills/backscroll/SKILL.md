---
name: backscroll
description: "Use when starting feature, bug, test, refactor, or decision work that may have prior session history; when recalling what happened, which command failed, where something ran, or what was decided; and before considering raw coding-agent session files."
user-invocable: true
allowed-tools:
  - Bash
---

# Backscroll Recipe — Recall-First for Agents

Backscroll is the primary local episodic index for coding-agent work. Run it before starting feature, bug, test, refactor, or decision work that may have history. A hit is evidence from indexed rows. An empty result is only query/index uncertainty: it does not prove the event, file, or decision never existed.

Every operational command validates active manifests and attempts one incremental
sync before executing. Session, plan, and Markdown files are ingestion inputs;
SQLite is the perennial record used by search, list, patterns, status, and validate.
Use `--source-path` on search as a filter, paired with query text, for database-backed retrieval scoped to a known input path.

## 1) Preflight (required)

```bash
command -v backscroll >/dev/null 2>&1
backscroll status
```

If the binary is missing:

```bash
curl -fsSL https://raw.githubusercontent.com/pablontiv/backscroll/master/install.sh | bash
# Optional: copy shipped input presets after the binary is in PATH.
config_dir="${BACKSCROLL_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}}"
mkdir -p "$config_dir/backscroll/inputs"
cp -n inputs/claude.inputs.toml inputs/pi.inputs.toml inputs/opencode.inputs.toml inputs/decisions.inputs.toml "$config_dir/backscroll/inputs/"
```

## 2) When to Invoke (automatic triggers)

Invoke `/skill:backscroll` automatically for:

- Starting a feature: query the feature name and goal.
- Fixing a bug: query the exact error, symptom, or failing command.
- Writing tests: query the test subject and related module.
- Refactoring: query the module, interface, or previous pattern.
- Decision questions: query the decision topic and alternatives.
- Debugging execution: query command names, paths, flags, exit codes, and use the search command with `--content-type tool` and query text.

Spanish trigger equivalents include "ya lo hicimos", "qué hicimos con", "qué error dio", "dónde corrí", and "qué decidimos". Do not wait for explicit recall requests; missed lookup cost is rework and duplicate decisions.

## 3) Canonical input location

Input manifests are loaded only from:

```text
<config_dir>/backscroll/inputs/*.inputs.toml
```

where `<config_dir>` is the OS config directory, or `BACKSCROLL_CONFIG_DIR`. The app config file is for database and embedding settings, not ingestion sources.

## 4) Agent output contract

Use machine-readable, budgeted output:

- Robot mode on search emits `result_N_field=value` lines; search string values escape backslash as `\\`, carriage return as `\r`, and newline as `\n`.
- `--fields minimal`: returns `source_path`, `snippet`, `score`, `role`, and `timestamp`.
- `--fields full`: use only for a selected source-path drill.
- `--max-tokens <budget>`: declare and enforce the output budget.

Canonical retrieval:

```bash
# First query: cwd-inferred project scope.
backscroll search "QUERY" --robot --fields minimal --max-tokens 2000

# Second query: broaden scope explicitly if the first result set is empty or irrelevant.
backscroll search "QUERY" --all-projects --robot --fields minimal --max-tokens 2000

# Execution-shaped queries: commands, flags, errors, paths.
backscroll search "command or error" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
```

If an explicit project is needed, use a semantic project ID, not a filesystem path:

```bash
backscroll search "split FTS index" --project backscroll --robot --fields minimal --max-tokens 2000
```

Token budget guidance:

- Feature/bug/decision recall: 1500–2000 tokens.
- Cross-project lookup: 2000–3000 tokens.
- Tool/error investigation: 1000–1500 tokens; use literal strings of at least three characters.
- Default ceiling: `--max-tokens 2000` unless a higher budget is justified.

If output is truncated, treat it as evidence that more indexed data exists. Refine the query, selected source, or budget instead of abandoning the index.

## 5) Query patterns by use case

### Decision recovery

```bash
backscroll search "should we use RRF or vector" --all-projects --robot --fields minimal --max-tokens 2000
backscroll search "migration v7 reasoning index" --all-projects --robot --fields minimal --max-tokens 2000
```

### Error investigation

```bash
backscroll search "SQLITE_BUSY database is locked" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
backscroll search "exit code 1" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
```

### Feature work recovery

```bash
backscroll search "split FTS index" --robot --fields minimal --max-tokens 2000
backscroll search "backscroll search --robot" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
```

### Code pattern lookup

```bash
backscroll search "SearchEngine interface" --robot --fields minimal --max-tokens 1500
```

### Cross-project execution

```bash
backscroll search "go test" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500
```

## Search discipline (hard rules)

1. **Drill the top hit.** If a top-ranked result contains relevant decision keywords, inspect indexed rows from that returned path before dismissing it by age or hunting another session.

```bash
SOURCE_PATH="<result_N_source_path>"
backscroll search --text "$QUERY" --all-projects --source-path "$SOURCE_PATH" --robot --fields full --max-tokens 4000
```

2. **Use the artifact's vocabulary.** For transcripts, logs, reports, and pasted artifacts, query literal speaker names, boilerplate, IDs, exact errors, paths, and the artifact language. A translated or paraphrased query is secondary evidence only.

3. **A failed invocation is a syntax problem first.** For unknown flags, missing arguments, warnings, or path/session resolution errors, check current help, correct the command, and retry once. Never cite one malformed call as tool failure.

```bash
backscroll search --help
backscroll list --help
```

4. **Two empty searches prove nothing.** Before concluding content is absent from the index: retry with artifact-literal terms; broaden to `--all-projects`; if a path or UUID is known, drill down with search `--source-path` plus query text; rely on mandatory startup sync to refresh active manifests; then collect diagnostics and report the gap.

```bash
backscroll search "literal speaker or error" --all-projects --robot --fields minimal --max-tokens 2000
backscroll search --text "artifact literal" --all-projects --source-path "*SESSION-UUID*" --json --fields minimal --limit 1
backscroll search "literal speaker or error" --all-projects --content-type tool --robot --fields minimal --max-tokens 2000
backscroll search --text "$QUERY" --all-projects --source-path "*SESSION-UUID*" --json --fields full --max-tokens 4000
backscroll status
backscroll validate
```

Report the source path or UUID, literal probes, scopes used, and full diagnostic output as an indexing gap when the probe remains absent.

5. **Raw-file boundary.** `cat`, `jq`, Python, or filesystem session hunting is not a normal retrieval fallback. Do not use raw JSONL parsing, directory listings for session hunting, or direct file inspection unless the user explicitly authorizes indexing-bug diagnosis after you report the gap and the indexed commands attempted. Database-backed search with `--source-path` and query text is the supported drill-down path.

## 6) Degradation and troubleshooting

**Index stale, locked, or unhealthy:** preserve full command output. Do not pipe diagnostics through filters that hide warnings or suggestions.

```bash
backscroll status
backscroll validate
```

If a search warns about scope, content type, or compatibility, follow the hint and rerun a corrected current command once.

**No results:** follow the hard rules: literal artifact vocabulary, all-projects scope, source-path/UUID probe through mandatory startup sync, then status and validate. Report uncertainty; do not convert empty rows into proof of absence.

**Tool-query tokenizer limits:** the tool index uses a trigram tokenizer. Prefer exact flags, paths, command names, and error fragments of at least three characters, for example `"--content-type tool"`, `"go test"`, or `"BUSY"`.

**Output truncated by budget:** narrow the query or selected source path, or increase the declared budget. Truncation means the index had more data than fit.

**Database locked:** wait a few seconds and retry. If persistent, identify the locking process before further remediation.

```bash
backscroll status
```

**Explicit index or FTS corruption:** reserve rebuild for corruption repair after diagnostics indicate an index problem; it is not missing-input discovery.

```bash
backscroll rebuild
```

## 7) Token budget allocation for agents

| Use case | Budget | Notes |
|---|---:|---|
| Pre-work feature/bug recall | 2000 | First lookup in the session. |
| Refinement | 1000–1500 | Narrow query after first pass. |
| Tool/error investigation | 1000–1500 | Exact command, flag, path, or error. |
| Cross-project reference | 2000 | Wider scope. |
| Decision context | 1500–2000 | Decision prose can be longer. |

Agents should usually spend about 5000 tokens across three or four lookups. Refine before increasing budget.

```bash
backscroll search "query" --all-projects --robot --fields minimal --max-tokens 2000
```

## References

- CLI help: `backscroll search --help`, `backscroll list --help`, `backscroll patterns --help`, `backscroll annotate --help`.
- Deployable version check: `backscroll --version`; `backscroll status` also shows deployed build and index state.
- v1.4.0+ search behavior: split FTS indexes; `tool_fts` uses trigram tokenization for exact command/error matching, while `messages_fts` uses porter tokenization for prose. Select with `--content-type`.
- Diagnostic skill: `backscroll-doctor` audits index bugs, gaps, and enhancement candidates.

## Pattern discovery: census, not retrieval

Search answers “find what I can already name.” For discovery — “what recurs that nobody named?” — use census commands. BM25 pattern queries usually yield anecdotes, not counts.

| Question | Command |
|---|---|
| What errors recur? | `backscroll patterns --kind templates --min-support 3` |
| What breaks, and is it growing? | `backscroll patterns --kind failures --trend` |
| Where did the user correct me/us? | `backscroll patterns --kind corrections --min-confidence 0.6` |
| What workflows repeat? | `backscroll patterns --kind sequences --min-support 20 --min-length 3` |
| What runs most for a project? | `backscroll patterns --kind commands --project backscroll` |

Agent-grade census output:

```bash
backscroll patterns --kind corrections --pending --batch 50 --robot
backscroll patterns --kind commands --all-projects --robot
```

Interpret the complete table returned. The census did the counting; the agent's job is judgment, not sampling.

### Classification loop (resumable by construction)

```bash
backscroll patterns --kind corrections --pending --batch 50 --robot
backscroll annotate --uuid <u> --kind correction --label "<free-form>"
# Re-run fetch: labeled candidates vanish, so no loop state is needed.
```

Full doc: `docs/patterns.md`. Calibration gate before trusting confidences: `docs/eval/corrections-calibration.md`.
