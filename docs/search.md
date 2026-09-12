---
estado: Completed
---
# Search Engine

The search command performs full-text search across all indexed sessions using BM25 relevance ranking. Results include highlighted snippets showing where the query matched. Results are always best-first: rank 1 is the strongest match. For a `--content-type` search the `Score`/`score` field is the raw FTS5 `bm25()` value, which is zero or negative with a more negative value meaning a better match, so rows are ordered by ascending score with row id as a deterministic tiebreaker. An unfiltered search merges the prose and tool indexes by rank position and reports the positive Reciprocal Rank Fusion score instead, where higher is better. `--source-path` is a filter: every executable search example must include positional query text or `--text <query>`.

## CLI Usage

```bash
backscroll search "migration plan"
backscroll search "error handling" --project "backscroll"
backscroll search "architecture" --json
backscroll search "deployment" --robot --max-tokens 2000
backscroll search "refactor" --fields full
backscroll search "artifact literal" --source-path "*/session.jsonl" --robot
```

### Flags

| Flag | Description |
| ------ | ------------- |
| `--project <NAME>` | Filter results to a specific project |
| `--json` | Output as a JSON array |
| `--robot` | Output compact `result_N_field=value` lines |
| `--fields minimal\|full` | Field set to include (default: `minimal`) |
| `--max-tokens <N>` | Approximate token limit for total output |
| `--source-path <PATH_OR_PATTERN>` | Filter a normal text query by indexed `source_path`; exact paths or `*`/SQL `LIKE` patterns |
| `--relax` | Opt in to bounded lexical term dropping after zero rows; all scope filters stay fixed |

## Output Formats

### Text (default)

Human-readable output with terminal bold for match highlights. Each result uses the exact text-layout envelope emitted by the CLI:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Rank: 1 | Source: session | Role: assistant | Score: -3.63
Path: /home/user/.claude/projects/backscroll/sessions/abc123/session.jsonl
...the migration plan involves three phases...
```

Match markers (`>>>` and `<<<` in the raw snippet) are rendered as bold text in the terminal.

### JSON

`--json` emits one JSON array. With `--fields minimal`:

```json
[
  {"source_path": "~/.claude/.../session.jsonl", "snippet": "...matched text...", "score": -3.63, "role": "assistant", "timestamp": "2026-08-20T12:34:56Z"}
]
```

With `--fields full`, ordinary results encode the existing `models.SearchResult` fields in their Go names (PascalCase):

```json
[
  {
    "Source": "session",
    "Role": "assistant",
    "Content": "...matched text...",
    "FilePath": "~/.claude/.../session.jsonl",
    "Timestamp": "2026-08-20T12:34:56Z",
    "SessionID": "",
    "ProjectPath": "backscroll",
    "Score": -3.63,
    "Tags": null,
    "ContentType": "text",
    "Rank": 1
  }
]
```

Ordinary full-mode fields are: `Source`, `Role`, `Content`, `FilePath`, `Timestamp`, `SessionID`, `ProjectPath`, `Score`, `Tags`, `ContentType`, and `Rank`. `ProjectPath` is a legacy field name; its value is the project identifier (for example `backscroll` or `myproj`), not a filesystem path. Minimal mode uses the snake_case payload (`source_path`, `snippet`, `score`, `role`, `timestamp`). After opt-in relaxation, both field sets additionally include `match_stage` and `dropped_terms`; ordinary results omit these keys.

### Robot

Robot mode emits deterministic `result_N_field=value` lines. Like JSON,
`--fields minimal` is the default and uses the bounded search snippet:

```
result_0_filepath=/home/user/.claude/projects/example/session.jsonl
result_0_content=bounded matched snippet
result_0_score=-3.63
result_0_role=assistant
result_0_timestamp=2026-08-20T12:34:56Z
```

Use `--fields full` when the consumer needs the complete indexed content and
metadata:

```
result_0_source=session
result_0_role=assistant
result_0_filepath=/home/user/.claude/projects/example/session.jsonl
result_0_content=complete content with escaped newlines
result_0_project=backscroll
result_0_content_type=text
result_0_timestamp=2026-08-20T12:34:56Z
result_0_score=-3.63
result_0_rank=1
```

No ANSI escape codes. Search robot string values escape backslash as `\\`, carriage return as `\r`, and newline as `\n`, keeping each field on one line for context windows.

## Token Limiting

The `--max-tokens` flag applies Picokit's approximate token estimator (word count
multiplied by 1.3) to the total output. Once the next complete result would
exceed the limit, search stops and emits a parseable omission record when that
record fits the remaining budget:

```
result_3_truncated=true
result_3_omitted=7
```

For robot output, the estimator is applied once to the complete escaped payload,
including all fields and the omission record; estimates rounded independently per
line or per result are not added together. Whole trailing results may be removed
to make room for the omission record. If even that record cannot fit, stdout is
empty. `--max-tokens 0` keeps output unlimited.

This is useful when feeding results into context-limited tools.

```bash
backscroll search "decisions" --robot --max-tokens 4000
backscroll search --text "$QUERY" --source-path "$SOURCE_PATH" --robot --fields full --max-tokens 4000
```

The limit is approximate — it will not truncate a result mid-output, but will stop before starting a result that would exceed the budget.

## Query-Echo Handling

Unfiltered search removes direct Backscroll search-tool calls and their
proven paired result rows before merging tool and prose rankings.
This prevents retrieval commands and copied search output from crowding out
historical prose. The same rule applies to both candidate streams, including
after an FTS rebuild.

The command boundary is the canonical direct invocation whose raw command text
starts with the two bare tokens `backscroll` and `search`, recognized per reader
from the raw tool input before serialization:

- Claude: a `Bash` tool call (`Bash command=backscroll search --text needle`;
  tool-name matching is case-insensitive), paired to its `tool_result` by
  `tool_use_id` within the source file.
- Codex: an `exec_command` call whose `cmd` is that command, or a `shell` call
  whose argv is exactly `[<shell>, "-c" | "-lc", "backscroll search --text needle"]` (Codex's
  own direct form, three elements, no other flags), paired to its
  `function_call_output` by `call_id` within the rollout.
- OpenCode: a `bash` tool part whose `state.input.command` is that command;
  the part carries both the input and output rows, so both are marked.
- Pi: a `bash` `toolCall` is recognized by the same text; Pi tool results are not
  indexed, so there is no paired output row.

Pairing is always by identity, never by adjacency or the appearance of
robot/JSON output. Explicit `--content-type tool` searches still return both
commands and results. Absolute paths and wrappers such as
`/path/backscroll search --text needle`,
`env backscroll search --text needle`, and, inside a Claude or OpenCode
command string or a Codex `cmd`, `bash -lc "backscroll search --text needle"`
remain ordinary tool results; the Codex argv form above is the only shell form
recognized, and only in that reader. Text that mentions Backscroll and
unrelated tool commands are unchanged. There is no opt-in flag or shell
parsing, query relaxation, or whole-session exclusion.

Migration v15 adds nullable `search_echo` provenance. Already-indexed Claude
session rows with surviving sources reparse through the existing bounded
incremental backfill, even when hashes and file metadata match. This updates
provenance without replacing perennial IDs or stored text. Codex and OpenCode
rows use the UUID-less per-file reload path. An index that stored those calls as
`search_echo=0` before their readers marked echoes re-enters the same bounded
replay while the surviving source still has a serialized direct search call;
paired outputs are marked by identity on that reparse, not by output shape.
While such a source awaits replay, the query-time exclusion already keeps its
zero-valued call rows out of unfiltered result pages and unfiltered `--relax`
IDF counting, recognizing the same serialized forms: the `bash`/`exec_command`
three-token prefixes (matched in SQL) and the Codex `shell` argv form (matched
by decoding the JSON-encoded argv, since its separator byte-sequences are
unbounded for SQL pattern matching).
Subsequent source expiry, `rebuild`, and supported canonical recovery preserve
proven pairing evidence. The general extraction epoch is unchanged.

**Limits:** outputs whose source files expired before their reader recorded
provenance (Claude rows indexed before v15, Codex/OpenCode rows indexed before
their readers marked echoes) lack reliable call linkage and remain searchable;
output shape is never guessed, and this is an accepted, documented limit rather
than a heuristic target. Unpaired outputs, absolute-path calls, and env/shell
wrappers other than the Codex argv form remain unchanged. This is a bounded
improvement to issue #64, not a claim to eliminate every possible echo or
repair unrelated BM25 ordering.

## Query Sanitization

User queries are automatically sanitized before being passed to the FTS5 engine:

1. **Dynamic stopword removal** — Sync stores up to 1000 vocabulary terms ordered by document frequency in `dynamic_stopwords`; the implementation does not apply a percentage threshold. The sanitizer compares lowercased input tokens against that Porter-stemmed vocabulary, so raw inflected words may not match their stored stems. This existing behavior is unchanged by the opt-in feature.
2. **Literal quoting** — Remaining tokens are wrapped in double quotes so special characters (hyphens, colons, parentheses, FTS5 operators like `AND`/`OR`/`NOT`) are treated as literal search terms.
3. **Prefix matching** — Each token gets an FTS5 prefix `*` suffix (e.g., "crash" matches "crashloopbackoff"); this is not arbitrary interior-substring matching.

If all tokens in a query are stopwords, the original query is used unfiltered as a fallback. The FTS5 tokenizer (`porter unicode61`) provides stemming on top of these features.

## Opt-in lexical relaxation

Ordinary searches, including `--relax=false`, retain the existing behavior and output. `--relax` is a lexical-only option, not a new default and not semantic search:

```bash
backscroll search --text 'violet handshake adaptation' --relax --robot --fields minimal --max-tokens 2000
backscroll search --text '+violet handshake technique adaptation' --relax --project example
backscroll search --text '"violet handshake" quartz marker adaptation' --relax --source-path '*/session.jsonl'
```

The deterministic sequence is:

1. Run strict AND search. Unmarked queries keep the existing sanitizer, ranking and snippets. Leading `+term` marks a term that cannot be dropped; quoted spans are protected phrase units. For queries containing these protected units, strict matching keeps every unit without dynamic stopword removal. Quotes preserve FTS phrase order; ordinary unquoted terms retain Porter stemming/prefix matching (trigram matching for tools).
2. Only if that stage has zero eligible rows, drop one **unprotected** term at a time, lowest IDF first. For a fixed corpus, this is highest document frequency first. Frequencies are counted with the actual tokenizer's MATCH expression over the applicable index(es), globally rather than within the result scope (project, path, dates, tags). Unfiltered IDF uses the same echo eligibility as unfiltered result pages: direct Backscroll retrieval-call tool rows (`search_echo=1`, a serialized `bash command=backscroll search ...` or `exec_command cmd=backscroll search ...` invocation, each matched as that three-token prefix regardless of what follows, or a serialized Codex `shell` call whose JSON-encoded argv is exactly `[<shell>, "-c" | "-lc", "backscroll search ..."]`, matched by decoding the argv rather than by text shape) do not inflate document frequency. Explicit `--content-type tool` keeps those rows in both the page and the IDF count. Equal frequencies drop in original query order. Zero-frequency terms have highest IDF and are not specially discarded. A term that appears only in those excluded echo rows is absent from the unfiltered corpus, so its document frequency is 0 and `--relax` drops it last — the same as any other zero-frequency extra term that can prevent recovery at the two-term floor. Each retry still requires every retained unit, bypassing dynamic stopwords so the retained core cannot silently disappear.
3. Stop at the first stage with results, or before fewer than **two distinct unprotected terms** remain. Protected terms are additional to that floor. Case-insensitive repeated spellings count once, and a keep marker on any occurrence protects that unit. Queries with at most two unprotected terms perform strict search only. There is no single-term/empty fallback and no global OR.

Stemming/phrase-expansion stages are skipped: stemming is already available and protected phrases must not weaken. Scope widening is always skipped. Project (including cwd-inferred project), source, source-path, content-type, role, dates, and tags are retained at every stage. Tool searches remain on their trigram index even when relaxation is explicitly requested. Existing unfiltered echo exclusion and cross-index RRF still apply.

Pagination and output budgets do not trigger extra relaxation: an exhausted page of an existing stage stays empty, and truncating a result to fit a budget does not cause a new query. Strict matches are never mixed with relaxed matches because fallback runs only after strict eligibility is empty. The existing index candidate limits and ranking are unchanged.

Opt-in syntax supports up to 32 distinct query units. A leading `+` requires a nonempty term; quoted phrases must be nonempty, balanced, and whitespace-delimited. Inside a phrase, double an inner quote (`""`). Every unit, including a protected term or phrase, must contain at least one Unicode letter or digit; punctuation-only units such as `...`, `&&`, or `::` are rejected rather than counting toward the core without constraining matches. These validations run before database/startup side effects. FTS operator words such as OR remain literal, not executable query syntax. Marker syntax and phrase parsing apply only with `--relax`.

### Provenance and limits

Every relaxed result carries its stage and the cumulative dropped terms. Robot mode adds these lines to the same budgeted result group (minimal and full):

```text
result_0_match_stage=drop-terms
result_0_dropped_terms=["adaptation"]
```

`dropped_terms` is a JSON string array encoded as a robot string: undo robot backslash/CR/LF escaping before JSON decoding. Human output includes `Match stage: drop-terms | Dropped terms: ["adaptation"]` in the result header. JSON adds optional snake_case `match_stage` and `dropped_terms` keys to both existing projections. Strict results have no extra result fields. An empty result page reports stages tried on **stderr**, for example `strict, drop-terms/1`; stdout retains its normal empty machine-readable shape. Skipped expansion/widening stages are identified as skipped, not as executed searches.

This addresses **recoverable term overload**, not vocabulary invention. In the native regression, the target contains `violet handshake`, while unrelated records make `adaptation` a common term; strict search misses and dropping that term recovers the target. In contrast, the existing synthetic `s2_conversational_paraphrase` and `s5_progressive_refinement` targets share no surviving content terms with their original queries. The refinement `violet handshake` adds new vocabulary; this feature does not promise to recover those zero-overlap cases. An absent, high-IDF extra term can also prevent recovery at the two-term floor. No production p95 or broad semantic-recall gain is claimed from the small fixture corpus.

Regression owners: `cmd/backscroll/search_relaxation_test.go` (native input-to-output recovery and unchanged strict controls), `cmd/backscroll/search_relaxation_echo_idf_e2e_test.go` (unfiltered IDF ignores query-echo rows), `cmd/backscroll/echo_shell_zero_query_gap_test.go` (zero-valued Codex `shell` echoes excluded from unfiltered pages and IDF before replay), `cmd/backscroll/search_relaxation_output_test.go` (budgeted provenance and early validation), `internal/storage/relaxation_test.go` (IDF order, protected core, scope and paging), and `internal/storage/relaxation_echo_idf_test.go` (echo-eligibility of unfiltered IDF, including echo-only DF=0 and the Codex `shell` argv boundary cases).

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Search completed (results may be empty) |
| `1` | Error (database not found, query failure) |
