---
name: backscroll
description: "Use when starting feature, bug, test, refactor, or decision work that may have prior session history; when recalling what happened, which command failed, where something ran, or what was decided; and before considering raw coding-agent session files."
user-invocable: true
allowed-tools:
  - Bash
---

# Backscroll Recipe — Recall-First for Agents

Backscroll is the primary local episodic index for coding-agent work and the source of indexed history. Run Backscroll before starting feature, bug, test, refactor, or decision work that may have history. A hit is evidence from indexed rows. An empty result is only query/index uncertainty: it does not prove the event, file, or decision never existed.

Every operational command validates active manifests and attempts one incremental
sync before executing. Session, plan, and Markdown files are ingestion inputs;
SQLite is the perennial record used by search, context, list, patterns, status, and validate.
Use `search` for discovery. Search publishes exact context identity with every result. For one selected result, use its opaque UUID when non-null; only when UUID is null, use its exact stored source path plus ordinal. Then use `context` for the immediate positional neighborhood. Both surfaces are database-backed.

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
cp -n inputs/claude.inputs.toml inputs/pi.inputs.toml inputs/opencode.inputs.toml inputs/decisions.inputs.toml inputs/codex.inputs.toml "$config_dir/backscroll/inputs/"
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
- `--robot --fields minimal`: emits `result_N_filepath`, nullable `result_N_uuid`, `result_N_ordinal`, `result_N_content` (bounded snippet), `result_N_score`, `result_N_role`, and `result_N_timestamp`.
- Minimal JSON publishes `source_path`, nullable `uuid`, `ordinal`, and `snippet`. Full JSON uses `FilePath`, nullable `UUID`, and `Ordinal`. Keep each selected result's identity fields together.
- `--fields full`: use only while discovery still needs richer search results; minimal mode already contains both context selector forms.
- `--max-tokens <budget>`: declare and enforce the output budget.
- Context JSON is one `anchor` / `records` / `truncated` / `omitted` envelope. Context robot output uses `anchor_*`, envelope, and zero-based `record_N_*` keys.

`backscroll list` without an explicit scope uses the project inferred from the current working directory. For global recovery or inventory, repeat with `backscroll list --all-projects`. An empty list does not prove indexed history is absent or lost.

Canonical retrieval:

```bash
# First query: cwd-inferred project scope.
backscroll search "QUERY" --robot --fields minimal --max-tokens 2000

# Second query: broaden scope explicitly if the first result set is empty or irrelevant.
backscroll search "QUERY" --all-projects --robot --fields minimal --max-tokens 2000

# Execution-shaped queries: commands, flags, errors, paths.
backscroll search "command or error" --all-projects --content-type tool --robot --fields minimal --max-tokens 1500

# Preferred when the selected result's UUID is non-null.
backscroll context --uuid "$UUID" --before 5 --after 5 --robot --max-tokens 2000

# Fallback only when that result's UUID is null.
backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 5 --after 5 --robot --max-tokens 2000
```

Assign `UUID`, or `SOURCE_PATH` and `ORDINAL`, from one selected search result. Never combine identity fields from different results or derive an ordinal from display order.

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

For explicit lexical term-overload recall, add `--relax` (for example `backscroll search --text '+violet handshake technique adaptation' --relax --robot --fields minimal --max-tokens 2000`); `+term`/quoted phrases stay protected, scope never widens, and relaxed results report `match_stage`/`dropped_terms`—this does not recover missing vocabulary.

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

1. **Drill the top hit.** If a top-ranked result contains relevant decision keywords, inspect its exact indexed neighborhood before dismissing it by age or hunting another session. Select one result explicitly and keep its published identity together. Treat UUIDs as opaque: if `uuid` is non-null, use it; only if it is null, use that result's exact `source_path` plus `ordinal`.

```bash
# UUID is non-null in the selected result.
backscroll context --uuid "$UUID" --before 5 --after 5 --robot --max-tokens 4000

# UUID is null in the selected result.
backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 5 --after 5 --robot --max-tokens 4000
```

If either selector returns `context_ambiguous`, do not guess or choose by role. Preserve the diagnostic, refine discovery, and prefer a non-null UUID from the intended search result when available. Context returns positional DB neighbors from the same stored source path, ordered by ordinal and then row ID; it does not select a conversational pair by user/assistant roles.

1. **Use the artifact's vocabulary.** For transcripts, logs, reports, and pasted artifacts, query literal speaker names, boilerplate, IDs, exact errors, paths, and the artifact language. A translated or paraphrased query is secondary evidence only.

2. **A failed invocation is a syntax problem first.** For unknown flags, missing arguments, warnings, or path/session resolution errors, check current help, correct the command, and retry once. Never cite one malformed call as tool failure.

```bash
backscroll search --help
backscroll context --help
backscroll list --help
```

1. **Two empty searches prove nothing.** Before concluding content is absent from the index: retry with artifact-literal terms; broaden to `--all-projects`; if exact UUID or source-path-plus-ordinal identity is known, query it with `context`; rely on mandatory startup sync to refresh active manifests; then collect diagnostics and report the gap.

```bash
backscroll search "literal speaker or error" --all-projects --robot --fields minimal --max-tokens 2000
backscroll search --text "artifact literal" --all-projects --source-path "*SESSION-UUID*" --json --fields minimal --limit 1
backscroll search "literal speaker or error" --all-projects --content-type tool --robot --fields minimal --max-tokens 2000
backscroll context --uuid "$UUID" --before 5 --after 5 --json --max-tokens 4000
backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 5 --after 5 --json --max-tokens 4000
backscroll status
backscroll validate
```

Report the source path or UUID, literal probes, scopes used, and full diagnostic output as an indexing gap when the probe remains absent.

1. **Raw-file boundary.** `cat`, `jq`, Python, or filesystem session hunting is not a normal retrieval fallback. Do not use raw JSONL parsing, directory listings for session hunting, or direct file inspection unless the user explicitly authorizes indexing-bug diagnosis after you report the gap and the indexed commands attempted. Database-backed `context` is the exact drill-down path; it reads perennial and recovered SQLite rows without raw fallback.

## 6) Degradation and troubleshooting

**Index stale, locked, or unhealthy:** preserve full command output. Do not pipe diagnostics through filters that hide warnings or suggestions.

```bash
backscroll status
backscroll validate
```

If a search warns about scope, content type, or compatibility, follow the hint and rerun a corrected current command once.

**No results:** follow the hard rules: literal artifact vocabulary, all-projects scope, then an exact context probe when published UUID or source-path-plus-ordinal identity exists. `context_not_found` and `context_ambiguous` are diagnostics, not permission to inspect raw files or select by role. Run status and validate, report uncertainty, and do not convert empty rows into proof of absence.

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
| --- | ---: | --- |
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

- CLI help: `backscroll search --help`, `backscroll context --help`, `backscroll list --help`, `backscroll patterns --help`, `backscroll annotate --help`.
- Deployable version check: `backscroll --version`; `backscroll status` also shows deployed build and index state.
- v1.4.0+ search behavior: split FTS indexes; `tool_fts` uses trigram tokenization for exact command/error matching, while `messages_fts` uses porter tokenization for prose. Select with `--content-type`.
- Diagnostic skill: `backscroll-doctor` audits index bugs, gaps, and enhancement candidates.

## Pattern discovery: census, not retrieval

Search answers “find what I can already name.” For discovery — “what recurs that nobody named?” — use census commands. BM25 pattern queries usually yield anecdotes, not counts.

| Question | Command |
| --- | --- |
| What errors recur? | `backscroll patterns --kind templates --min-support 3` |
| What breaks, and is it growing? | `backscroll patterns --kind failures --trend` |
| Where did the user correct me/us? | `backscroll patterns --kind corrections --origin human --min-confidence 0.6` |
| What workflows repeat? | `backscroll patterns --kind sequences --min-support 20 --min-length 3` |
| What runs most for a project? | `backscroll patterns --kind commands --project backscroll` |

Agent-grade census output:

```bash
backscroll patterns --kind corrections --origin human --pending --batch 50 --robot
backscroll patterns --kind commands --all-projects --robot
```

Interpret the complete table returned. The census did the counting; the agent's job is judgment, not sampling.

For corrections, `--origin human|assistant|system|automation|unknown` is opt-in,
valid only with `--kind corrections`, and filters before pagination. Use
`--origin human` whenever the question is about what the human said. Origin is
parser-backed from native structured records; never infer it from text or a
historical role. An indexed message without sufficient native evidence is
`unknown`. Startup reparse can enrich a historical `unknown` only while its
configured source remains available. Recovery can also enrich `unknown` by
merging a compatible duplicate with proven origin; contradictory proven origins
are rejected. With the flag, text adds `Origin: <value>`, robot adds
`result_N_origin=<value>`, and JSON
candidates add `"Origin":"<value>"`; without it, all three omit that field and
retain their previous shape.

### Classification loop (resumable by construction)

```bash
backscroll patterns --kind corrections --origin human --pending --batch 50 --robot

# Use the UUID only when its value is not empty or null.
if [ -n "${UUID:-}" ] && [ "$UUID" != "null" ]; then
  backscroll context --uuid "$UUID" --before 1 --after 1 --robot --max-tokens 2000
  backscroll annotate --uuid "$UUID" --kind correction --label "$LABEL"
else
  backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 1 --after 1 --robot --max-tokens 2000
  backscroll annotate --path "$SOURCE_PATH" --ordinal "$ORDINAL" --kind correction --label "$LABEL"
fi
# Re-run fetch: labeled candidates vanish, so no loop state is needed.
```

Assign `UUID`, `SOURCE_PATH`, and `ORDINAL` from one selected candidate and set `LABEL` to the intended free-form label. A usable UUID is non-null and non-empty. The historical patterns contract can expose a legacy row as `"UUID":""` in JSON or `result_N_uuid=` in robot output; treat either form like null and use the exact published `source_path` plus `ordinal` from that same candidate. Use `context` whenever a candidate already has exact identity; do not approximate its labeling window with ranked search. Context defaults to 5/5 positional DB records (maximum 50 each), does not filter neighbors by role, never reads raw provider files, caps each text at 4000 Unicode code points, and defaults to `--max-tokens 2000` (valid range 64–16384). `context_not_found`, `context_ambiguous`, and `context_budget_too_small` are structured, budget-exempt diagnostics. Record origin is parser-backed and may remain `unknown`.

Full docs: `docs/context.md` and `docs/patterns.md`. Calibration gate before trusting confidences: `docs/eval/corrections-calibration.md`.
