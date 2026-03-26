# Backscroll Library Refactor — Design Spec

## Context

Backscroll is currently a standalone CLI tool. Kedral's agnostic engine needs programmatic access to backscroll's session parsing, noise filtering, and FTS5 search — without subprocess overhead. The dependency chain is:

```
rootline domains (✅ done) → kedral agnostic engine (blocked) → this refactor
```

This refactor is the **next bottleneck** in the ecosystem pipeline.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Crate layout | Single crate, lib + bin | Simplest change, no workspace overhead |
| API surface | Core parsing + storage | Kedral needs the full pipeline: parse → sync → search |
| Database sharing | Separate databases | Backscroll = lite, Kedral = enterprise; each owns its index |
| Error handling | Keep miette everywhere | Avoids churn; Kedral can use miette too |
| Approach | Minimal extraction | Wire `lib.rs` as re-export facade; zero logic changes |

## Changes

### 1. `Cargo.toml` — Add lib + bin targets

```toml
[lib]
name = "backscroll"
path = "src/lib.rs"

[[bin]]
name = "backscroll"
path = "src/main.rs"
```

No dependency changes. `clap`, `figment`, and `tracing-subscriber` remain non-optional for now (see Deferred Items).

### 2. New `src/lib.rs` — Re-export facade

```rust
#![forbid(unsafe_code)]

pub mod core;
pub mod storage;
```

Exposes `backscroll::core::*` (types, traits, parsing, tagging) and `backscroll::storage::sqlite::Database`.

### 3. `src/main.rs` — Consume the library

Change module declarations to library imports:

```rust
// Before:
mod core;
mod storage;

// After:
use backscroll::core;
use backscroll::storage;
```

Keep `config` and `output` as local CLI-only modules:

```rust
mod config;  // CLI-only: figment-based config resolution
mod output;  // CLI-only: text/json/robot formatting
```

Update **all** `crate::core::*` and `crate::storage::*` references across binary-side modules to `backscroll::core::*` / `backscroll::storage::*`. This includes:

- `main.rs` — module declarations and all `crate::core::*` / `crate::storage::*` imports
- `output.rs` — uses `crate::core::SearchResult` (must become `backscroll::core::SearchResult`)
- Any inline `crate::core::reader::read_session()` calls in `main.rs`

### 4. No changes to library-side modules

These files remain untouched — they're already `pub` and well-structured. **Why no changes?** After the split, `crate::` in library modules refers to the library crate (where `core` and `storage` live), so all existing `crate::core::*` references in these files resolve correctly without modification:

- `src/core/mod.rs` — All types and `SearchEngine` trait already pub
- `src/core/sync.rs` — `parse_sessions()`, `filter_noise()`, `compute_hash()` already pub
- `src/core/models.rs` — `SessionRecord`, `MessageContent` already pub
- `src/core/plans.rs` — `parse_plan()` already pub
- `src/core/reader.rs` — `read_session()` already pub
- `src/core/tagging.rs` — Tagging functions already pub
- `src/storage/sqlite.rs` — `Database` and `SearchEngine` impl already pub

### 5. Visibility cleanup (optional, minor)

`config.rs` and `output.rs` currently use `pub` visibility. Since they stay in `main.rs` as local modules, their visibility is effectively scoped. No changes required, but could add `pub(crate)` for clarity.

## Public API Surface

After the refactor, library consumers (Kedral) can:

```rust
use backscroll::core::{ParsedFile, ParsedMessage, SearchResult, SearchEngine, SearchParams};
use backscroll::core::sync::{parse_sessions, filter_noise};
use backscroll::core::plans::parse_plan;
use backscroll::core::tagging;
use backscroll::storage::sqlite::Database;

// Open a database (separate from CLI's database)
let db = Database::open("/path/to/kedral.db")?;
db.setup_schema()?;

// Parse and sync sessions
let hashes = db.get_file_hashes()?;
let files = parse_sessions(&session_dir, &hashes, false)?;
db.sync_files(files)?;

// Search
let params = SearchParams { limit: 10, ..Default::default() };
let results = db.search("error handling", &params)?;
```

Thread safety: `LazyLock` patterns in `sync.rs` (noise filter regexes) already support concurrent cross-thread usage.

## Testing Strategy

- **Existing CLI tests** (`tests/cli.rs`): Unchanged, continue testing the binary
- **Existing unit tests**: Unchanged, co-located in each module
- **New library integration test**: Add `tests/lib_api.rs` that imports `backscroll::core` and `backscroll::storage` directly, exercises parse → sync → search pipeline to verify the public API surface compiles and works

## Verification

1. `just check` — rustfmt, clippy, cargo check all pass
2. `just test` — all existing tests pass
3. New `tests/lib_api.rs` passes
4. `cargo doc --no-deps` — public API renders correctly
5. Manual: verify `cargo build --release` produces the same binary

## Deferred Items (Future Work)

These are explicitly **not** part of this refactor. They should be revisited once Kedral is consuming the library and we have real consumer feedback:

1. **API redesign** — Builder pattern, convenience wrappers (e.g., `Backscroll::new(path).sync().search("query")`)
2. **Feature-gated CLI deps** — Move `clap`, `figment`, `tracing-subscriber` behind a `cli` feature flag so library consumers don't compile CLI-only dependencies
3. **Flattened re-exports** — `backscroll::SearchEngine` instead of `backscroll::core::SearchEngine` for ergonomic imports
4. **Library-specific error types** — `thiserror`-based errors in the library with `miette` only in the CLI layer
5. **Config as library module** — Expose backscroll's config/path resolution for consumers who want it
