# Spec: Backscroll Library Refactor

## Context
Backscroll is currently a standalone CLI tool. Other ecosystem components (like Kedral) need its sophisticated session parsing and noise-filtering logic without the overhead of subprocess calls.

## Ecosystem Impact

> **Priority elevated post-domain types** (2026-03-26): Rootline's `domain` semantic types are now implemented (commit `6593c1c`), enabling consumer tools to discover fields like `lifecycle_state` without knowing localized names. This unblocks Kedral's agnostic engine spec (`kedral-agnostic-engine.md`), which depends on `libbackscroll` for noise-filtered session analysis. Dependency chain:
>
> ```
> rootline domains (✅ done) → kedral agnostic engine (blocked on libbackscroll) → this refactor
> ```
>
> This refactor is now the **next bottleneck** in the ecosystem pipeline.

## Objectives
1. **Library-First Architecture**: Refactor the codebase to support both a library (`lib`) and a binary (`bin`).
2. **Core Abstraction**: Move parsing, noise filtering, and models into `src/lib.rs`.
3. **Consumer Stability**: Expose a stable, thread-safe API for concurrent usage by daemons (e.g., Kedral's watcher).

## Changes

### 3.1 Cargo.toml Refactor
- Add a `[lib]` target.
- Ensure all parsing-related dependencies (`serde`, `regex`) are available to the library.

### 3.2 Refactor `src/main.rs` to `src/lib.rs`
- Extract `core/`, `storage/`, and `models/` modules into the library.
- Keep `main.rs` as a thin CLI wrapper that consumes the library.

### 3.3 Public Public API
Expose the following as the primary interface:
- `pub fn filter_noise(text: &str) -> Option<String>`: Cleans Claude-specific tags and noise.
- `pub fn parse_sessions(dir: &str, ...) -> Result<Vec<ParsedFile>>`: The canonical session parser.
- `pub struct ParsedMessage` and `pub struct ParsedFile`: Standardized data models for AI sessions.

### 3.4 Concurrency
Ensure internal regular expressions and filters are stored in `LazyLock` or similar to allow safe cross-thread usage when imported by long-running daemons.
