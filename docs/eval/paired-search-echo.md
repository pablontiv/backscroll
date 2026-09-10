# Paired search-result echo correction

## Question and bounded experiment

Can the real CLI retain its no-echo historical top-five result when three direct Backscroll search calls and their actual captured outputs are ingested as paired Claude `tool_use`/`tool_result` records, without losing explicit tool retrieval?

- Limit: one paired-output CLI experiment plus one causal comparison; stop on inconclusive evidence.
- Candidate verified before changes: `0df913f3dae42b97b27e3adccbd36754ee66de8d`.
- Base and merge-base: `7642ee416aec4d529b725f9cc644a72d83db93c6`.
- Existing change: <https://github.com/pablontiv/backscroll/pull/70>.
- Prior evidence: alternate-family exact-head review identified omitted result rows; its output fixture was synthetic. This experiment instead captures actual dev-CLI robot output and re-ingests it.
- Boundary: real Go CLI, input manifest discovery, Claude JSONL reader, sync, SQLite FTS and CLI result rendering. Disposable resources are under the task's ignored `.worktrees/paired-poc/`; no global database, installed binary, preset, or configuration is used.
- Native CLI commands exercise the capability. Python is used only to encode JSONL fixtures and inspect JSON output, not as a substitute implementation or reusable runner.
- Global Backscroll recall is intentionally not invoked: startup sync mutates the global index, forbidden by this task. The prior standalone review supplies established history instead.

## Observations

Demonstrated at the reviewed candidate, before any production edits:

```text
baseline: decoy-2:text:1 decoy-1:text:2 decoy-0:text:3 target:text:4
paired:   echo-0:tool:1 decoy-2:text:2 echo-1:tool:3 decoy-1:text:4 decoy-0:text:5 echo-2:tool:6 target:text:7
top-five: echo-0:tool:1 decoy-2:text:2 echo-1:tool:3 decoy-1:text:4 decoy-0:text:5
tool-only: all six command/result rows retained
```

Native sequence: build `go build -mod=readonly -buildvcs=false -o <lab>/backscroll ./cmd/backscroll`; search the four-prose fixture with `--all-projects --json --fields full --max-tokens 0 --limit 20`; capture the same search with `--robot --fields minimal --max-tokens 0 --limit 2`; encode that exact output in three result blocks paired by call ID; repeat JSON search at limits 20 and 5 and with `--content-type tool`. All invocations use lab HOME/config/database paths. Captured JSON and robot output remain in the owned ignored lab.

Confirmed cause: the reader has cross-record call-ID pairing, but storage retains neither that link nor an echo provenance marker. Query-time recognition sees only the serialized command, not its paired output. Output-shape guessing would incorrectly exclude unrelated tools returning similar text. Historical rank is compared to the same executable's no-echo baseline; this does not claim to fix the separately reported BM25 inversion.

Next: version a regression using actual CLI output, observe behavioral RED on the unchanged production candidate, then persist precise paired-result provenance and preserve the existing command filter. Already-indexed rows with surviving source data require reparsing and metadata-only updates without replacing perennial IDs; expired rows lacking pairing evidence must remain searchable rather than be guessed from adjacency or output shape.
