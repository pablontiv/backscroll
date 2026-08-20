# Legacy Sources Migration and Markdown Inputs Design

**Date:** 2026-08-19
**Issue:** [#31 — upgrade from a 0.3.x database is not seamless](https://github.com/pablontiv/backscroll/issues/31)

## Summary

Issue #31 reported two upgrade regressions from 0.3.18 to v3.2.5:

1. V6 attempted to drop `search_items.source_metadata` from a released lineage that never had the column.
2. The legacy `[sources]` table remained accepted by `config.toml` but was no longer consumed, silently dropping decision and knowledge-source ingestion.

The first regression is already resolved on `main` by #34. Compatibility planning now inspects the actual schema shape and skips V6 when `source_metadata` is absent. This design does not change migrations.

The remaining regression will be handled as an explicit migration boundary. Any legacy `[sources]` table will make every executable subcommand fail before database or source side effects, with deterministic instructions to migrate to `*.inputs.toml`. The migration destination will be made functional by adding declarative readers for whole-document and sectioned Markdown inputs.

## Goals

- Never silently ignore a legacy `[sources]` table.
- Block every executable subcommand when `[sources]` is present, including `config`, `read`, and `recover`.
- Report every config file containing `[sources]`, its configured keys, and the declarative input directory.
- Keep `--help` and `--version` available for diagnosis.
- Make `decode.format = "markdown_sections"` and `decode.format = "markdown_document"` functional in the normal reader/autosync pipeline.
- Preserve source classification through `InputDefinition.Source` (`decision`, `ke`, `memory`, `rule`, `spec`, or `backlog`).
- Correct the shipped decisions preset so its instructions match the current CLI and parser behavior.
- Preserve the schema-shape migration behavior already delivered by #34.
- Deliver the change through a dedicated PR for issue #31.

## Non-goals

- Restoring the legacy `[sources]` ingestion path.
- Automatically rewriting `config.toml` or generating manifests.
- Supporting glob syntax inside legacy `[sources]` values.
- Adding a `sync` command.
- Parsing or indexing YAML frontmatter fields as structured metadata.
- Changing SQLite schema, migrations, recovery, or perennial lifecycle rules.
- Activating the shipped decisions preset by default.

## Existing State

### Database compatibility

`internal/compat.remainingStepsFor` already omits migration V6 when the inspected shape lacks `source_metadata`. Existing compatibility tests distinguish otherwise-equivalent V5/V3 lineages by observed columns, and migration application remains atomic.

No new compatibility mechanism is needed. The issue #31 PR will retain and execute the existing regression coverage rather than modifying migration code.

### Legacy configuration

`internal/config.Config` still contains `Sources SourcesConfig`, and `loadConfigFile` still merges its values. No production caller reads `cfg.Sources`, so a valid-looking configuration is silently ignored.

### Declarative Markdown inputs

`inputs/decisions.inputs.toml` declares `decode.format = "markdown_sections"`. `newDefaultAutoSyncRegistry` registers only Claude, Pi, and OpenCode readers, so activating the preset fails with `no reader registered for format "markdown_sections"`.

The preset also instructs users to run the removed `backscroll sync` command and claims structured frontmatter behavior that the current source parser does not provide.

## Design

### 1. Legacy configuration detection

Add a config-layer validator:

```go
func ValidateNoLegacySources(inputsDir string) error
```

The validator will inspect the same two config locations used by `config.Load`, in resolution order:

1. global `config.toml`;
2. local `./backscroll.toml`.

It will parse a minimal TOML envelope with a pointer-valued `Sources` field. A non-nil pointer means the `[sources]` table exists; even an empty table is unsupported and must fail.

The validator will collect all offending files rather than stopping at the first. For each file it records configured non-empty keys from this fixed vocabulary:

```text
backlog, decisions, ke, memories, rules, specs
```

Keys are sorted lexicographically, and files retain deterministic global-then-local order. An empty table is reported as `keys: none`.

The returned typed error will contain structured entries and an `Error()` message suitable for direct CLI output. The message includes the canonical `inputsDir` supplied by the caller and the two supported migration formats:

```text
markdown_document
markdown_sections
```

Missing config files are ignored. TOML syntax errors continue to be reported by normal config loading; the validator reports parse errors with the offending path rather than treating malformed TOML as legacy configuration.

### 2. Global CLI preflight

`buildRootCmd` will install a `PersistentPreRunE` hook that:

1. resolves the canonical input directory with `input_config.InputsDir()`;
2. calls `config.ValidateNoLegacySources(inputsDir)`;
3. returns the error before the selected command's `RunE` executes.

This makes the rejection uniform across all executable subcommands, including commands that currently bypass `config.Load`, such as `read`.

Cobra's built-in help and version exits do not execute command hooks, so these diagnostic paths remain available:

```bash
backscroll --help
backscroll --version
backscroll <command> --help
```

The preflight runs before any database open, migration, autosync, recovery, or direct source read. There is no cached-index fallback for this configuration error.

### 3. Markdown reader boundary

Add a focused Markdown reader implementation in `internal/readers`. It will expose two registered formats:

```text
markdown_document
markdown_sections
```

Both implement the existing `SessionReader` interface and reuse established components:

- discovery: `input_config.DiscoverFiles`;
- hashing: `hashfile.HashFile`;
- parsing: `sources.ParseDocument` or `sources.ParseSectioned`;
- source classification: `InputDefinition.Source`, applied later by `maybeAutoSync` to `storage.IndexedFile.Source`.

A shared internal implementation may back two small constructors or reader types, but each registered instance has a stable `Name()` matching its decode format.

### 4. Markdown record mapping

Each parsed source item becomes one `models.Message`:

```text
Role:        document
Content:     source item content
ContentType: text
Timestamp:   source file modification time
UUID:        empty
```

The returned `models.ParsedFile` contains:

```text
Path: source file path
Hash: SHA-256 from hashfile
Cwd:  source file parent directory
```

An empty UUID is intentional. Markdown sources are mutable non-session inputs, so the existing `SyncFiles` wipe-and-reload path remains correct and idempotent by file hash.

For `markdown_sections`, each `## ` section becomes a record. A file without `## ` sections becomes one whole-document record, preserving `sources.ParseSectioned` behavior. `markdown_document` always produces one record.

File stat, hash, discovery, and parse errors are returned with path context. The reader does not invent structured frontmatter fields, stable message UUIDs, or section timestamps.

### 5. Registry and autosync integration

`newDefaultAutoSyncRegistry` will register both Markdown formats alongside Claude, Pi, and OpenCode.

No parallel source-ingestion path will be added. Declarative Markdown definitions flow through the existing sequence:

```text
ActiveInputs
  -> Registry.ForDef
  -> Discover / Hash / Parse
  -> storage.IndexedFile{Source: def.Source}
  -> SyncFiles
```

This gives Markdown inputs the same incremental hashing, project identification, source filtering, and storage lifecycle as other inputs.

### 6. Preset and migration guidance

Update `inputs/decisions.inputs.toml` to:

- keep `active = false`;
- retain directory/glob discovery;
- retain `decode.format = "markdown_sections"`;
- remove the nonexistent `backscroll sync` instruction;
- explain that active inputs sync automatically before index-consuming commands;
- remove claims that YAML frontmatter is parsed into structured metadata.

The legacy error and documentation will provide this mapping:

| Legacy key | Declarative source | Decode format |
|---|---|---|
| `ke` | `ke` | `markdown_document` |
| `memories` | `memory` | `markdown_document` |
| `decisions` | `decision` | `markdown_sections` |
| `rules` | `rule` | `markdown_sections` |
| `specs` | `spec` | `markdown_sections` |
| `backlog` | `backlog` | `markdown_sections` |

The tool reports the migration; it does not perform it automatically.

## Data Flow

### Unsupported legacy configuration

```text
CLI invocation
  -> root PersistentPreRunE
  -> resolve InputsDir
  -> inspect global + local config
  -> [sources] found
  -> deterministic migration error
  -> command RunE never executes
  -> no DB/source side effects
```

### Migrated declarative Markdown input

```text
*.inputs.toml
  -> ActiveInputs
  -> Markdown reader selected by decode.format
  -> DiscoverFiles
  -> hash + document/section parse
  -> ParsedFile records
  -> IndexedFile with def.Source
  -> SyncFiles
  -> FTS search by --source
```

## Error Handling

- Missing config files: ignored.
- Malformed config TOML during preflight: hard error with path; no command execution.
- One or more `[sources]` tables: one deterministic hard error listing all offending files.
- Unknown Markdown source type: the reader preserves `def.Source`; it does not enforce a closed source enum.
- Unsupported decode format: existing `Registry.ForDef` error remains unchanged.
- Missing/unreadable Markdown file: contextual reader error; autosync fails rather than silently skipping a configured source.
- No Markdown sections: one whole-document text record.
- Empty Markdown file: one empty document record, consistent with current `ParseDocument`/`ParseSectioned`; storage behavior remains deterministic.

## Testing

### Config validator

- Detect a global `[sources]` table.
- Detect a local `[sources]` table.
- Report both files in deterministic order.
- Reject an empty `[sources]` table.
- Sort configured keys deterministically.
- Include the supplied input directory and both Markdown formats in the error.
- Ignore absent config files.
- Preserve path context for malformed TOML.

### CLI preflight

Use direct `buildRootCmd` execution with an isolated HOME, config directory, working directory, and database path.

- Table-test every executable subcommand with syntactically valid minimum arguments and require the legacy migration error.
- Assert the database path and command-specific output files remain absent.
- Prove `read` is blocked before opening its requested source file.
- Prove `recover` is blocked before reading or replacing databases.
- Prove `config` is blocked rather than displaying silently ignored values.
- Prove root help, root version, and subcommand help remain available.

### Markdown readers

- `markdown_document` returns exactly one document record.
- `markdown_sections` returns one record per `## ` section.
- Section mode falls back to one record when no sections exist.
- Role, content type, modtime, hash, path, cwd, and empty UUID are correct.
- Discovery honors configured roots/globs.
- Missing and unreadable files return contextual errors.
- Registry resolves both exact format names.

### End-to-end declarative ingestion

Create a hermetic active decisions manifest and Markdown decision file, then run an index-consuming CLI command:

- autosync discovers and parses the file;
- `search --source decision` returns a unique section term;
- a second invocation with unchanged hash is idempotent;
- no legacy `[sources]` config is present.

### Existing migration regression

Run the existing compatibility tests that prove a released lineage without `source_metadata` skips V6 and reaches current schema. Do not duplicate migration code or alter migration fixtures unless a missing assertion is discovered.

### Verification commands

```bash
just check
just test
just ci
git diff --check origin/main...HEAD
```

## Documentation Impact

- Add `MarkdownReader` to the `internal/readers` description in `CLAUDE.md` and Package Layout because the reader package's implemented formats change.
- Update `inputs/decisions.inputs.toml` comments.
- Keep issue #31 and PR text explicit that migration compatibility was delivered by #34 while this PR resolves the remaining silent configuration loss and repairs the documented migration target.
- After this PR, re-evaluate issue #33. Its requested directory/glob model already exists; a working Markdown reader may satisfy the remaining observed failure.
