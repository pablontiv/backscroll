# Backscroll Library Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract backscroll's core and storage modules into a library target so Kedral can consume them as a Rust dependency.

**Architecture:** Single crate with both `[lib]` and `[[bin]]` targets. `src/lib.rs` re-exports `core` and `storage` modules. `main.rs` becomes a thin CLI wrapper that imports from the library. Config and output remain CLI-only local modules.

**Tech Stack:** Rust 1.85+, edition 2024, rusqlite (bundled), miette, clap

---

### Task 1: Add lib + bin targets to Cargo.toml

**Files:**
- Modify: `Cargo.toml` (add `[lib]` and `[[bin]]` sections)

- [ ] **Step 1: Add the [lib] and [[bin]] sections to Cargo.toml**

Insert after the `exclude` array (after line 21), before `[dependencies]`:

```toml
[lib]
name = "backscroll"
path = "src/lib.rs"

[[bin]]
name = "backscroll"
path = "src/main.rs"
```

- [ ] **Step 2: Verify Cargo.toml parses correctly**

Run: `cd /opt/backscroll && cargo metadata --format-version 1 --no-deps | head -c 200`

Expected: JSON output with both lib and bin targets listed, no parse errors.

- [ ] **Step 3: Commit**

```bash
git add Cargo.toml
git commit -m "feat: add lib + bin targets to Cargo.toml for library refactor"
```

---

### Task 2: Create src/lib.rs re-export facade

**Files:**
- Create: `src/lib.rs`

- [ ] **Step 1: Create src/lib.rs**

```rust
#![forbid(unsafe_code)]

pub mod core;
pub mod storage;
```

This file re-exports the `core` and `storage` modules as the public library API. `config` and `output` are intentionally excluded — they stay as CLI-only modules in `main.rs`.

- [ ] **Step 2: Verify it compiles (expect errors in main.rs)**

Run: `cd /opt/backscroll && cargo check --lib 2>&1 | head -20`

Expected: The library target compiles. There may be warnings but no errors in the lib itself. (The binary will have errors because `main.rs` still declares `mod core; mod storage;` which now conflict — that's fixed in Task 3.)

- [ ] **Step 3: Commit**

```bash
git add src/lib.rs
git commit -m "feat: add lib.rs re-export facade for core and storage modules"
```

---

### Task 3: Update main.rs to consume the library

**Files:**
- Modify: `src/main.rs`

This is the most detailed task. We replace local module declarations with library imports and update all `crate::core` / `crate::storage` references.

- [ ] **Step 1: Replace module declarations (lines 3-6)**

Change:

```rust
mod config;
mod core;
mod output;
mod storage;
```

To:

```rust
mod config;
mod output;
```

The `core` and `storage` modules now live in the library crate, not as local modules.

- [ ] **Step 2: Update the import block (lines 8-16)**

Change:

```rust
use crate::core::plans::parse_plan;
use crate::core::sync::parse_sessions;
use crate::core::{SearchEngine, SearchParams};
use crate::output::{OutputFormat, OutputOptions, format_results};
use clap::{Parser, Subcommand};
use config::Config;
use miette::Result;
use std::path::PathBuf;
use storage::sqlite::Database;
```

To:

```rust
use backscroll::core::plans::parse_plan;
use backscroll::core::sync::parse_sessions;
use backscroll::core::{SearchEngine, SearchParams};
use backscroll::storage::sqlite::Database;
use crate::output::{OutputFormat, OutputOptions, format_results};
use clap::{Parser, Subcommand};
use config::Config;
use miette::Result;
use std::path::PathBuf;
```

Note: `crate::output` stays as `crate::` because `output` is still a local module. `config::Config` also stays unchanged (local module, relative import). `storage::sqlite::Database` becomes `backscroll::storage::sqlite::Database`.

- [ ] **Step 3: Update the inline crate::core reference on line 462**

Change:

```rust
            let messages = crate::core::reader::read_session(path)?;
```

To:

```rust
            let messages = backscroll::core::reader::read_session(path)?;
```

- [ ] **Step 4: Verify main.rs compiles**

Run: `cd /opt/backscroll && cargo check 2>&1 | head -20`

Expected: May still fail due to `output.rs` (fixed in Task 4). If only output.rs errors remain, proceed to Task 4.

- [ ] **Step 5: Commit**

```bash
git add src/main.rs
git commit -m "refactor: update main.rs to consume library instead of local modules"
```

---

### Task 4: Update output.rs to use library import

**Files:**
- Modify: `src/output.rs:1`

- [ ] **Step 1: Change the import on line 1**

Change:

```rust
use crate::core::SearchResult;
```

To:

```rust
use backscroll::core::SearchResult;
```

- [ ] **Step 2: Verify everything compiles clean**

Run: `cd /opt/backscroll && cargo check 2>&1`

Expected: No errors. Possibly clippy warnings (addressed in Task 5).

- [ ] **Step 3: Commit**

```bash
git add src/output.rs
git commit -m "refactor: update output.rs to import SearchResult from library"
```

---

### Task 5: Run full check suite

**Files:** None (verification only)

- [ ] **Step 1: Run rustfmt check**

Run: `cd /opt/backscroll && cargo fmt --check`

Expected: No formatting issues.

- [ ] **Step 2: Run clippy**

Run: `cd /opt/backscroll && cargo clippy -- -D warnings 2>&1 | tail -20`

Expected: No errors or warnings.

- [ ] **Step 3: Run all existing tests**

Run: `cd /opt/backscroll && cargo test --all-features 2>&1 | tail -30`

Expected: All existing tests pass. No regressions.

- [ ] **Step 4: Fix any issues found**

If fmt/clippy/tests fail, fix the issues before proceeding. Common issues:
- Import ordering (rustfmt may want `backscroll::` imports grouped differently)
- Clippy may flag unused imports if any were missed

- [ ] **Step 5: Commit fixes if any**

```bash
git add src/main.rs src/output.rs src/lib.rs
git commit -m "fix: resolve lint/test issues from library refactor"
```

---

### Task 6: Add library integration test

**Files:**
- Create: `tests/lib_api.rs`

- [ ] **Step 1: Write the library API integration test**

```rust
//! Integration test verifying the public library API surface.
//! Exercises the parse → sync → search pipeline as a library consumer (like Kedral).

use backscroll::core::sync::{filter_noise, parse_sessions};
use backscroll::core::{ParsedFile, SearchEngine, SearchParams, SearchResult};
use backscroll::storage::sqlite::Database;
use std::collections::HashMap;
use std::fs;
use tempfile::tempdir;

#[test]
fn test_library_parse_sync_search_pipeline() {
    // Setup: create a temp directory with a JSONL session file
    let session_dir = tempdir().unwrap();
    let db_dir = tempdir().unwrap();
    let db_path = db_dir.path().join("lib_test.db");

    let session_content = r#"{"type":"human","message":{"role":"human","content":"How do I fix the authentication bug?"},"timestamp":"2026-03-01T10:00:00Z","session_id":"test-session-1"}
{"type":"assistant","message":{"role":"assistant","content":"The authentication bug is caused by an expired token. You need to refresh the OAuth token before making the API call."},"timestamp":"2026-03-01T10:01:00Z","session_id":"test-session-1"}"#;

    fs::write(session_dir.path().join("session.jsonl"), session_content).unwrap();

    // Step 1: Open database and setup schema (as a library consumer would)
    let db = Database::open(db_path.to_str().unwrap()).unwrap();
    db.setup_schema().unwrap();

    // Step 2: Parse sessions
    let hashes: HashMap<String, String> = db.get_file_hashes().unwrap();
    let files: Vec<ParsedFile> =
        parse_sessions(session_dir.path().to_str().unwrap(), &hashes, false).unwrap();
    assert!(!files.is_empty(), "Should parse at least one file");

    // Step 3: Sync to database
    db.sync_files(files).unwrap();

    // Step 4: Search
    let params = SearchParams {
        limit: 10,
        ..SearchParams::default()
    };
    let results: Vec<SearchResult> = db.search("authentication", &params).unwrap();
    assert!(!results.is_empty(), "Should find results for 'authentication'");
}

#[test]
fn test_filter_noise_exposed() {
    // Verify filter_noise is accessible as a library function
    let clean = filter_noise("This is clean text");
    assert!(clean.is_some());

    // Noise should be filtered
    let noisy = filter_noise("<system-reminder>internal noise</system-reminder>");
    // filter_noise returns None for pure noise lines
    assert!(noisy.is_none(), "system-reminder tags should be filtered as noise");
}
```

- [ ] **Step 2: Run the new test to verify it passes**

Run: `cd /opt/backscroll && cargo test --test lib_api -- --nocapture 2>&1`

Expected: Both tests pass.

- [ ] **Step 3: Run the full test suite to ensure no regressions**

Run: `cd /opt/backscroll && cargo test --all-features 2>&1 | tail -20`

Expected: All tests pass including the new ones.

- [ ] **Step 4: Commit**

```bash
git add tests/lib_api.rs
git commit -m "test: add library API integration test for parse-sync-search pipeline"
```

---

### Task 7: Verify docs and final check

**Files:** None (verification only)

- [ ] **Step 1: Generate library documentation**

Run: `cd /opt/backscroll && cargo doc --no-deps 2>&1 | tail -10`

Expected: Docs generate without errors. Public API (`backscroll::core`, `backscroll::storage`) is documented.

- [ ] **Step 2: Run the full check suite (equivalent of `just check`)**

Run: `cd /opt/backscroll && cargo fmt --check && cargo clippy -- -D warnings && cargo check 2>&1`

Expected: All clean.

- [ ] **Step 3: Build release binary and verify it works**

Run: `cd /opt/backscroll && cargo build --release 2>&1 && ./target/release/backscroll --help | head -5`

Expected: Binary builds successfully and shows help output identical to before the refactor.

- [ ] **Step 4: Run just test as final confirmation**

Run: `cd /opt/backscroll && just test 2>&1 | tail -20`

Expected: All tests pass.
