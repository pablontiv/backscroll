# Codex injected-wrapper exclusion

## Evidence and bounded question

The independent review of `8f48b8b` identified a missing content-class boundary:
Codex injects some non-conversation material as `response_item/message` with
`role=user`. Which known wrappers can be removed without dropping real requests?

The review's read-only structural census of 948 Codex 0.153.4 files found leading
`recommended_plugins` (752 messages, 40.8 MB), `environment_context` (154),
`heartbeat` (12) and `turn_aborted` (3) wrappers. The four classes together were
reported at roughly 43 MB. These are observed corpus totals, **not a measured
post-fix index-size reduction**. Only tag names/counts were retained. No private
transcript values are in this repository.

A bounded producer-side structural recheck (first/last three files from each
root, 12 files total) verified complete leading pairs for `recommended_plugins`
(7), `environment_context` (14) and `turn_aborted` (3), with no incomplete pairs
or trailing prose among those observations. No heartbeat occurred in this sample;
its tag-name evidence remains the independent census. Only tags/counts/completeness
booleans were printed; no text values were retained.

`task` wrappers (514 messages, mostly distinct) were also observed; they remain
searchable because they can carry real assignments. Unknown wrappers likewise
remain searchable. A generic XML/HTML stripper or dropping all user messages
would remove legitimate recall content.

## Cause and native RED

Trigger: a known injected wrapper in a user text block. The reader previously
passed it unchanged to `sync.CleanContent`, whose tag-with-content list has no
Codex wrappers. The visible symptom is boilerplate returned as user prose through
normal manifest discovery, startup sync, SQLite FTS and CLI search. A corpus or
fixture without those content classes masks the defect.

`tests/fixtures/codex-wrappers-v1.jsonl` is wholly synthetic. On the unchanged
production reader, this command failed six exclusion cases while all six
preservation controls passed:

```bash
go test ./cmd/backscroll -run '^TestCodexInjectedWrappersV1E2E$' -count=1 -v
```

The failing cases cover all four observed wrapper names plus mixed trailing prose
and mixed content blocks. Counterfactual controls retain ordinary user requests,
`task` assignments, wrappers quoted after ordinary prose, assistant text and
real prose accompanying an injected block. The RED fixture/test is committed
before production changes in `6c4e355`.

## GREEN and preservation boundary

The same E2E command now passes all twelve cases. Production removes only
complete leading pairs for the four known tags, independently within each user
text block and before whitespace normalization. Repeated leading pairs are
removed in order; processing stops at ordinary text, an unknown tag or an
unclosed pair. Trailing real prose and other blocks are preserved. Task and
unknown wrappers, quoted/embedded examples and assistant/tool/reasoning content
remain searchable. This is not a generic markup cleaner or a change to other
readers.

Layer tests cover all four tag names, a catalog-sized synthetic block, whitespace,
trailing requests, repeated pairs, exact-name/attribute/case boundaries, incomplete
pairs, task/unknown wrappers and non-user content. A property fuzz test verifies
idempotence and that output is only a suffix of the original trimmed text.
The manual skill preset-copy command now includes Codex and has a regression test.

Validation: `just check`, targeted Codex/living-doc/skill tests, `just ci` and the
full race suite all pass; aggregate statement coverage is **86.2%**, readers
**91.2%**. Five-second wrapper fuzzing passed **798,969 executions** in this run.

No real Codex ingestion or configuration changes were performed. This correction
precedes the first operator ingestion. As already documented, reader-logic changes
alone do not invalidate unchanged input hashes; this fix does not add a migration
or promise cleanup of a previously indexed private Codex corpus. Fresh independent
review must bind to the updated head before merge.
