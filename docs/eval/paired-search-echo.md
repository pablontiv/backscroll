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

The observed defect proceeded to a versioned regression using actual CLI output, behavioral RED on unchanged production, and a fresh production correction preserving the existing command filter. Already-indexed rows with surviving source data require reparsing and metadata-only updates without replacing perennial IDs; expired rows lacking pairing evidence must remain searchable rather than be guessed from adjacency or output shape.

## Versioned RED and unchanged GREEN oracle

The regression was committed before production edits in `c1fe0fb` (parent: the reviewed candidate). The source at RED was still exactly `0df913f3dae42b97b27e3adccbd36754ee66de8d`.

```text
Test: cmd/backscroll/paired_echo_e2e_test.go
SHA-256: 1760545a1af8106cb7229e3efa091632d09fd1f17c6d27049c396e45a70913df
Command: go test ./cmd/backscroll -run '^TestPairedBackscrollSearchResultsDoNotCrowdRecall$' -count=1 -v
RED: exit 1; target rank=7; paired results crowded target out of top five
GREEN: exit 0; paired shape exactly equals baseline, target rank=4
```

The E2E file/hash is unchanged between RED and GREEN. Both assertions run again after deleting only owned source fixtures; all ten perennial rows remain and all six command/result rows remain explicit-tool searchable.

## Correction and hardening

- The Claude reader marks direct Bash searches from raw input and propagates that fact to results by `tool_use_id` within the parsed file. No output-shape or proximity heuristic is used.
- V15 adds nullable `search_echo`. Existing session rows start NULL; surviving sources are eligible for reparsing even with unchanged hash/size/mtime. Metadata-only updates preserve original IDs, UUIDs, text and extraction epochs; later incomplete parses cannot erase positive evidence.
- The v15 replay has a measured 200-unchanged-file startup bound. The initial inherited counter did not count metadata-prefilter hits: an actual CLI regression with 201 pending files observed all 201 replayed at once. The correction counts pending echo files independently of older extraction epochs, preventing already-classified perennial rows from starving remaining NULL rows. The regression verifies 201→1→0 for both current and older extraction epochs. Other extraction semantics are not redesigned.
- Canonical recovery reads and retains positive evidence and combines it monotonically across otherwise-identical legacy/current duplicates. A regression first failed with `recovery read lost paired echo evidence`, then passed through real recovery-destination import and independent verification. Canonical content identity/hashes are unchanged.
- A further observed E2E failure after `rebuild` showed the external-content rebuild places marked tool rows in `messages_fts` too: the real SQLite MATCH returned four prose rows and six marked tool rows from that index. Therefore both unfiltered candidate streams use the same narrow echo filter/refill. This does not change rebuild semantics, explicit text/tool paths, general query matching, session inclusion, or BM25 ordering.

## Upgrade and negative-control evidence

The original PoC database, created by the reviewed binary, was opened by a fresh corrected dev binary. Native CLI search migrated v14→v15 and restored target rank 4. A read-only comparison verified all original `(id, uuid, text)` tuples unchanged, with four ordinary rows and six marked rows; repeated JSON output was byte-identical.

Committed regression coverage:

- `paired_echo_e2e_test.go`: actual robot output, baseline/top-five, explicit six-row retention, repeat after source expiry, SQLite integrity.
- `paired_echo_upgrade_test.go`: retained v14 schema populated with real CLI records and matching metadata; replay convergence, bounded-output baseline, original identities, expiry/rebuild; identical-output unrelated commands, wrappers, orphans and same-session prose.
- `echo_backfill_cap_test.go`: actual CLI replay count, 200-file bound, current/older epoch convergence.
- `internal/readers/search_echo_test.go`: raw-command boundary, interleaved paired calls/results, error output, unrelated identical output, unmatched IDs and file-local pairing.
- `internal/storage/search_echo_test.go`: migration backlog, expired/unprovable rows retained, idempotent metadata-only enrichment, immutable row identity/text/epoch and persistent positive evidence.
- `internal/storage/echo_refill_test.go`: 205 marked results plus one unrelated result survive refill in both indexes after rebuild; explicit tool search retains all 206 rows.
- `internal/storage/recovery_echo_test.go`: current/legacy duplicate unions in either order preserve positive evidence through verified recovery.
- `TestEveryCatalogFixtureReachesCurrentSemanticHead`: every retained physical lineage upgrades to v15, with the exact migration ledger verified; no old fixture or migration rewritten.

## Scope and residual limits

This is a bounded improvement, not an automatic closure of #64. Paired-result provenance currently comes from the Claude reader; Pi/OpenCode output rows remain unchanged. Pre-v15 outputs whose files expired before enrichment lack reliable call linkage and remain searchable. Absolute-path, env and shell wrappers remain negative controls. Historical prose and unrelated tool results are never excluded by session membership. The pre-existing BM25 sort inversion is explicitly out of scope.

The proposed ADR records the metadata boundary; Firstmate owns the new alternate-family exact-head review and any subsequent integration decision. No second PR, merge, release, install, or global-state mutation is part of delivery. The effective Git configuration in this task copy has no `core.hooksPath` and only sample hooks; ordinary pushes do not execute the installation-changing project pre-push script. Validation runs its repository-required checks explicitly, without installing hooks or claiming an installed-binary freshness check.
