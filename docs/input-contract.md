# Input manifest contract

Backscroll loads user-scoped `*.inputs.toml` manifests that select ingestion
sources and one registered reader for each source. A manifest tells Backscroll
**where** to discover inputs and **which dedicated decoder** understands them;
the reader owns provider-specific record selection, field extraction, content
normalization, and tool metadata.

The runtime ingestion boundary is:

```text
manifest discovery -> registered reader -> ParsedFile -> perennial SQLite
```

There is no generic JSON/JSONL selector engine. Tables such as `record`, `map`,
`content`, and `text` belonged to a retired declarative pipeline and do not
configure current readers.

## Runtime location

Manifests are loaded from:

```text
<config_dir>/backscroll/inputs/*.inputs.toml
```

`<config_dir>` is `$HOME/.config`, or `BACKSCROLL_CONFIG_DIR` when set
(`internal/input_config/loader.go` owns this resolution).
`backscroll.toml` remains application configuration and is not an ingestion
manifest.

| OS | Manifest directory |
| --- | --- |
| Linux / macOS | `$HOME/.config/backscroll/inputs/` |
| Windows | `<user-home>\\.config\\backscroll\\inputs\\` |
| Override | `$BACKSCROLL_CONFIG_DIR/backscroll/inputs/` |

Repository presets under `inputs/` are examples to install into this runtime
directory. Backscroll does not read the repository preset directory directly.

## File shape

A Claude session manifest uses only discovery and decoder selection:

```toml
version = 1

[[inputs]]
id = "claude"
source = "session"
active = true

[inputs.discover]
roots = ["~/.claude/projects"]
include = ["**/*.jsonl"]
exclude = ["**/subagents/**"]
follow_symlinks = false

[inputs.decode]
format = "claude"
```

## Manifest fields

### Top level

| Field | Type | Use |
| --- | ---: | --- |
| `version` | integer | Required manifest contract version: exactly `1`. |
| `inputs` | array | Ordered `[[inputs]]` definitions. |

### `[[inputs]]`

| Field | Type | Use |
| --- | ---: | --- |
| `id` | string | Required non-empty name for active inputs, shown in configuration and diagnostics. |
| `source` | string | Arbitrary semantic label stored in SQLite; absent/empty defaults to `session`. |
| `active` | bool | Only active definitions participate in ingestion. Set it explicitly. |

`source` is not the provider or decoder name. Claude, Pi, OMP, OpenCode, and Codex
conversation inputs all use `source = "session"`.

### `inputs.discover`

| Field | Type | Use |
| --- | ---: | --- |
| `roots` | array of strings | Active inputs require at least one non-empty root. `~` is expanded. |
| `include` | array of strings | Active inputs require at least one non-empty pattern; `**` is supported. |
| `exclude` | array of strings | Glob patterns to skip. |
| `follow_symlinks` | bool | Follow symlinks when true; false in shipped presets. |

Missing or unreadable roots are skipped. Discovery never grants a reader access
outside the configured root.

### `inputs.decode`

| Field | Type | Use |
| --- | ---: | --- |
| `format` | string | Registered decoder: `claude`, `pi`, `omp`, `opencode`, `codex`, `markdown_document`, or `markdown_sections`; absent/empty defaults to `claude`. |
| `index_reasoning` | bool | Pi/OMP/Codex opt-in for readable reasoning blocks; false by default. |

Include and exclude glob syntax is checked for every segment before discovery,
even for missing roots or patterns that would match no files. Empty patterns are
invalid. Inactive definitions skip active-input semantic checks.

An active input with an unregistered decoder fails before startup side effects
with an actionable
`no reader registered for format ...` error. Adding a new format requires a
reader implementation and registration; a manifest alone cannot define a new
provider schema.

## Ingestion-semantics replay

Each indexed source stores a versioned `v1` ingestion-semantics fingerprint in
addition to its byte hash. The fingerprint covers the registered decoder format,
that decoder's replay epoch, the manifest `source`, and `index_reasoning`.
Changing any of those semantics therefore reparses matching sources even when
their bytes are unchanged.

Startup considers only sources currently returned by active discovery. Semantic
mismatches share the deterministic 200-path replay budget with the other bounded
startup reparses and drain over later invocations. Sources that are expired,
unmounted, excluded, or otherwise unavailable are not opened or changed; their
indexed rows and stored fingerprint remain intact. Recovery preserves a source
fingerprint when its inputs agree.

Replay never retroactively purges perennial UUID-bearing history. Turning
`index_reasoning` off can prevent newly observed reasoning from being captured,
but it does not delete reasoning rows already stored under stable identities.
Readers without stable record UUIDs use whole-file snapshot replacement when an
available source is reparsed; records omitted by the new decoder configuration
can therefore leave that source snapshot. Use `purge` for intentional retention
deletion.

## Parser-backed message origin

Readers attach one of five origins to each indexed message: `human`,
`assistant`, `system`, `automation`, or `unknown`. This provenance comes only
from the reader's native structured records. Backscroll does not inspect prose,
headings, frontmatter, quoted labels, or a historical stored `role` to invent an
origin. A message that is indexed without sufficient native evidence uses
`unknown`. Depending on the reader, malformed records, incompatible
discriminators, and unsupported record types or roles may instead be omitted.

The built-in readers map native records as follows:

| Decoder | Native evidence | Stored origin |
| --- | --- | --- |
| `claude` | Matching record envelope and nested role: `user`, `assistant`, `reasoning`, or `system` | `human`, `assistant`, `assistant`, or `system`, respectively |
| `claude` | `tool_use` block | Origin proved by the enclosing record; `unknown` if its envelope and role disagree |
| `claude` | `tool_result` block | `automation`, even though Claude carries the block in a user-role record |
| `pi` (Pi/Pion) | Supported message role `user` or `assistant` | `human` or `assistant` |
| `pi` (Pi/Pion) | `toolCall` or optional `thinking` block | `assistant` |
| `pi` (Pi/Pion) | Native `message.role="toolResult"` or supported Pi `custom` output | `automation` |
| `omp` | User with `attribution="user"`, not synthetic | `human` |
| `omp` | User with `synthetic=true` or `attribution="agent"` | `automation` |
| `omp` | User without supported attribution | `unknown` |
| `omp` | Assistant text, thinking or tool call | `assistant` |
| `omp` | Native `toolResult` | `automation` |
| `omp` | Native `developer` | `system` |
| `codex` | Supported message role `user` or `assistant` | `human` or `assistant` |
| `codex` | Function/custom tool call or readable reasoning | `assistant` |
| `codex` | Function/custom tool output | `automation` |
| `opencode` | Native message role `user` or `assistant` | `human` or `assistant`; other roles are `unknown` |
| `opencode` | Tool input or explicit tool output | Input inherits the validated message origin; output is `automation` |
| `markdown_document`, `markdown_sections` | Document text, headings, or frontmatter | Always `unknown` |

Codex wrapper removal affects content only: a supported native user message
remains `human`; wrapper-like text is never treated as actor evidence. Pion's
`recordType` rules remain as described below and do not turn unsupported tool
records into automation evidence.

V16 adds the constrained origin fields and migrates older rows to `unknown`
without deriving provenance from their text or role. Startup sync can reparse a
bounded set of stale paths and enrich `unknown` only when the configured source
still exists and its reader provides evidence. An unavailable or expired source
therefore remains `unknown`. For stable identities, a later partial parse cannot
erase a proven origin or replace the retained payload, while contradictory
proven origins are rejected.

Recovery follows the same evidence rule. It preserves matching proven origins,
enriches an `unknown` duplicate from a compatible proven duplicate, and retains
independent compatible provenance. Two different proven origins for the same
identity are a recovery conflict, so the union is rejected rather than choosing
one.

## Complete Claude example

```toml
version = 1

[[inputs]]
id = "claude"
source = "session"
active = true

[inputs.discover]
roots = ["~/.claude/projects"]
include = ["**/*.jsonl"]
exclude = ["**/subagents/**"]
follow_symlinks = false

[inputs.decode]
format = "claude"
```

The Claude reader extracts text, tool inputs/results, stable message UUIDs,
timestamps, interruption markers, exit codes, and provider noise directly from
Claude's session schema.

## Complete Pi example

```toml
version = 1

[[inputs]]
id = "pi"
source = "session"
active = true

[inputs.discover]
roots = ["~/.pi/agent/sessions", "~/.pi/agent/sessions-archive"]
include = ["**/*.jsonl"]
exclude = ["**/run-*/session.jsonl"]
follow_symlinks = false

[inputs.decode]
format = "pi"
index_reasoning = false

[[inputs]]
id = "pi-subagents"
source = "session"
active = false

[inputs.discover]
roots = ["~/.pi/agent/sessions", "~/.pi/agent/sessions-archive"]
include = ["**/run-*/session.jsonl"]
exclude = []
follow_symlinks = false

[inputs.decode]
format = "pi"
index_reasoning = false
```

Ordinary Pi sessions are active by default. Nested delegated runs matching
`**/run-*/session.jsonl` are a separate, disjoint input because they can repeat
parent context and contain sensitive delegated prompts. To opt in, change only
`pi-subagents` to `active = true` in the installed `pi.inputs.toml`, then run
`backscroll config` and verify that both `pi` and `pi-subagents` are listed.
Reasoning remains independently disabled for both inputs unless its definition
explicitly sets `index_reasoning = true`. A missing `sessions-archive` root is
non-fatal.

The Pi reader selects user/assistant text from discovered Pi and Pion sessions.
Path inclusion is separate from record parsing: opting into a child path does not
make unsupported records searchable. Legacy Pi records use `type`; Pion message
records may use `recordType` as supported by [issue #98](https://github.com/pablontiv/backscroll/issues/98).
If both fields exist, they must match. Conflicts and unknown record kinds are
skipped. A lone `recordType` supports only `message`; it does not map Pion
`tool_start` or `tool_end` records onto Pi `custom` records.

Native `toolCall` blocks produce one structured event per call, with the native
tool name and call ID and a shell command head when present. Native
`message.role="toolResult"` records link by `toolCallId`, including interleaved
or out-of-order results. Their explicit `isError` true/false enriches the call;
an absent signal stays unknown. Results remain searchable automation output,
not additional calls. Direct-search output inherits echo provenance only from
its matched native call. Legacy `custom` output remains searchable separately.

When the source supplies a native session ID and complete entry/call IDs, stable
identities are `pi:<session>:call:<callID>` for calls and
`pi:<session>:entry:<entryID>#<kind>` for text, results and custom output.
Thinking identities include their physical block index (`#thinking:<index>`).
A call without its own ID uses its native entry plus physical block index
(`#call:<index>`); its result linkage remains unknown. Toggling reasoning
therefore does not move other identities. An identified native
session with an emitted record missing identity fails parsing before storage
changes. Legacy files without a native session ID retain UUID-less whole-file
snapshot replacement; identity is never derived from paths or payloads.

Pi decoder epoch 2 recovers these native events from unchanged available sources
through the shared 200-path ingestion-semantics replay queue. Retained canonical
calls can gain a later explicit result without replacement or extra events.
Unavailable, expired or excluded sources retain their stored history and
fingerprint; missing native evidence cannot be reconstructed as success.

Installers preserve an existing manifest by default. Existing users whose Pi
input still has `exclude = []` keep that behavior until they intentionally add
the `**/run-*/session.jsonl` exclusion and companion definition above, or
explicitly refresh the shipped preset. Excluding a path later is not retroactive:
rows already indexed remain in the perennial SQLite store, and `rebuild` does
not remove them. The perennial store has no deletion path.

## Complete OMP example

Install `inputs/omp.inputs.toml` into the runtime manifest directory:

```toml
version = 1

[[inputs]]
id = "omp"
source = "session"
active = true

[inputs.discover]
roots = ["~/.omp/agent/sessions"]
include = ["*/*.jsonl"]
exclude = []
follow_symlinks = false

[inputs.decode]
format = "omp"
index_reasoning = false
```

OMP is a dedicated decoder, **not a Pi alias**. Initial support is native OMP
v3 UTF-8 filesystem JSONL (structural evidence: [OMP input evidence](research/omp-input-evidence.md)).
A fixed-width `title` slot may precede the `session` header. Exactly one header
with `version=3`, nonempty `id`/`cwd` and a valid RFC3339 timestamp is required;
unsupported/missing/duplicate headers reject the file. Malformed neighboring
lines and unknown records are skipped without raw-JSON fallback.

Supported `type="message"` entries have their own nonempty `id`, RFC3339
`timestamp` and `message` envelope:

- User/developer/assistant string content or `text` blocks becomes text.
- Assistant `thinking.thinking` becomes `reasoning` only with
  `index_reasoning=true`; signatures and provider payloads are never indexed.
- Assistant `toolCall` blocks require native `id`, `name` and an object
  `arguments`; input serialization preserves tool name and command head.
- Native `role="toolResult"` reads only `content` blocks with `type="text"` as
  tool output. `toolCallId` pairs with the raw call ID in this file, even when
  output precedes input. Matching tool names guard the pairing; `parentId` is
  branch ancestry and is never used to pair calls/results.
- Native `isError` is the error signal. Only Bash (`toolName="bash"`) numeric
  integer `details.exitCode` proves an exit code; only
  `details.__interrupted=true` proves interruption. These signals are captured
  before each tool text block is capped at 4,000 runes and paired onto the call.
  Textual exit codes/interrupt phrases and assistant `stopReason="aborted"`
  are not evidence. Direct Backscroll Bash search inputs/results use the shared
  raw-input query-echo boundary, not output-text guesses.

Origin follows the native attribution table above. Missing/unsupported user
attribution stays `unknown`; textual claims never prove a human actor.
Every emitted block uses `omp:<header-session-id>:<entry-id>#b<physical-index>`
(string content uses `#b0`). Omitted images/reasoning do not renumber later
blocks. Entry IDs alone are session-scoped and cannot be global UUIDs. Header
cwd determines project identity; bucket names, `additionalDirectories` and
`parentSession` do not. All physical message entries, including stored branch
history, are indexed in order rather than reconstructing only the active branch.
UUID-bearing rows use the existing perennial append-only sync path; no new
source value or schema migration is introduced.

Excluded: title/header/admin records (`thinking_level_change`, `model_change`,
`service_tier_change`, `model_usage`, `custom`, `label`, `title_change`,
`ttsr_injection`, `credential_pin`, `mode_change`, `reset_boundary`), derived
`compaction`/`branch_summary`, `session_init`, unknown records/roles,
`redactedThinking`/encrypted reasoning, images/blob references, provider
payloads/cache keys/context snapshots and arbitrary details/custom data.
Native special roles `bashExecution`, `pythonExecution`, `fileMention`, and
custom/hook messages are outside this initial boundary. No arbitrary JSON
object is serialized to rescue unsupported content.

Canonical discovery includes direct files exactly one level under project
buckets (`*/*.jsonl`), not root-level files or nested child-agent sessions.
Missing roots are non-fatal. `PI_CODING_AGENT_DIR`, custom profiles/XDG layouts
and `--session-dir` require explicit user manifest roots; environment variables
are not interpolated or used to infer those roots. This decoder does not access
OMP SQL/Redis stores, mutate OMP configuration or change transcript files.

Verify with an installed manifest (use your actual custom source path if needed):

```bash
backscroll config --json
backscroll search --text "a phrase you remember" --all-projects --source session --source-path '*omp*' --json
backscroll search --text "a command fragment" --all-projects --source session --content-type tool --source-path '*omp*' --robot
```

## Complete Codex example

Install `inputs/codex.inputs.toml` into the runtime manifest directory:

```toml
version = 1

[[inputs]]
id = "codex"
source = "session"
active = true

[inputs.discover]
roots = ["~/.codex/sessions", "~/.codex/archived_sessions"]
include = ["**/*.jsonl"]
exclude = []
follow_symlinks = false

[inputs.decode]
format = "codex"
index_reasoning = false
```

For custom `CODEX_HOME`, replace both roots explicitly; the preset does not read
Codex global configuration or interpolate that environment variable.

The Codex reader indexes `response_item` user/assistant `input_text`/`output_text`,
JSON-string function arguments, free-form custom-tool inputs, and string or text-block
tool outputs. It uses `session_meta.cwd` for ordinary project identity and the
record timestamp for date filtering. Supported function/custom calls each emit one
tool event with the native tool name and nullable `native_call_id` from `call_id`.
Results pair to calls by that ID across interleaving, including later appended
results. Calls and outputs remain searchable as `--content-type tool`; outputs
are automation-origin text, not additional call events.

`exec_command.cmd` and `shell.command` argv prove shell command heads. Custom-tool
input and generic `exec` wrapper code have unknown command heads. Native result
`is_error` booleans preserve true/false; shell JSON `metadata.exit_code` and native
`Process exited with code …`/`Output:` envelopes prove shell outcomes before text
truncation. `exec` `Script completed`/`Script failed` envelopes describe only the
wrapper: nested tool success/failure is unknown. Other responses and missing
outputs remain unsignalled. The serialized direct-search predicate and paired
echo exclusion are unchanged; explicit tool searches still expose those records.

Verification after installation (these commands update only Backscroll's index;
Codex files remain read-only):

```bash
backscroll config --json
backscroll search --text "a phrase you remember" --all-projects --source session --source-path '*codex*' --json
backscroll search --text "a command fragment" --all-projects --content-type tool --source-path '*codex*' --robot --fields minimal --max-tokens 1000
backscroll validate --json
```

Use the actual custom root in `--source-path` if it does not contain `codex`.
There is no provider flag: `source` remains `session`; narrow by path or project.

Limits (observed format boundary and RED/GREEN evidence: [Codex input evidence](research/codex-input-evidence.md)):

- Tested shapes are Codex CLI 0.153.4 rollout JSONL, with or without envelope
  ordinals or item IDs. Event-only legacy logs are not supported. Unknown records,
  developer/system messages, UI events, compaction replacement histories, encrypted
  reasoning and image/audio payloads are not indexed.
- For user text blocks only, complete leading `<recommended_plugins>`,
  `<environment_context>`, `<heartbeat>` and `<turn_aborted>` pairs are removed
  before normalization. Trailing real prose and other content blocks survive.
  `<task>`, unknown or incomplete wrappers, embedded/quoted examples, and
  assistant/tool/reasoning text are not subject to this exclusion. Tag names
  match exactly (no attribute/case guessing). See [wrapper RED/GREEN evidence](research/codex-wrapper-evidence.md).
- `index_reasoning = true` includes only readable summary/plaintext reasoning.
  Changing it schedules the same bounded ingestion-semantics replay as source or
  decoder changes. Codex records are UUID-less, so the whole-file snapshot
  replacement caveat above applies.
- Malformed JSON/blocks, invalid response timestamps and unsupported payload shapes
  are skipped; valid neighboring records still ingest. File I/O errors are returned.
- Text is normalized by existing cleaning/classification; tool text is capped at
  4,000 Unicode code points after outcome extraction. Shared migration V19 adds
  nullable `tool_events.native_call_id` independently of canonical `message_uuid`.
  Codex search rows remain UUID-less snapshots; native call IDs do not establish
  perennial row identity or cross-file ownership. Native session/item IDs and Git
  metadata retain their existing indexing boundary.
- Codex reader epoch 2 schedules available epoch-1 inputs through the shared
  200-path replay queue, even when source bytes are unchanged. Missing sources
  retain history and their prior fingerprint. Migration leaves historical native
  IDs NULL; recovery rederives tool events from canonical text with unknown native
  identity until an available native source is reparsed.
- Changed files use the UUID-less per-file reload path; unchanged matching
  fingerprints skip work, and missing files keep their indexed history. Moving an
  already-indexed file into the archive can retain both path identities; there is
  no cross-path/session-ID deduplication. Fork/subagent histories are not specially
  filtered by Codex metadata; narrow discovery roots/excludes as needed.

## Markdown document inputs

Whole-document Markdown uses `markdown_document` and preserves every input byte
in one record. Sectioned Markdown uses `markdown_sections`, which splits on
headings starting with two hash characters followed by a space. Each section
retains its heading and internal whitespace, with outer whitespace trimmed;
content before the first heading is discarded. With no such headings, section
mode returns one whole-document record with outer whitespace trimmed. Both
modes share cancellable file reading and base-ID preparation while retaining
their distinct record expansion. Indexed Markdown identity is the source path
plus zero-based record ordinal; document UUIDs remain empty.

```toml
version = 1

[[inputs]]
id = "claude-plans"
source = "plan"
active = true

[inputs.discover]
roots = ["~/.claude/plans"]
include = ["**/*.md", "**/*.markdown"]

[inputs.decode]
format = "markdown_sections"
```

```toml
version = 1

[[inputs]]
id = "knowledge-entries"
source = "ke"
active = true

[inputs.discover]
roots = ["docs/knowledge"]
include = ["**/*.md"]

[inputs.decode]
format = "markdown_document"
```

Use `source = "plan"`, `"ke"`, `"decision"`, `"memory"`, `"rule"`, `"spec"`,
or `"backlog"` to preserve the semantic source stored in SQLite. Markdown
readers index text; they do not parse YAML frontmatter into structured metadata.

## Loading and failure behavior

- Malformed TOML fails with the manifest path in the error.
- Unsupported versions, invalid glob patterns, missing required active-input
  fields, and unsupported decoder names fail before acquiring a startup lock,
  preparing the index, or opening/writing the database. Diagnostics include the
  manifest path and field (and input identity for definition errors).
- Application configuration and manifests are read once per invocation;
  validation, discovery, ingestion, and `config` output reuse that snapshot.
  Replacing a config pathname after validation cannot change that invocation.
  Separate files are not read as one atomic filesystem transaction.
- Help and version remain independent of configuration. Recovery retains its
  ordinary index-prepare and pre-handler-sync exemption; apply uses the same
  validated snapshot for post-install sync.
- Missing discovery roots yield no files so presets for absent tools can coexist.
- The current TOML loader ignores unrecognized fields. That permissiveness is
  not an extension mechanism: ignored fields do not alter reader behavior.
- Manifests cannot run shell commands or external processes.

Every operational command validates the active configuration before startup side
effects. Owners attempt one incremental sync except recovery; read-safe followers
use the last committed SQLite snapshot, and busy `config` followers print the
validated configuration without opening the database. Use
`backscroll config` to inspect resolved manifests and `backscroll validate
--json` to inspect index health.

### OpenCode incremental snapshot token

OpenCode uses a deterministic SHA-256 token over length-framed native `message`
and `part` rows ordered by their primary IDs, including payloads and membership,
and native `session.id`/`session.directory` metadata when the session table is present.
All table scans share one SQLite read transaction, so committed WAL changes are
visible without relying on main-file size, modification time, or
`MAX(message.time_updated)`. `Parse` computes its returned token and reads its
records inside that same snapshot; a concurrent later commit is detected on the
next startup rather than attached to older parsed records. Equal-timestamp
additions, deletions, and part-only edits therefore invalidate the token, while
an unchanged native snapshot hashes identically. OpenCode reader epoch 2 queues pre-native exports through the shared, bounded 200-path ingestion-fingerprint replay.

### OpenCode native session identity

A single SQLite capability remains one `source_path`; it can contain many native
sessions. Each indexed record carries the structured `message.session_id` and,
when available, the joined `session.directory`. A present directory is used for
project-registry identification. When native cwd is absent, project proof remains
unknown; the database location supplies no native project attribution. Missing rows in a present
session table, empty identities, contradictory message/part session IDs, and
relative or NUL-bearing directories fail the parse conservatively. Message/part-only
exports remain supported without inventing native cwd.

Text identity is a deterministic UUIDv5 over a namespace-qualified native message
ID. Tool identity is a deterministic UUIDv5 over native message and part IDs, with
fixed `#t0` input and `#r0` output suffixes. Neither payload, source path, parser
position, nor timestamp participates. Identified OpenCode records therefore use
perennial append/reconciliation: insertion and reordering preserve anchors;
mutation never replaces retained payload or proven session/project ownership.

Migration V20 adds nullable `search_items.session_id` and provider-neutral
`session_identity_tags`, because the prior schema had no native-session column
and path-only tags cannot distinguish sibling sessions. Historical NULL session
identities retain source-path sequence/context behavior and legacy path tags.
Proven native sessions use only identity-scoped tags. Search/list/context expose
native session identity, context windows stay within the anchor session, and
sequence grouping uses native identity within each source capability. Recovery
retains/enriches matching session proof, rejects contradictory proof, and derives
identity-scoped tags from retained records without consulting expired sources.
