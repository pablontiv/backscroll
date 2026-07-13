# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Backscroll is a Go CLI tool that indexes Claude Code, Pi, and OpenCode sessions, plans, and external knowledge sources into SQLite for full-text search (BM25 via FTS5). It treats sessions as an event store with incremental sync via SHA-256 deduplication.

**Status**: Go port complete — `main` branch is the active Go implementation. The Rust implementation is frozen in the `v0` branch.

Implemented: `internal/config`, `internal/input_config`, `internal/models`, `internal/readers`, `internal/sync`, `internal/tagging`, `internal/plans`, `internal/sources`, `internal/storage`, `internal/projects`, `internal/reader`. CLI commands in `cmd/backscroll/` (8 v2 commands via cobra).

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
└── storage/           — SQLite adapter (dual FTS5 indexes: tool_fts + messages_fts, BM25, WAL mode, migrations v1–v7, search_items, session_tags)
```

Eight v2 CLI commands: `list [--project] [--all-projects] [--order timestamp:desc|asc] [--limit] [--offset] [--json]`, `search [--text <query>] [--project] [--all-projects] [--after] [--before] [--limit] [--offset] [--indexed-only] [--json]`, `read --path <path> [--tail <n>] [--semantic] [--pretty]`, `status`, `validate [--indexed-only]`, `rebuild`, `purge --before <date>`, `config [--json]`.

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
github.com/pablontiv/backscroll/internal/storage       — Database schema, migrations v1–v7, FTS5 indexes
github.com/pablontiv/backscroll/internal/projects      — Project identity registry
github.com/pablontiv/backscroll/internal/reader        — Direct session file reader
github.com/pablontiv/backscroll/internal/readers       — SessionReader interface, Registry, ClaudeReader (text+tool_use+tool_result), PiReader (text+toolCall+custom results), OpenCodeReader (text+tool state.input+state.output); toolfmt serializer
```
