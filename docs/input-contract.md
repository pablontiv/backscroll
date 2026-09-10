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
| `version` | integer | Manifest contract version. Shipped manifests use `1`. |
| `inputs` | array | Ordered `[[inputs]]` definitions. |

### `[[inputs]]`

| Field | Type | Use |
| --- | ---: | --- |
| `id` | string | Stable name shown in configuration and diagnostics. |
| `source` | string | Semantic source stored in SQLite, such as `session`, `plan`, `decision`, or `ke`. |
| `active` | bool | Only active definitions participate in ingestion. Set it explicitly. |

`source` is not the provider or decoder name. Claude, Pi, OpenCode, and Codex
conversation inputs all use `source = "session"`.

### `inputs.discover`

| Field | Type | Use |
| --- | ---: | --- |
| `roots` | array of strings | Files or directories to scan. `~` is expanded. |
| `include` | array of strings | Glob patterns relative to each root; `**` is supported. |
| `exclude` | array of strings | Glob patterns to skip. |
| `follow_symlinks` | bool | Follow symlinks when true; false in shipped presets. |

Missing or unreadable roots are skipped. Discovery never grants a reader access
outside the configured root.

### `inputs.decode`

| Field | Type | Use |
| --- | ---: | --- |
| `format` | string | Registered decoder: `claude`, `pi`, `opencode`, `codex`, `markdown_document`, or `markdown_sections`. |
| `index_reasoning` | bool | Pi/Codex opt-in for readable reasoning blocks; false by default. |

An active input with an unregistered decoder fails startup with an actionable
`no reader registered for format ...` error. Adding a new format requires a
reader implementation and registration; a manifest alone cannot define a new
provider schema.

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
exclude = []
follow_symlinks = false

[inputs.decode]
format = "pi"
index_reasoning = false
```

The Pi reader selects user/assistant text from Pi sessions, indexes supported
tool activity, and includes reasoning only when `index_reasoning = true`.

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
record timestamp for date filtering. Tool names and arguments are searchable as
`--content-type tool`; rich error/exit-code/UUID metadata is not synthesized.

Verification after installation (these commands update only Backscroll's index;
Codex files remain read-only):

```bash
backscroll config --json
backscroll search --text "a phrase you remember" --all-projects --source session --source-path '*codex*' --lexical-only --json
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
  Choose it before initial ingestion; as with existing readers, changing decode
  options alone does not invalidate unchanged file hashes.
- Malformed JSON/blocks, invalid response timestamps and unsupported payload shapes
  are skipped; valid neighboring records still ingest. File I/O errors are returned.
- Text is normalized by existing cleaning/classification; tool text is capped at
  4,000 Unicode code points. Native item/session IDs and Git metadata are not new
  storage fields. No database migration or direct Codex database access is needed.
- Like Pi/OpenCode, changed files use the UUID-less per-file reload path; unchanged
  hashes skip work, and missing source files keep their indexed history. Moving an
  already-indexed file into the archive can retain both path identities; there is
  no cross-path/session-ID deduplication. Fork/subagent histories are not specially
  filtered by Codex metadata; narrow discovery roots/excludes as needed.

## Markdown document inputs

Whole-document Markdown uses `markdown_document`. Sectioned Markdown uses
`markdown_sections`, which splits on headings starting with two hash characters
followed by a space. When a file contains one or
more such headings, content before the first heading is not indexed; when it
contains none, the entire document is indexed as one record.

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
- Unsupported decoder names fail when operational startup resolves the active
  reader; cached results are not returned after that sync failure.
- Missing discovery roots yield no files so presets for absent tools can coexist.
- The current TOML loader ignores unrecognized fields. That permissiveness is
  not an extension mechanism: ignored fields do not alter reader behavior.
- Manifests cannot run shell commands or external processes.

Every operational command validates the active configuration, attempts one
incremental sync, and then queries the perennial SQLite record. Use
`backscroll config` to inspect resolved manifests and `backscroll validate
--json` to inspect index health.
