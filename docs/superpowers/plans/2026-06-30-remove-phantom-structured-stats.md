# Remove Phantom Structured-Stats Layer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the never-populated structured-stats surface (`stats` command, structured `list` filters, `session_events` query/insert path, and the table itself) and redirect tool-activity use cases to `search --content-type tool`.

**Architecture:** Pure removal + one additive migration. Remove consumers first (CLI), then the storage functions they used, then the sync insert and purge delete, then drop the table via migration V5. Docs/skill updates travel in the same commit as the code that changes each surface, so every commit is pre-push-hook clean.

**Tech Stack:** Go, cobra, modernc.org/sqlite (FTS5), stdlib testing, just.

## Global Constraints

- Schema rule: NEVER edit migration blocks V1–V4. New schema changes get a new `applyVNMigration` + a `schema_versions` row. Current version is V4; the new one is V5.
- Pre-push hook blocks Go-source changes whose push range has no `docs/`, `README.md`, or `CLAUDE.md` change. Every task below that edits Go also edits CLAUDE.md (and README/skill where relevant) in the SAME commit.
- Gates before any push: `just check`, `just test`, `just coverage-check` (≥85% per package, `.coverage-floors.toml`).
- Conventional commits. No AI attribution. End commit body with `Claude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy`.
- `search --content-type tool` (search_items + tool_fts) and `list`/`read`/`status`/`validate`/`rebuild`/`purge`/`config` MUST keep working after every task.

---

### Task 1: Remove the `stats` command

**Files:**
- Delete: `cmd/backscroll/stats.go`
- Modify: `cmd/backscroll/main.go:61-65` (drop `newStatsCmd(stdout, stderr),` from `root.AddCommand(...)`)
- Modify (tests): `cmd/backscroll/main_test.go` — delete `TestStatsCommandExists`, `TestStatsGroupByAgent`, `TestStatsGroupByAgentNoTypeFilter` (the regression test added with the crash fix)
- Modify (docs): `README.md` (remove the `## ...stats` usage block and the `stats` row in the command table), `CLAUDE.md` (command list: 9 → 8 commands; remove the `stats.go` line in Module Layout and the `--group-by`/stats mentions), `.claude/skills/backscroll/SKILL.md` (remove `stats` rows from the command table and the invocation mapping; rewrite workflow 5.2 as a `search --content-type tool` example)

**Interfaces:**
- Consumes: nothing.
- Produces: removal only. After this task `backscroll stats` is an unknown command.

- [ ] **Step 1: Write the failing test** — replace the deleted stats tests with one asserting the command is gone.

```go
// in cmd/backscroll/main_test.go
func TestStatsCommandRemoved(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, stderr, err := runCmd("stats", "--group-by", "agent")
	if err == nil {
		t.Fatal("expected error: stats command should no longer exist")
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("expected 'unknown command' on stderr, got: %q", stderr)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/backscroll/ -run TestStatsCommandRemoved -v`
Expected: FAIL — `stats` still registered, so no error returned.

- [ ] **Step 3: Make the change** — delete `cmd/backscroll/stats.go`, remove the `newStatsCmd(stdout, stderr),` line from `cmd/backscroll/main.go`, and delete the three old stats tests (`TestStatsCommandExists`, `TestStatsGroupByAgent`, `TestStatsGroupByAgentNoTypeFilter`).

```bash
git rm cmd/backscroll/stats.go
# edit main.go: remove the newStatsCmd line from root.AddCommand(...)
# edit main_test.go: remove the three old stats tests
```

- [ ] **Step 4: Update docs/skill in the same change** — remove the `stats` section + command-table row from `README.md`; update `CLAUDE.md` (8 commands, drop `stats.go` Module Layout line, drop `--group-by`/stats decision mentions); rewrite the skill's stats rows/mapping and workflow 5.2 to use `search --content-type tool`.

- [ ] **Step 5: Run tests + gates**

Run: `go test ./cmd/backscroll/ -run TestStatsCommandRemoved -v && just check && just test`
Expected: PASS; build compiles (nothing else references `newStatsCmd`/`groupEvents`/`StatEntry`).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "$(printf 'refactor(cli): remove the phantom stats command\n\nstats was built entirely on never-populated session_events columns;\nevery output was <unknown> or message:N. Tool-activity use cases move\nto `search --content-type tool`.\n\nClaude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy')"
```

---

### Task 2: Remove the structured `list` filter path

**Files:**
- Modify: `cmd/backscroll/list.go` — remove the `eventType`/`toolName`/`command` vars (25-27), the `--type`/`--tool`/`--command` flags (64-66), the structured branch (`if eventType != "" || toolName != "" || command != "" { ... ListSessionEventsV2 ... }`, ~103-122), and drop those params from `runList`'s signature (51, 73). `list` keeps only the `ListSessions` path.
- Modify (tests): `cmd/backscroll/main_test.go` / `list` tests — delete any asserting `list --type/--tool/--command`.
- Modify (docs): `CLAUDE.md` — update the `list` flag list (remove `--type`/`--tool` from the `list` command signature line).

**Interfaces:**
- Consumes: `db.ListSessions(project, recent)` (unchanged).
- Produces: `list` with no structured filters. `ListSessionEventsV2` now has zero callers.

- [ ] **Step 1: Write the failing test**

```go
// in cmd/backscroll/main_test.go
func TestListStructuredFlagsRemoved(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, stderr, err := runCmd("list", "--type", "tool_call")
	if err == nil {
		t.Fatal("expected error: --type flag should be removed from list")
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected 'unknown flag', got: %q", stderr)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/backscroll/ -run TestListStructuredFlagsRemoved -v`
Expected: FAIL — `--type` still accepted.

- [ ] **Step 3: Make the change** — in `cmd/backscroll/list.go` delete the three vars, three flags, the structured `if` branch, and the three params from `runList` and its call site. Delete `list --type/--tool/--command` tests.

- [ ] **Step 4: Update CLAUDE.md** — change the `list [...]` signature line to drop `--type`/`--tool`.

- [ ] **Step 5: Run tests + gates**

Run: `go test ./cmd/backscroll/ -run 'TestList' -v && just check && just test`
Expected: PASS; `list` normal path works; build compiles.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "$(printf 'refactor(cli): drop structured list filters (--type/--tool/--command)\n\nThey queried never-populated session_events columns. list keeps its\nsearch_items-backed path.\n\nClaude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy')"
```

---

### Task 3: Remove the storage structured-query functions

**Files:**
- Modify: `internal/storage/queries.go` — delete `ListSessionEventsV2` and `StructuredEventRow` (and any now-unused imports, e.g. `sql` if nothing else uses it — check before removing).
- Modify: `internal/storage/records.go` — delete `QuerySessionEvents`, `SessionEvent`, `SessionEventQuery` (test-only callers; no production caller).
- Modify (tests): `internal/storage/unit_test.go` — delete `TestListSessionEventsV2*` (incl. `TestListSessionEventsV2NullToolNameAndActor` from the crash fix) and `QuerySessionEvents` tests.
- Modify (docs): `CLAUDE.md` — remove the `QuerySessionEvents`/`ListSessionEventsV2` mentions in the Coverage-gate note and Key Design Decisions.

**Interfaces:**
- Consumes: nothing (Tasks 1–2 removed all callers).
- Produces: removal only.

- [ ] **Step 1: Confirm zero production callers remain**

Run: `rg -n "ListSessionEventsV2|QuerySessionEvents|StructuredEventRow|SessionEventQuery" --glob '!*_test.go' internal cmd`
Expected: no matches in `internal/` or `cmd/` Go files (CLAUDE.md hits are fine — fixed in Step 3).

- [ ] **Step 2: Make the change** — delete the functions/types above and their tests. If `database/sql` becomes unused in `queries.go`, remove the import.

- [ ] **Step 3: Update CLAUDE.md** — drop the `QuerySessionEvents`/`ListSessionEventsV2` references.

- [ ] **Step 4: Run gates**

Run: `just check && just test && just coverage-check`
Expected: PASS. If a storage-package floor dips below 85% after deleting tested code, note it — Task 5 also removes code; re-check there. If still below floor, the removed functions were over-represented in coverage; that's acceptable to surface to the user, not to paper over.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "$(printf 'refactor(storage): remove dead session_events query functions\n\nListSessionEventsV2/StructuredEventRow and QuerySessionEvents/SessionEvent\nhad no remaining callers after the CLI surface was removed.\n\nClaude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy')"
```

---

### Task 4: Stop writing session_events (sync + purge)

**Files:**
- Modify: `internal/storage/sync.go` — delete the `INSERT INTO session_events (...)` loop (the per-message block ~82-101).
- Modify: `internal/storage/queries.go` — in the purge path, delete the `DELETE FROM session_events` statement (~620) so purge only touches `search_items`/`indexed_files`.
- Modify (tests): adjust any test asserting `session_events` row counts after sync/purge.
- Modify (docs): `CLAUDE.md` — update the "Content-type classification" decision note (the `claude`/`pi`/`opencode` inputs no longer feed `session_events`; tool content lives only in `search_items`/`tool_fts`).

**Interfaces:**
- Consumes: existing `search_items` insert (unchanged).
- Produces: sync writes only `search_items` (+ tags, indexed_files). `session_events` receives no writes.

- [ ] **Step 1: Write the failing test** — prove sync no longer writes session_events.

```go
// in internal/storage/unit_test.go
func TestSyncDoesNotWriteSessionEvents(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	pf := models.ParsedFile{ /* one record, mirror an existing sync test's fixture */ }
	if err := db.SyncFiles([]models.ParsedFile{pf}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM session_events").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 session_events rows after sync, got %d", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/ -run TestSyncDoesNotWriteSessionEvents -v`
Expected: FAIL — sync still inserts rows (n > 0).

- [ ] **Step 3: Make the change** — remove the `INSERT INTO session_events` loop from `sync.go` and the `DELETE FROM session_events` from the purge path in `queries.go`.

- [ ] **Step 4: Update CLAUDE.md** — revise the content-type classification note.

- [ ] **Step 5: Run gates**

Run: `go test ./internal/storage/ -run TestSyncDoesNotWriteSessionEvents -v && just check && just test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "$(printf 'refactor(storage): stop writing session_events on sync/purge\n\nNothing reads the table anymore. sync writes only search_items; purge no\nlonger references session_events.\n\nClaude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy')"
```

---

### Task 5: Drop the session_events table (migration V5)

**Files:**
- Modify: `internal/storage/migrations.go` — add `applyV5Migration` following the V4 pattern; wire it into `setupSchema` after the V4 block; insert a `schema_versions` row `(5, 'V5 drop phantom session_events', CURRENT_TIMESTAMP, ?)`.
- Modify (tests): `internal/storage/unit_test.go` — add a migration test asserting `session_events` no longer exists after `setupSchema`.
- Modify (docs): `CLAUDE.md` — note migration V5 in the schema-migration design decision; remove `session_events` from any schema/table descriptions.

**Interfaces:**
- Consumes: the existing migration runner / version check in `setupSchema`.
- Produces: schema at version 5 with no `session_events` table.

- [ ] **Step 1: Write the failing test**

```go
// in internal/storage/unit_test.go
func TestMigrationV5DropsSessionEvents(t *testing.T) {
	db, cleanup := newTestDB(t) // newTestDB runs setupSchema to the latest version
	defer cleanup()

	var name string
	err := db.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='session_events'",
	).Scan(&name)
	if err == nil {
		t.Fatalf("session_events table should not exist after V5, but found %q", name)
	}
	// sql.ErrNoRows is the expected outcome.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/ -run TestMigrationV5DropsSessionEvents -v`
Expected: FAIL — table still created by V1 and never dropped.

- [ ] **Step 3: Make the change** — add the V5 migration (do NOT edit V1–V4):

```go
// applyV5Migration drops the phantom session_events table. Nothing reads or
// writes it after the structured-stats surface was removed. Per the schema
// rule, this is a new migration block; V1 still creates the table on the way up.
func applyV5Migration(tx *sql.Tx, checksum string) error {
	stmts := []string{
		`DROP INDEX IF EXISTS idx_session_events_order`,
		`DROP INDEX IF EXISTS idx_session_events_project`,
		`DROP TABLE IF EXISTS session_events`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("V5 migration: %w", err)
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_versions (version, description, applied_at, checksum)
		 VALUES (5, 'V5 drop phantom session_events', CURRENT_TIMESTAMP, ?)`,
		checksum,
	); err != nil {
		return fmt.Errorf("V5 schema_versions: %w", err)
	}
	return nil
}
```

Wire it into `setupSchema` after the `currentVersion == 4` (V4) block, mirroring how V4 is gated (`if currentVersion < 5 { ... applyV5Migration ... }` — match the exact guard style already used for V2–V4).

- [ ] **Step 4: Update CLAUDE.md** — record migration V5 and remove `session_events` from schema descriptions.

- [ ] **Step 5: Run gates**

Run: `go test ./internal/storage/ -run TestMigrationV5DropsSessionEvents -v && just check && just test && just coverage-check`
Expected: PASS, including the per-package coverage floors.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "$(printf 'feat(storage)!: drop phantom session_events table (migration V5)\n\nThe table was write-only dead weight. V5 drops it and its indexes; V1-V4\nare untouched per the schema rule.\n\nBREAKING CHANGE: the stats command and structured list filters are removed;\nuse `search --content-type tool` for tool activity.\n\nClaude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy')"
```

---

### Task 6: Final verification + push

**Files:** none (verification only).

- [ ] **Step 1: Full grep for stragglers**

Run: `rg -n "session_events|ListSessionEventsV2|QuerySessionEvents|newStatsCmd|groupEvents|--group-by" --glob '!docs/**' .`
Expected: no Go-source matches; only this plan/spec under `docs/` may mention them historically.

- [ ] **Step 2: Full gate run**

Run: `just check && just test && just coverage-check`
Expected: all PASS, every package ≥ its floor.

- [ ] **Step 3: Behavior smoke test against the live binary** (after the pre-push hook installs it)

Run: `backscroll stats 2>&1; backscroll search "go test" --all-projects --content-type tool --max-tokens 500 >/dev/null && echo SEARCH_OK`
Expected: `stats` → unknown command; `SEARCH_OK` printed (the replacement path works).

- [ ] **Step 4: Push once** — single push over the whole task range; the pre-push hook runs gates, installs the binary, and (because every commit carried its docs) the docs-update gate passes.

```bash
git push origin main
```

---

## Self-Review

**Spec coverage:** stats removal (T1), structured list removal (T2), storage fn removal (T3), sync/purge stop-write (T4), table drop V5 (T5), docs/skill (folded into T1–T5), `search --content-type tool` preserved + smoke-tested (T6). Demand analysis and risks from the spec are reflected in the coverage-floor watch (T3/T5) and the "surface, don't paper over" note. All spec scope mapped.

**Placeholder scan:** Task 4 Step 1 fixture says "mirror an existing sync test's fixture" — intentional pointer to a real in-repo pattern, not a TODO; the executor copies the nearest `SyncFiles` test's `ParsedFile`. No "TBD"/"add error handling"/"write tests for the above" patterns.

**Type consistency:** `applyV5Migration(tx *sql.Tx, checksum string)` matches the V4 helper shape; `session_events`/`ListSessionEventsV2`/`QuerySessionEvents` names match the codebase grep. Guard style is deferred to "match V2–V4 exactly" because the exact `setupSchema` gating idiom must be read at execution, not guessed.
