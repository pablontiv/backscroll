---
estado: Specified
tipo: task
---
# T001: Remove public read command

**Contribuye a**: preserve the DB-as-source-of-truth invariant by removing the public direct-file read path and replacing path/session lookup with a SQLite-backed search filter.

## Preserva

- INV1: Existing search/list/topics/resume workflows remain unchanged.
  - Verificar: existing CLI tests for search/list/topics/resume still pass.
- INV2: Canonical ingestion remains manifest-driven and user-scoped.
  - Verificar: no implicit Claude/Pi fallback, no `session_dirs` ingestion path, and no `sync --path` reintroduction.
- INV3: Search ranking, embeddings, hybrid RRF, and sync parsing internals are not refactored for this task.
  - Verificar: changes are limited to CLI surface, search filter plumbing for `source_path`, tests, and docs/skill.

## Contexto

`backscroll read` currently bypasses the SQLite source of truth:

- CLI command: `src/main.rs` declares `Commands::Read { path }`.
- Handler: `src/main.rs` calls `backscroll::core::reader::read_input_file(path, &input_config)`.
- Direct reader: `src/core/reader.rs` discovers and parses physical files via input manifests, without consulting the DB and without auto-sync.

Other public read/query workflows (`search`, `resume`, `list`, `topics`, `insights`, `export`, `status`, `reindex`) already auto-sync before consulting SQLite. The direct `read` path creates stale reads and violates the project invariant that user-visible data comes from ingestion into SQLite.

Recent local investigation found uncommitted changes that document/test `backscroll read <PATH|SESSION_UUID>` plus a helper `Database::find_session_paths_by_session_id()`, but the CLI does not integrate it and `cargo test test_cli_read_by_session_uuid --test cli` fails because the UUID is treated as a path. Do not continue that direction unless product scope changes explicitly.

Decision for this task: remove or hide `backscroll read` as a public command. Do not implement or document `read UUID`. Provide the replacement as a DB-backed `backscroll search` filter over `search_items.source_path`.

## Alcance

**In**:
1. Remove or hide `Commands::Read` from the public CLI help.
2. Stop recommending `backscroll read PATH` or `backscroll read UUID` in the Backscroll skill and user docs.
3. Add a path-based lookup path through SQLite, preferably `backscroll search ... --source-path <PATH_OR_PATTERN>` backed by `search_items.source_path`.
4. Add/adjust CLI tests proving public help/docs behavior and DB-backed path lookup.
5. Reconcile or remove local tests/docs that currently promise `read <PATH|SESSION_UUID>`.

**Out**:
- No direct-file public read path.
- No `read UUID` feature.
- No implicit Claude/Pi parser fallback.
- No reintroduction of `sync --path`, legacy `session_dirs` ingestion, or cwd-local input manifests as canonical ingestion.
- No refactor of sync, ranking, hybrid search, embeddings, or input parser internals beyond the minimal filter plumbing needed for `source_path`.

## Estado inicial esperado

- `backscroll read` is public and parses files without DB/sync.
- `src/core/reader.rs` provides a direct manifest-backed file reader.
- Docs/skill may still mention `backscroll read PATH` and local dirty docs may mention `read <PATH|SESSION_UUID>`.
- A local `test_cli_read_by_session_uuid` currently fails if present.

## Criterios de Aceptación

- `backscroll --help` no longer shows `read` as a public command, or `read` is hidden/gated and cannot be used as the recommended direct-file workflow.
- The Backscroll skill does not recommend `backscroll read PATH` or `backscroll read UUID`; it uses `search`/DB-backed lookup instead.
- User docs (`README.md`, `docs/read.md` or replacement docs) do not advertise direct-file `read` as the primary lookup workflow.
- Path-based session lookup works through SQLite, using `search_items.source_path` via a documented `backscroll search` filter.
- Tests cover the replacement lookup and the absence/gating of public `read`.
- `cargo test test_cli_read_by_session_uuid --test cli` is removed, renamed, or changed so it no longer expects public `read UUID`.
- `just check` passes.
- `just test` passes.

## Fuente de verdad

- `src/main.rs`
- `src/core/reader.rs`
- `src/core/mod.rs`
- `src/storage/sqlite.rs`
- `tests/cli.rs`
- `README.md`
- `docs/read.md`
- `.claude/skills/backscroll/SKILL.md`
