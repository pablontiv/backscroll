# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Backscroll is a Go CLI tool that indexes Claude Code, Pi, and OpenCode sessions, plans, and external knowledge sources into SQLite for full-text search (BM25 via FTS5). It treats sessions as an event store with incremental sync via SHA-256 deduplication.

**Status**: Go port complete — `main` branch is the active Go implementation. The Rust implementation is frozen in the `v0` branch.

Implemented: `internal/config`, `internal/input_config`, `internal/models`, `internal/readers`, `internal/sync`, `internal/tagging`, `internal/plans`, `internal/sources`, `internal/storage`, `internal/projects`, `internal/reader`, `internal/templates`, `internal/corrections`. CLI commands in `cmd/backscroll/` (10 v2 commands via cobra).

Stack: cobra, go-toml/v2, goldmark, modernc.org/sqlite (pure Go, no CGO), stdlib testing.

## Build & Test Commands

```bash
just check              # gofmt --check + go vet
just test               # go test ./...
just fmt                # gofmt -w .
just build              # go build -o backscroll ./cmd/backscroll
just coverage-summary   # go test -cover ./...
just coverage           # coverage report via pkcov
just coverage-check     # coverage report + enforce per-package floors (≥85%)
just audit              # go mod verify
just ci                 # local mirror of CI gate: build + scrubbed-HOME tests + coverage ≥85%
```

Run a single test: `go test -run TestName ./internal/...`

**Pre-push gate**: the pre-push hook validates that Module Layout and Package Layout sections in CLAUDE.md are up to date whenever a Go package is added or deleted. When deleting a package, remove its entries from the "Implemented:" list, the `internal/` tree in Module Layout, and the Package Layout table before committing, or the push will be rejected. The hook also runs `just ci` when any `*.go` file changes — push is blocked if the CI gate fails (build error, test failure, or coverage below 85%). Test-only changes (`*_test.go`) are exempt from the docs-update requirement.

**Coverage**: the release-blocking gate is **aggregate** statement coverage ≥85%, checked identically by CI (crossbeam `go-ci` light profile) and the local pre-push hook via `just ci`. Per-package floors in `.coverage-floors.toml` (default 85%) remain available as an advisory quality check via `just coverage-check` (pkcov), but are **not** release-blocking — individual packages may dip below 85% as long as the aggregate holds. backscroll conforms to [coverage-spec v1.0](https://github.com/pablontiv/picokit/blob/main/docs/coverage-spec.md).

Tests use stdlib `testing` + subprocess or direct `run()` invocation. Unit tests are co-located in each package. Integration tests in `cmd/backscroll/main_test.go` (CLI integration via direct `run()` invocation). Additional unit tests: `internal/storage/unit_test.go`, `internal/sync/noise_test.go`, `internal/reader/semantic_test.go`. The push gate and CI both enforce aggregate coverage ≥85% (`just ci`); `just coverage-check` (pkcov per-package floors) is advisory. Tests must be hermetic — scrub machine state with `testEnv(t)` / `t.Setenv("HOME", tempDir)` so they pass in CI's bare environment, which `just ci` reproduces via a scrubbed `HOME`/`BACKSCROLL_CONFIG_DIR`.

## Architecture

### Module Layout

```
cmd/backscroll/
├── main.go            — entrypoint; run(stdout, stderr, args) for testability
├── list.go            — list command (v2: --input, --order, --type, --tool)
├── search.go          — search command (v2: --text, --input)
├── read.go            — read command (v2: --path, --tail, --semantic, --pretty)
├── patterns.go        — patterns command (v2: --kind commands|failures|templates|corrections [--pending] [--batch N], --project, --tag, --min-support, --min-confidence, --json, --robot)
├── annotate.go        — annotate command (F3b: --uuid --kind --label; validates message existence; upsert semantics)
├── status.go          — status command
├── validate.go        — validate command (--indexed-only)
├── rebuild.go         — rebuild command (replaces reindex)
├── purge.go           — purge command
├── config.go          — config command (shows effective config + inputs)
└── sync_helpers.go    — shared auto-sync helpers (maybeAutoSync, runSync)
internal/
├── config/            — config resolution: backscroll.toml → ~/.config → env → defaults
├── input_config/      — input manifest loading, discovery, and legacy session-dirs compatibility
├── models/            — domain types: SessionRecord, MessageContent, ParsedFile, SearchResult, Stats
├── sync/              — WalkDir, SHA-256 dedup, JSONL parsing, noise filtering, content-type classification
├── tagging/           — heuristic auto-tagging (debugging, refactoring, feature, testing, docs, config)
├── plans/             — Markdown plan parser (split by ## headers, goldmark)
├── sources/           — external source parsers (ke, decision, memory, rule, spec, backlog) + SourceRegistry
├── projects/          — project identity registry: LoadGlobalRegistry(), Identify(), LoadLocalHint()
├── reader/            — direct reading and filtering of individual session files
├── readers/           — SessionReader interface, Registry, ClaudeReader (text+tool_use+tool_result), PiReader (text+toolCall+custom results), OpenCodeReader (text+tool state.input+state.output); toolfmt serializer
├── templates/         — F2 Drain-inspired miner: Miner, ProcessLine, ExtractErrorLines, deterministic signature via SHA256
├── corrections/       — F3 correction detection: bilingual lexicon, interrupt flags, denial heuristics, rephrase-similarity; detector registry + implementations
└── storage/           — SQLite adapter (dual FTS5 indexes: tool_fts + messages_fts, BM25, WAL mode, migrations v1–v12, search_items, session_tags, tool_events, message_templates, template_matches, correction_signals, annotations, AggregateCommands, AggregateFailures, AggregateTemplates, AggregateCorrections, UpsertAnnotation)
```

Ten v2 CLI commands: `list [--project] [--all-projects] [--order timestamp:desc|asc] [--limit] [--offset] [--json]`, `search [--text <query>] [--project] [--all-projects] [--after] [--before] [--limit] [--offset] [--indexed-only] [--json]`, `read --path <path> [--tail <n>] [--semantic] [--pretty]`, `patterns --kind commands|failures|templates|corrections [--pending] [--batch N] [--project] [--all-projects] [--tag] [--min-support N] [--min-confidence F] [--limit] [--offset] [--indexed-only] [--json] [--robot]`, `annotate --uuid <u> --kind <k> --label <l> [--path <p> --ordinal <n>]`, `status`, `validate [--indexed-only]`, `rebuild`, `purge --before <date>`, `config [--json]`.

The `SearchEngine` interface is the port; `internal/storage` is the adapter. Database opened lazily. `OpenReadOnly()` provides read-only access for external consumers.

### Core Pipeline

```
JSONL files → fs.WalkDir → SHA-256 dedup → ParseSessions() ─┐
Markdown plans → DiscoverPlanFiles() → ParsePlan() ──────────┤
External sources → SourceRegistry.ParseAll() ─────────────────┤
                                                              ▼
                                              SyncFiles() → SQLite FTS5
                                                            │
CLI query → Search() → BM25 → format_results()
```

### External Source Types

Configurable in `[sources]` section of `backscroll.toml`. Source types: `ke`, `decision`, `memory`, `rule`, `spec`, `backlog`. Each has per-type parsers (whole-document or sectioned by ## headers). All filterable via `--source` flag.

### Key Design Decisions

- **Defensive parsing**: `SessionRecord` wrapper with `json.RawMessage` for fields handles legacy schemas and noise.
- **Noise filtering**: Excludes `system-reminder`, `task-notification`, and subagent sessions by default.
- **External FTS5**: Uses `search_items` as content table with SQLite triggers, `snippet()` extraction, and Porter stemmer tokenizer for morphological matching.
- **Incremental sync**: SHA-256 hash per file stored in `indexed_files` table; unchanged files are skipped.
- **Plan indexing**: Markdown plans from `~/.claude/plans/` split by `##` headers, each section indexed as a separate search item with `source='plan'`.
- **Source filtering**: `search_items.source` column distinguishes sessions from plans; `--source` flag filters at query time.
- **Date filtering**: `--after`/`--before` flags filter by `search_items.timestamp` with NULL-safe guards; `--before` uses exclusive `<` comparison.
- **Multi-path config**: `SessionDirs []string` with backward-compatible `session_dir` alias and auto-discovery of `~/.claude/projects/`.
- **Auto-tagging**: Regex heuristics in `internal/tagging` detect session categories (debugging, refactoring, feature, testing, docs, config) during sync; stored in `session_tags` table.
- **Content-type classification**: Messages classified as `text`/`code`/`tool`/`reasoning` based on message content types during sync. Tool content is indexed in separate `search_items` rows with `content_type='tool'`. Pi agent reasoning blocks are captured when `index_reasoning=true` (default off) in the input manifest and indexed with `content_type='reasoning'`. Sync writes only to `search_items`; the `session_events` table was dropped in migration v5.
- **Split FTS by retrieval semantics**: tool content (`content_type='tool'`) lives in a separate FTS5 index `tool_fts` (tokenizer `trigram`, substring/exact match for paths/commands/errors); prose content (text, code, reasoning) lives in `messages_fts` (`porter unicode61`). Migration v4 branched the triggers by content type. Migration v7 updated the triggers to route 'reasoning' alongside 'text'/'code' to `messages_fts`. `--content-type tool` queries `tool_fts`; prose queries `messages_fts`; an unfiltered query merges both via Reciprocal Rank Fusion (RRF, k=60), which fuses by rank position, not score magnitude, and is immune to incomparable cross-tokenizer BM25 scales. The trigram tokenizer matches substrings of ≥3 characters, so tool queries shorter than 3 characters will match zero results.
- **Pure Go SQLite**: `modernc.org/sqlite` — no CGO, trivially cross-compilable.
- **Connection pragmas via `_pragma`**: `modernc.org/sqlite` honors DSN pragmas only in the `_pragma=name(value)` form; the mattn-style `_name=value` (e.g. `_busy_timeout=5000`, `_journal_mode=WAL`) is silently ignored, which had left the DB in rollback (delete) journal mode with a zero busy timeout — the root cause of `database is locked` (SQLITE_BUSY) errors. Both connections in `internal/storage/storage.go` set `_pragma=journal_mode(WAL)`, `_pragma=synchronous(NORMAL)`, and `_pragma=busy_timeout(5000)` (read-only sets only the busy timeout; journal mode is persisted in the file). Always use `_pragma=name(value)` for any new connection pragma here.
- **Autoupdate**: `picokit/autoupdate` fetches and stages the latest GitHub release in the background; `run()` waits up to 10s after the command completes so short-lived commands don't kill the download before it finishes.
- **Schema migration rule**: Every new table or column MUST be introduced as a new migration version (increment the version number and add a new version-check block in `SetupSchema()`). Never modify existing migration blocks — existing databases that already passed that version will never re-run them. Migration v5 drops the phantom `session_events` table (and its indexes `idx_session_events_order` and `idx_session_events_project`) — the table was write-only dead weight after structured-stats filtering was removed. Migration v6 drops the phantom `search_items.source_metadata` column via `ALTER TABLE ... DROP COLUMN` — it had a setter but zero production callers and was never read.
- **F0a rich capture (migration v8)**: readers extract per-message identity and tool metadata BEFORE serialization/cleaning destroys the evidence — `uuid` (record uuid; tool blocks get stable `#tN`/`#rN` suffixes by block index), `tool_name`, `command_head`, `is_error` (`*bool`, three-valued: tool_result blocks carry it and it is paired back onto the tool_use message cross-record via `tool_use_id`), and `was_interrupted` (detected on raw content before `CleanContent` strips "Request interrupted"). Persisted to `search_items` (`extraction_version`, `was_interrupted` columns) and the perennial `tool_events` satellite table (`UNIQUE(source_path, ordinal)`, no CASCADE lifecycle — only `purge` deletes from it, explicitly). Claude reader only; Pi/OpenCode emit zero values and stay on the legacy path. Design: `docs/superpowers/specs/2026-07-17-pattern-discovery-northstar-design.md`.
- **F0b perennial sync**: the DB is the perennial event store — session JSONL files expire (~30 days), indexed sessions survive them. Session files where EVERY message has a uuid sync append-only (no DELETE; `INSERT OR IGNORE` + UNIQUE constraints; row ids stable forever), with a one-time transition cleanup of legacy uuid-NULL rows per file. Files with any uuid-less message (Pi/OpenCode, legacy Claude) keep wipe-and-reload. `rebuild` is NON-destructive: re-derives both FTS indexes from `search_items` via FTS5 external-content `'rebuild'` in one transaction (`RebuildFTS()`), then runs incremental sync — it never deletes rows and never re-reads disk as source of truth. `purge --before` is the only deletion path and deletes `tool_events` satellites explicitly in the same transaction (no CASCADE).
- **F1 exit code mining**: during sync, Bash tool_result text is parsed via regex patterns (case-insensitive matches on `exit code N` / `Exit code: N` / `returned N`; note tool text is capped at 4000 runes by toolfmt, so a code beyond the cap yields NULL indistinguishable from no match) and the extracted exit code is stored in the `tool_events.exit_code` column (migration v8; NULL for non-Bash tools or no match). The `patterns` command aggregates tool_events by (tool_name, command_head) for commands or (tool_name, is_error, exit_code) for failures, returning top N sorted by frequency with optional filters by project, session tag, and time window. Coverage metric reports the count of events with non-NULL is_error (signalled events) against the total failure count in the result set.
- **F2 template mining (migration v10)**: unsupervised Drain-inspired template miner (`internal/templates/Miner`) discovers recurring error patterns from tool output during sync. Miner uses fixed-depth token prefix clustering (depth=2) to group messages; beyond the prefix, numeric/path/UUID tokens become `<*>` variables. Error-bearing lines (is_error=true) are extracted per tool via `ExtractErrorLines` (calibrated for Bash, Go, others) and deterministically mined with SHA256 signature. Templates stored in `message_templates` (signature, template_text, occurrence_count, first_seen, last_seen) joined via `template_matches` (template_id, source_path, ordinal, item_uuid) with UNIQUE constraint for idempotency. Mining runs inside `SyncFiles` transaction; re-syncing increments occurrence_count only for new matches (detected via INSERT OR IGNORE). Query method `AggregateTemplates(opts)` filters by min_support (default 3), project, date range; `patterns --kind templates [--min-support N]` exposes results in text/JSON/robot formats.
- **F3 correction detection (migration v11)**: deterministic message-level correction detection as a funnel for agent-classification loops. Four detectors: (1) bilingual correction lexicon (es+en, confidence 0.8), (2) interrupt flags from F0a (confidence 0.5), (3) permission denials ("denied"/"rechaza", confidence 0.4), (4) rephrase-similarity via Jaccard ≥0.6 (confidence 0.6). All pure Go, no ML. Detection runs at sync time; results stored in perennial `correction_signals` (UNIQUE(source_path, ordinal, detector), migration v11). Query method `AggregateCorrections(opts)` groups by ordinal, filters by project and min-confidence, returns top N with detector names and max confidence. `patterns --kind corrections [--min-confidence F]` exposes candidates for F3b agent labeling. Calibration procedure in `docs/eval/corrections-calibration.md` (hand-label 50 candidates to measure per-detector precision before F3b launch). Known limitation: Spanish false positive "no, eso no es un bug, es esperado" (acceptable v1 trade-off).
- **F3b classification loop checkpoint semantics (migration v12)**: `annotations` table is append-and-replace (INSERT OR REPLACE on UNIQUE key), keyed by (source_path, ordinal, kind). Agent loop queries `patterns --kind corrections --pending` to get un-annotated candidates; after annotating a batch, the next query automatically resumes from where it left off (LEFT JOIN filter). Crash-safe: re-running the loop command always shows correct pending state. Labels free-form in v1; `label_enum` table (enum constraint) is a future slice post-calibration, added in a new migration that pre-fills the enum from observed labels and rejects new labels outside the enum.
- **Early input validation**: CLI commands validate flag values (e.g. `--format`) before opening the database, so invalid inputs fail fast without side effects.
- **Coverage gate**: CI enforces ≥85% aggregate statement coverage via `go test ./... -race -coverprofile`. Local check: `bash scripts/check-coverage.sh`. Tests that depend on local machine state (e.g. `~/.config/backscroll/projects.toml`) must use `t.Setenv("HOME", tempDir)` to stay reproducible on CI. Likewise, `InputsDir` branches requiring `BACKSCROLL_CONFIG_DIR` to be unset must set it to `""` via `t.Setenv`. To test the `Validate` orphan path, insert directly into `search_items` without a matching `indexed_files` row.
- **Zero-result guidance**: when `search`/`list` return no rows, actionable suggestions (`--all-projects`, `--content-type tool`, `backscroll status`) are printed to STDERR — never STDOUT, so `--json` stays a clean empty payload.
- **Robot output contract**: `search --robot` emits `result_N_field=value` lines exactly once-wrapped (the robot path writes lines directly; passing pre-formatted lines through the picokit formatter double-wraps them as `result_N=result_N_field=...`).
- **Cross-host project identity**: `projects.Identify()` normalizes session cwd against registry roots by matching root tails (≥2 components, case-insensitive), so `/home/shared/<proj>` sessions resolve against `/Users/Shared/<proj>` roots on a synced index. Registry roots should keep distinct suffixes — two projects whose roots share the same trailing components could misbucket.
- **Recall eval-set**: `docs/eval/queries.toml` (~20 real mined queries with `expected_match` ground truth) + `scripts/eval.sh` compute recall@5; a query counts only if its expected target appears in the top 5. Local regression gate, not a required CI step.

## Dependencies

- `github.com/spf13/cobra` — CLI argument parsing with subcommands
- `modernc.org/sqlite` — Pure Go SQLite with FTS5, WAL mode (no CGO)
- `github.com/pelletier/go-toml/v2` — TOML config parsing
- `github.com/yuin/goldmark` — Markdown parsing for plan indexing
- `github.com/pablontiv/picokit` — Output formatting (text/robot/JSON), file hashing, path security, autoupdate (v0.5+ is a zero-dependency module)
- `crypto/sha256` (stdlib) — SHA-256 hashing for incremental sync deduplication
- `io/fs` + `path/filepath` (stdlib) — Recursive directory traversal
- `regexp` (stdlib) — Noise filter patterns (compiled at init)
- `encoding/json` (stdlib) — Defensive JSONL deserialization with RawMessage

## Project Documentation

- `docs/research/` — Structured research documents: feasibility study and architecture decisions
- `docs/roadmap/` — Roadmap decomposition (O01–O06): outcomes and tasks with frontmatter metadata
- `.claude/skills/backscroll/` — Claude Code skill for `/backscroll` (distributed to `~/.claude/skills/` via pre-push hook)
- `inputs/` — Shipped input presets (`claude.inputs.toml`, `pi.inputs.toml`, `decisions.inputs.toml`, `opencode.inputs.toml`); copied to `<config_dir>/backscroll/inputs/` by `install.sh` and the pre-push hook (skips if already present; `BACKSCROLL_FORCE_INPUTS=1` to overwrite)
- Documentation is written in a mix of Spanish and English (roadmap fields like `estado`, `tipo` are in Spanish)

## Code Style

- `gofmt` for formatting
- `go vet` for static analysis
- Standard Go conventions: exported identifiers documented, unexported identifiers clear from context

## Commit Convention

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`type(scope): description`).

| Type | Semver Impact | When to use |
|------|--------------|-------------|
| `feat` | minor | New user-facing functionality |
| `fix` | patch | Bug fix |
| `refactor` | none | Internal restructuring, no behavior change |
| `perf` | patch | Performance improvement |
| `test` | none | Adding or updating tests |
| `docs` | none | Documentation only |
| `chore` | none | Build, CI, dependency updates |

Breaking changes use `!` suffix (e.g., `feat!:`) for major version bumps.

## Release Flow

Releases are fully automated via CI. On every push to `main`, CI analyzes conventional commits since the last tag, calculates the next semver version, claims it via atomic `git push origin <tag>`, injects the version into binaries at build time via `-ldflags "-X main.version={{.Version}}"`, builds multi-platform binaries via goreleaser, and creates a GitHub Release.

No manual release steps are needed — just push to `main` with conventional commit messages. Tags follow `v{VERSION}` format.

## CI/CD

Workflows delegate to [pablontiv/crossbeam](https://github.com/pablontiv/crossbeam) reusable workflows at `@v1`:

| Workflow | Crossbeam caller |
|---|---|
| `ci.yml` | `go-ci.yml`, `gitleaks.yml`, `go-release.yml` |
| `codeql.yml` | `codeql.yml` |
| `scorecard.yml` | `scorecard.yml` |

## Config Resolution Order

1. `./backscroll.toml` (current directory)
2. `~/.config/backscroll/config.toml`
3. Environment variables: `BACKSCROLL_DATABASE_PATH`, `BACKSCROLL_SESSION_DIRS`
4. Defaults: `~/.backscroll.db`, current directory

## Package Layout

```
github.com/pablontiv/backscroll/cmd/backscroll         — CLI entrypoint
github.com/pablontiv/backscroll/internal/config        — Config structs and resolution
github.com/pablontiv/backscroll/internal/input_config  — Input manifest loading, discovery, and legacy session-dirs compatibility
github.com/pablontiv/backscroll/internal/models        — Domain types and SearchEngine interface
github.com/pablontiv/backscroll/internal/sync          — Session parsing and noise filtering
github.com/pablontiv/backscroll/internal/plans         — Markdown plan parsing
github.com/pablontiv/backscroll/internal/tagging       — Heuristic session auto-tagging
github.com/pablontiv/backscroll/internal/sources       — External source parsers + SourceRegistry
github.com/pablontiv/backscroll/internal/templates     — F2 Drain-inspired miner: Miner, ProcessLine, ExtractErrorLines
github.com/pablontiv/backscroll/internal/corrections   — F3 correction-signal detectors: lexicon (es+en), interrupt, denial, rephrase-similarity; registry pattern for deterministic, pluggable detectors
github.com/pablontiv/backscroll/internal/storage       — Database schema, migrations v1–v11, FTS5 indexes
github.com/pablontiv/backscroll/internal/projects      — Project identity registry
github.com/pablontiv/backscroll/internal/reader        — Direct session file reader
github.com/pablontiv/backscroll/internal/readers       — SessionReader interface, Registry, ClaudeReader (text+tool_use+tool_result), PiReader (text+toolCall+custom results), OpenCodeReader (text+tool state.input+state.output); toolfmt serializer
```
