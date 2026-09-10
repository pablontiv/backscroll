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
before production changes.
