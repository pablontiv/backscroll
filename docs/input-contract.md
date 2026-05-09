# Generic input manifest contract

This is the O02 MVP contract for `*.inputs.toml` files. A file such as
`claude.inputs.toml` or `pi.inputs.toml` declares how Backscroll turns agent
conversation records into the stable ingestion boundary:

- `ParsedFile { source, source_path, hash, project, messages }`
- `ParsedMessage { role, text, ordinal, uuid, timestamp, content_type }`

The manifest carries provider-specific details in data, while Backscroll keeps a
provider-neutral pipeline. Runtime loaders discover manifests from the OS-aware
user config directory at `<config_dir>/backscroll/inputs/*.inputs.toml`; set
`BACKSCROLL_CONFIG_DIR` to override `<config_dir>` in tests or custom installs.
`backscroll.toml` remains application configuration and is not the canonical
source of ingestion routes.


```text
discover -> decode -> record -> map -> content -> text -> emit
```

## File shape

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

[inputs.decode]
format = "jsonl"
encoding = "utf-8"

[inputs.record]
selector = "$"
include_when = [
  { selector = "$.type", op = "in", value = ["user", "assistant"] },
  { selector = "$.isMeta", op = "ne", value = true },
]

[inputs.map]
role = "$.message.role"
uuid = "$.uuid"
timestamp = "$.timestamp"
session_id = "$.sessionId"

[inputs.map.role_aliases]
human = "user"

[inputs.content]
selector = "$.message.content"
string = "$"
blocks = "$.message.content[*]"
block_text = "$.text"
content_type = "$.type"
include_when = [
  { selector = "$.type", op = "eq", value = "text" },
]

[inputs.text]
join = "\n"
trim = true
drop_empty = true
remove = [
  { kind = "regex", pattern = "<system-reminder>[\\s\\S]*?</system-reminder>" },
  { kind = "regex", pattern = "<task-notification>[\\s\\S]*?</task-notification>" },
]
```

## Top-level fields

| Field | Type | Default | Meaning |
|---|---:|---:|---|
| `version` | integer | required | Contract version. MVP uses `1`. |
| `inputs` | array | required | Ordered input definitions. |

## `[[inputs]]`

| Field | Type | Default | Meaning |
|---|---:|---:|---|
| `id` | string | required | Stable manifest-local input name such as `claude` or `pi`. |
| `source` | string | required | Semantic Backscroll source emitted to storage. Conversations use `session`. |
| `active` | bool | `true` | Allows disabling an input without deleting it. |

`source` is not the provider name and not the file format. Claude and Pi
conversation manifests both set `source = "session"`; provider details belong in
`id`, `discover`, `decode`, selectors, and filters.

## `discover`

Finds candidate files without provider-specific Rust rules.

| Field | Type | Default | Meaning |
|---|---:|---:|---|
| `roots` | array of strings | required | Files or directories to scan. |
| `include` | array of glob strings | required | Positive file patterns. |
| `exclude` | array of glob strings | `[]` | Negative file patterns. |
| `follow_symlinks` | bool | `false` | Whether directory walking follows symlinks. |

Claude subagent exclusion is expressed here as data:

```toml
[inputs.discover]
roots = ["~/.claude/projects"]
include = ["**/*.jsonl"]
exclude = ["**/subagents/**"]
```

The core only applies generic glob include/exclude rules; it does not need to
know what `subagents` means.

## `decode`

Declares the technical file format.

| Field | Type | Default | Meaning |
|---|---:|---:|---|
| `format` | enum | required | MVP values: `jsonl`, `json`, `markdown`, `markdown_sections`. |
| `encoding` | string | `utf-8` | Text encoding for file reads. |

## `record`

Selects and filters raw decoded records before mapping.

| Field | Type | Default | Meaning |
|---|---:|---:|---|
| `selector` | JSONPath string | `$` | Record selector inside each decoded item. |
| `include_when` | array of predicates | `[]` | Predicates that must match. |
| `exclude_when` | array of predicates | `[]` | Predicates that drop a record when matched. |

Predicate shape:

```toml
{ selector = "$.type", op = "eq", value = "assistant" }
```

MVP operators are `eq`, `ne`, `in`, `exists`, and `missing`.

## `map`

Maps record fields to Backscroll metadata. Required for `jsonl` and `json` inputs. Markdown inputs (`markdown` and `markdown_sections`) emit document text directly and may omit this section.

| Field | Type | Default | Meaning |
|---|---:|---:|---|
| `role` | JSONPath string | required | Role value before aliasing. |
| `uuid` | JSONPath string | unset | Message or session identifier. |
| `timestamp` | JSONPath string | unset | Message timestamp. |
| `session_id` | JSONPath string | unset | Conversation identifier. |
| `project` | JSONPath string | unset | Project value when present in data. |
| `role_aliases` | table | `{}` | Provider role names mapped to Backscroll roles. |

## `content`

Selects text-bearing values and optional content blocks. Required for `jsonl` and `json` inputs. Markdown inputs may omit this section; when omitted, emitted messages use `content_type = "text"`.

| Field | Type | Default | Meaning |
|---|---:|---:|---|
| `selector` | JSONPath string | required | Content value selector. |
| `string` | JSONPath string | `$` | String selector when content is a scalar. |
| `blocks` | JSONPath string | unset | Selector for arrays of content blocks. |
| `block_text` | JSONPath string | unset | Text selector within each block. |
| `content_type` | JSONPath string | unset | Selector for a block or message content type. |
| `include_when` | array of predicates | `[]` | Predicates that a block must match. |
| `exclude_when` | array of predicates | `[]` | Predicates that drop a block when matched. |
| `default_content_type` | string | `text` | Content type used when no selector yields a value. |

Pi non-text block filtering is expressed here as data:

```toml
[inputs.content]
selector = "$.message.content"
blocks = "$.message.content[*]"
block_text = "$.text"
content_type = "$.type"
include_when = [
  { selector = "$.type", op = "eq", value = "text" },
]
```

The core only evaluates the generic predicate; it does not need Pi-specific
knowledge of `thinking` or `toolCall` blocks.

## `text`

Normalizes extracted text.

| Field | Type | Default | Meaning |
|---|---:|---:|---|
| `join` | string | `\n` | Separator for multiple text fragments. |
| `trim` | bool | `true` | Trim leading and trailing whitespace. |
| `drop_empty` | bool | `true` | Drop messages whose final text is empty. |
| `remove` | array of remove rules | `[]` | Ordered text removal rules. |

Remove rule shape:

```toml
{ kind = "regex", pattern = "<system-reminder>[\\s\\S]*?</system-reminder>" }
```

MVP `kind` values are `regex`, `prefix`, and `suffix`.

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

[inputs.decode]
format = "jsonl"

[inputs.record]
selector = "$"
include_when = [
  { selector = "$.type", op = "in", value = ["user", "assistant"] },
]
exclude_when = [
  { selector = "$.isMeta", op = "eq", value = true },
]

[inputs.map]
role = "$.message.role"
uuid = "$.uuid"
timestamp = "$.timestamp"
session_id = "$.sessionId"

[inputs.content]
selector = "$.message.content"
string = "$"
blocks = "$.message.content[*]"
block_text = "$.text"
content_type = "$.type"
include_when = [
  { selector = "$.type", op = "eq", value = "text" },
]
default_content_type = "text"

[inputs.text]
join = "\n"
trim = true
drop_empty = true
remove = [
  { kind = "regex", pattern = "<system-reminder>[\\s\\S]*?</system-reminder>" },
  { kind = "regex", pattern = "<task-notification>[\\s\\S]*?</task-notification>" },
]
```

## Complete Pi example

```toml
version = 1

[[inputs]]
id = "pi"
source = "session"
active = true

[inputs.discover]
roots = ["~/.pi/agent/sessions"]
include = ["**/*.jsonl"]
exclude = []

[inputs.decode]
format = "jsonl"

[inputs.record]
selector = "$"
include_when = [
  { selector = "$.type", op = "eq", value = "message" },
  { selector = "$.message.role", op = "in", value = ["user", "assistant"] },
]

[inputs.map]
role = "$.message.role"
uuid = "$.id"
timestamp = "$.timestamp"

[inputs.content]
selector = "$.message.content"
string = "$"
blocks = "$.message.content[*]"
block_text = "$.text"
content_type = "$.type"
include_when = [
  { selector = "$.type", op = "eq", value = "text" },
]
default_content_type = "text"

[inputs.text]
join = "\n"
trim = true
drop_empty = true
```

## Markdown document inputs

Plans and external documents are declared as normal inputs. Whole-document markdown uses `decode.format = "markdown"`; sectioned markdown uses `decode.format = "markdown_sections"`, which splits on `## ` headers and preserves any pre-header preamble as the first message.

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
format = "markdown"
```

Use `source = "plan"`, `"ke"`, `"decision"`, `"memory"`, `"rule"`, `"spec"`, or `"backlog"` to preserve the semantic source stored in SQLite. Specs can opt into `markdown_sections` when section-level indexing is desired.

## Validation policy

- Unknown fields are invalid at every level.
- Missing required fields are invalid.
- Predicate operators outside the MVP set are invalid.
- `source` must be explicit; conversation manifests for Claude and Pi use
  `source = "session"`.
- All selectors are JSONPath in the MVP. JMESPath is outside the MVP and is
  reserved for the future evaluation in
  [T013](roadmap/O02-generic-agnostic-input-engine/T013-evaluate-jmespath-future.md).
- The contract has no fields that run shell or external processes. It is limited
  to discovery, decoding, selectors, predicates, mapping, and text normalization.
