# Codex input evidence (recipe revision 2026-09-09)

## Narrow format question

Which persisted Codex JSONL records are the authoritative user/assistant text and tool input/output stream, and which records are duplicate UI events that must not be indexed twice?

The probe inspects only structural metadata (record discriminators, field names, JSON types and counts), never transcript values. All retained fixtures and searchable markers are synthetic. Real Codex stores are read-only; probes and CLI execution use isolated local data/config/database state.

## Findings and native probe

Observed 2026-09-10 with `codex-cli 0.153.4`: active `~/.codex/sessions/` (274 JSONL files) and `~/.codex/archived_sessions/` (674). Sample: first/last three sorted files from each root (12 files, not a compatibility census). Printed field names/types/counts only; no identifiers, paths inside records, commands, or transcript values were retained.

- `session_meta` contains `id`, sometimes `session_id`, `cwd`, timestamp and provenance; envelopes include optional `ordinal`.
- `response_item/message` contains user `input_text` and assistant `output_text` blocks. Developer instructions are separate messages. IDs can be absent.
- `function_call.arguments` was a JSON-encoded object string (80 sampled calls). `custom_tool_call.input` is a free-form string. Function/custom outputs occurred as both strings and `input_text` block arrays (52/28 function outputs; 21/652 custom outputs).
- `reasoning.summary` uses `summary_text`; encrypted content is opaque. Only readable reasoning should be opt-in.
- UI/event records (`item_completed`, `task_complete`, token counts), `turn_context`, world state and compaction replacement histories coexist with response items. Indexing them as extra messages would duplicate or pollute recall; this reader deliberately indexes only response items. Event-only legacy logs are not supported.

Independent public schema corroboration: [Codex protocol models](https://github.com/openai/codex/blob/main/codex-rs/protocol/src/models.rs), inspected 2026-09-10 (`ResponseItem`, `ContentItem`, `FunctionCallOutputBody`). The schema supports string and content-item tool outputs; `input_image`/audio are not text.

Native Go PoC: a disposable test-only `codex` registry implementation parsed synthetic user `input_text` from `tests/fixtures/codex-rollout-v1.jsonl`. It copied the fixture into a temp source root and used isolated HOME/config/SQLite, manifest discovery, startup sync and `runCmd("search", "codexuserquartz", "--all-projects", "--json", "--lexical-only")`. `go test ./cmd/backscroll -run '^TestCodexNativeSpike$' -v` passed in 0.02s. The prototype was removed from the build before production work; it is not reused as the implementation. Native reader-to-FTS feasibility is verified, not all historical Codex formats.

The fixture is hand-authored synthetic data matching observed shapes, not a copied private transcript. The `event_msg/user_message` compatibility duplicate and image block are explicit negative controls; the sampled current producer primarily emitted `item_completed` instead. No live Backscroll recall was run because ordinary startup would mutate the shared index outside this task's scope.

## Versioned RED

Before any production edits, `go test ./cmd/backscroll -run '^TestCodexRolloutV1E2E$' -v` failed for all seven positive recall cases with `index_stale: index sync failed: resolve reader for input "codex": no reader registered for format "codex"`. The test and synthetic fixture were committed before production implementation. The oracle crosses real manifest routing, startup sync, SQLite, project/source/path/role/content/date filters and CLI JSON; it requires one match per marker, not merely a successful parser return.
