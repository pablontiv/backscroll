# Split FTS — Slice 1: Tool-Activity Index — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give tool-call content its own FTS5 index (`tool_fts`) with a path/command-aware trigram tokenizer, moving tool rows out of `messages_fts` so exact path/command/error lookup works and prose search stops being crowded.

**Architecture:** Add a second external-content FTS5 table `tool_fts` (tokenizer `trigram`) over the existing `search_items` content table. Migration v4 creates it, replaces the unconditional sync triggers with `content_type`-branched triggers (tool → `tool_fts`, text/code → `messages_fts`), and repopulates both indexes from `search_items`. `Search` routes by `--content-type`: tool queries hit `tool_fts`, prose queries hit `messages_fts`, and the no-filter path merges both with per-table score normalization.

**Tech Stack:** Go (stdlib `database/sql`, `testing`), `modernc.org/sqlite` (pure-Go SQLite with FTS5, includes the built-in `trigram` tokenizer), cobra CLI.

## Global Constraints

- Schema changes are append-only migrations: add a new `version = 4` block; never edit migrations v1–v3 (CLAUDE.md schema-migration rule).
- Per-package statement coverage floor ≥85% (pkcov pre-push gate + CI `just coverage-check`).
- `gofmt` + `go vet` must pass (`just check`); all tests pass (`just test`).
- Pure Go only — no CGO; do not add dependencies.
- Conventional commits (`type(scope): description`). No AI attribution.
- FTS5 `trigram` tokenizer: matches substrings of **≥3 characters**; tokens shorter than 3 chars cannot match. Do not use porter-style prefix `*` wildcards against `tool_fts`.

---

### Task 1: Migration v4 — create `tool_fts`, branch triggers, repopulate

**Files:**
- Modify: `internal/storage/migrations.go` (add `applyV4Migration`, `sqlV4*` consts, wire into `SetupSchema`)
- Test: `internal/storage/unit_test.go` (add migration test)

**Interfaces:**
- Consumes: existing `search_items` table (`id`, `text`, `content_type`), existing `messages_fts` (external content, `content=search_items`, `content_rowid=id`).
- Produces: virtual table `tool_fts(text)` with `tokenize='trigram'`; vocab `tool_vocab`; triggers `search_items_ai_tool`, `search_items_ai_msg`, `search_items_ad_tool`, `search_items_ad_msg`, `search_items_au_tool`, `search_items_au_msg`. After migration: `messages_fts` holds only rows where `content_type <> 'tool'`; `tool_fts` holds only rows where `content_type = 'tool'`.

- [ ] **Step 1: Write the failing test**

Add to `internal/storage/unit_test.go`:

```go
func TestV4MigrationRoutesToolRowsToToolFTS(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v4.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.SetupSchema(); err != nil {
		t.Fatalf("setup schema: %v", err)
	}

	// Insert one prose row and one tool row directly.
	_, err = db.db.Exec(`INSERT INTO indexed_files(path, hash) VALUES ('p1','h1')`)
	if err != nil {
		t.Fatalf("seed indexed_files: %v", err)
	}
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session','p1',0,'user','architecture decision about retries','text'),
		       ('session','p1',1,'assistant','internal/storage/sync.go','tool')`)
	if err != nil {
		t.Fatalf("seed search_items: %v", err)
	}

	var msgCount, toolCount int
	if err := db.db.QueryRow(`SELECT count(*) FROM messages_fts WHERE messages_fts MATCH '"architecture"'`).Scan(&msgCount); err != nil {
		t.Fatalf("query messages_fts: %v", err)
	}
	if err := db.db.QueryRow(`SELECT count(*) FROM tool_fts WHERE tool_fts MATCH '"sync.go"'`).Scan(&toolCount); err != nil {
		t.Fatalf("query tool_fts: %v", err)
	}

	if msgCount != 1 {
		t.Errorf("messages_fts: want 1 prose hit, got %d", msgCount)
	}
	if toolCount != 1 {
		t.Errorf("tool_fts: want 1 tool hit, got %d", toolCount)
	}

	// The tool row must NOT be in messages_fts (no crowding).
	var leak int
	if err := db.db.QueryRow(`SELECT count(*) FROM messages_fts WHERE messages_fts MATCH '"sync"'`).Scan(&leak); err != nil {
		t.Fatalf("query leak: %v", err)
	}
	if leak != 0 {
		t.Errorf("tool content leaked into messages_fts: got %d hits", leak)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestV4MigrationRoutesToolRowsToToolFTS ./internal/storage/ -v`
Expected: FAIL — `no such table: tool_fts` (table not created yet).

- [ ] **Step 3: Add the v4 SQL constants**

In `internal/storage/migrations.go`, after the `sqlV3` const, add:

```go
const sqlV4ToolFTS = `
CREATE VIRTUAL TABLE IF NOT EXISTS tool_fts USING fts5(
    text,
    content=search_items,
    content_rowid=id,
    tokenize='trigram'
);

CREATE VIRTUAL TABLE IF NOT EXISTS tool_vocab USING fts5vocab(tool_fts, 'row');
`

// Drop the unconditional v1 triggers and replace them with content_type-branched
// triggers: tool rows index into tool_fts, everything else into messages_fts.
const sqlV4Triggers = `
DROP TRIGGER IF EXISTS search_items_ai;
DROP TRIGGER IF EXISTS search_items_ad;
DROP TRIGGER IF EXISTS search_items_au;

CREATE TRIGGER IF NOT EXISTS search_items_ai_tool AFTER INSERT ON search_items
WHEN new.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ai_msg AFTER INSERT ON search_items
WHEN new.content_type <> 'tool' BEGIN
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ad_tool AFTER DELETE ON search_items
WHEN old.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(tool_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ad_msg AFTER DELETE ON search_items
WHEN old.content_type <> 'tool' BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_au_tool AFTER UPDATE ON search_items
WHEN old.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(tool_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO tool_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_au_msg AFTER UPDATE ON search_items
WHEN old.content_type <> 'tool' BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;
`

// Repopulate both indexes from search_items by content_type. 'delete-all' is valid
// for external-content FTS5 tables and resets the index without touching content rows.
const sqlV4Repopulate = `
INSERT INTO messages_fts(messages_fts) VALUES('delete-all');
INSERT INTO messages_fts(rowid, text) SELECT id, text FROM search_items WHERE content_type <> 'tool';
INSERT INTO tool_fts(rowid, text) SELECT id, text FROM search_items WHERE content_type = 'tool';
`
```

- [ ] **Step 4: Add `applyV4Migration` and wire it into `SetupSchema`**

In `internal/storage/migrations.go`, add after `applyV3Migration`:

```go
// applyV4Migration adds the tool_fts index (trigram tokenizer), branches the
// sync triggers by content_type, and repopulates both indexes from search_items.
func (d *Database) applyV4Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(sqlV4ToolFTS); err != nil {
		return fmt.Errorf("create tool_fts: %w", err)
	}
	if _, err := tx.Exec(sqlV4Triggers); err != nil {
		return fmt.Errorf("rebuild triggers: %w", err)
	}
	if _, err := tx.Exec(sqlV4Repopulate); err != nil {
		return fmt.Errorf("repopulate indexes: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV4ToolFTS + sqlV4Triggers))
	checksumHex := fmt.Sprintf("%x", checksum)

	_, err = tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (4, 'V4 tool_fts trigram index', CURRENT_TIMESTAMP, ?)
	`, checksumHex)
	if err != nil {
		return fmt.Errorf("record migration v4: %w", err)
	}

	return tx.Commit()
}
```

In `SetupSchema`, after the version-3 block (line ~59), add:

```go
	// Check if version 4 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 4").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 4: %w", err)
	}

	if count == 0 {
		if err := d.applyV4Migration(); err != nil {
			return err
		}
	}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -run TestV4MigrationRoutesToolRowsToToolFTS ./internal/storage/ -v`
Expected: PASS.

- [ ] **Step 6: Verify the full suite and formatting still pass**

Run: `just check && go test ./internal/storage/...`
Expected: no vet/format errors; storage tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/storage/migrations.go internal/storage/unit_test.go
git commit -m "feat(storage): add tool_fts trigram index via migration v4

Branch sync triggers by content_type: tool rows index into tool_fts
(trigram tokenizer), text/code into messages_fts. Repopulate both from
search_items so existing databases migrate cleanly.

Claude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy"
```

---

### Task 2: Route `--content-type tool` queries to `tool_fts`

**Files:**
- Modify: `internal/storage/search.go` (add a trigram sanitizer; pick the FTS table by content type)
- Test: `internal/storage/unit_test.go`

**Interfaces:**
- Consumes: `tool_fts` from Task 1; `models.SearchOptions` (`ContentType` field, already present).
- Produces: `func sanitizeFTS5QueryTrigram(query string, stopwords map[string]struct{}) string` (quotes each ≥1-char token as a phrase, NO prefix `*`); `Search` queries `tool_fts` + `bm25(tool_fts)` + `snippet(tool_fts, ...)` when `opts.ContentType == "tool"`, otherwise `messages_fts` as today.

- [ ] **Step 1: Write the failing test**

This is the measured regression case: exact path lookup must rank the right file first.

```go
func TestToolSearchRanksExactPathFirst(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "toolsearch.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.SetupSchema(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, _ = db.db.Exec(`INSERT INTO indexed_files(path, hash) VALUES ('p1','h1')`)
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session','p1',0,'assistant','edited internal/storage/hybrid.go','tool'),
		       ('session','p1',1,'assistant','read internal/storage/sync.go','tool'),
		       ('session','p1',2,'assistant','ran go test ./internal/storage/','tool')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	results, err := db.Search("internal/storage/sync.go", models.SearchOptions{ContentType: "tool", Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("want at least 1 result, got 0")
	}
	if !strings.Contains(results[0].Text, "sync.go") {
		t.Errorf("want sync.go ranked first, got %q", results[0].Text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestToolSearchRanksExactPathFirst ./internal/storage/ -v`
Expected: FAIL — query still runs against `messages_fts`, so either zero results or `sync.go` not first.

- [ ] **Step 3: Add the trigram sanitizer**

In `internal/storage/search.go`, after `sanitizeFTS5Query`:

```go
// sanitizeFTS5QueryTrigram builds a MATCH query for the trigram-tokenized
// tool_fts. Unlike the porter sanitizer it does NOT append a prefix wildcard
// (trigram matches substrings directly) and it preserves path/command tokens.
func sanitizeFTS5QueryTrigram(query string, stopwords map[string]struct{}) string {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return ""
	}
	var filtered []string
	for _, t := range tokens {
		if _, ok := stopwords[strings.ToLower(t)]; !ok {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) == 0 {
		filtered = tokens
	}
	var parts []string
	for _, t := range filtered {
		escaped := strings.ReplaceAll(t, `"`, `""`)
		parts = append(parts, fmt.Sprintf(`"%s"`, escaped))
	}
	return strings.Join(parts, " ")
}
```

- [ ] **Step 4: Pick the FTS table by content type in `Search`**

In `internal/storage/search.go`, replace the fixed table references. Near the top of `Search`, after computing `stopwords`, decide the table and query string:

```go
	ftsTable := "messages_fts"
	ftsQuery := sanitizeFTS5Query(query, stopwords)
	if opts.ContentType == "tool" {
		ftsTable = "tool_fts"
		ftsQuery = sanitizeFTS5QueryTrigram(query, stopwords)
	}
```

Then make the MATCH clause, `snippet(...)`, `bm25(...)`, and `FROM ... JOIN` use `ftsTable`. Update:

- Line ~106: ``whereClauses = append([]string{ftsTable + " MATCH ?"}, whereClauses...)``
- In the `sqlQuery` template, change `snippet(messages_fts, 0, ...)` → `snippet(%[1]s, 0, ...)`, `bm25(messages_fts)` → `bm25(%[1]s)`, `FROM messages_fts` → `FROM %[1]s`, and `JOIN search_items si ON messages_fts.rowid = si.id` → `JOIN search_items si ON %[1]s.rowid = si.id`, passing `ftsTable` as the first `fmt.Sprintf` arg (use indexed verbs `%[1]s` for the table and `%[2]s`/`%[3]s` for `tagJoin`/`whereSQL`).

(The `si.content_type = ?` filter at line 77 still applies and is harmless — every row in `tool_fts` already has `content_type='tool'`.)

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -run TestToolSearchRanksExactPathFirst ./internal/storage/ -v`
Expected: PASS — `sync.go` ranked first.

- [ ] **Step 6: Run the storage suite**

Run: `just check && go test ./internal/storage/...`
Expected: PASS (existing prose-search tests unaffected — default `ContentType==""` still uses `messages_fts`).

- [ ] **Step 7: Commit**

```bash
git add internal/storage/search.go internal/storage/unit_test.go
git commit -m "feat(storage): route --content-type tool searches to tool_fts

Tool queries use a trigram-aware sanitizer (no prefix wildcard) and the
tool_fts index, so exact path/command/error lookup ranks correctly.

Claude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy"
```

---

### Task 3: Merge both indexes for the no-content-type ("search everything") path

**Files:**
- Modify: `internal/storage/search.go` (when `opts.ContentType == ""`, query both tables and merge by normalized rank)
- Test: `internal/storage/unit_test.go`

**Interfaces:**
- Consumes: the per-table `Search` path from Task 2.
- Produces: `func (d *Database) searchTable(ftsTable, query string, opts models.SearchOptions) ([]SearchResult, error)` (the existing single-table logic, extracted), and a `Search` that, when `ContentType==""`, calls it for both `messages_fts` and `tool_fts`, min-max normalizes each table's BM25 scores into `[0,1]`, merges, sorts descending, and applies `Limit`/`Offset` in Go.

- [ ] **Step 1: Write the failing test**

```go
func TestSearchEverythingReturnsBothProseAndTool(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "merge.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.SetupSchema(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, _ = db.db.Exec(`INSERT INTO indexed_files(path, hash) VALUES ('p1','h1')`)
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session','p1',0,'user','retry backoff strategy discussion','text'),
		       ('session','p1',1,'assistant','ran retry-backoff.sh and saw retry','tool')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	results, err := db.Search("retry", models.SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	var sawText, sawTool bool
	for _, r := range results {
		if r.ContentType == "text" {
			sawText = true
		}
		if r.ContentType == "tool" {
			sawTool = true
		}
	}
	if !sawText || !sawTool {
		t.Errorf("want both prose and tool hits; sawText=%v sawTool=%v", sawText, sawTool)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestSearchEverythingReturnsBothProseAndTool ./internal/storage/ -v`
Expected: FAIL — default path only queries `messages_fts`, so the tool row is missing.

- [ ] **Step 3: Extract the single-table query into `searchTable`**

Refactor `Search`: move the body that builds and runs the SQL into a new method `searchTable(ftsTable, query string, opts models.SearchOptions) ([]SearchResult, error)` that takes the resolved table name and uses the matching sanitizer (`sanitizeFTS5QueryTrigram` when `ftsTable == "tool_fts"`, else `sanitizeFTS5Query`). It must NOT apply `Limit`/`Offset` when called for a merge — accept a flag or always fetch up to a cap (e.g. `opts.Limit + opts.Offset`, default cap 200) and let the caller paginate.

- [ ] **Step 4: Implement the merge in `Search`**

```go
func (d *Database) Search(query string, opts models.SearchOptions) ([]SearchResult, error) {
	switch opts.ContentType {
	case "tool":
		return d.searchTable("tool_fts", query, opts)
	case "text", "code":
		return d.searchTable("messages_fts", query, opts)
	case "":
		prose, err := d.searchTable("messages_fts", query, withoutPaging(opts))
		if err != nil {
			return nil, err
		}
		tool, err := d.searchTable("tool_fts", query, withoutPaging(opts))
		if err != nil {
			return nil, err
		}
		merged := mergeNormalized(prose, tool)
		return paginate(merged, opts.Limit, opts.Offset), nil
	default:
		return d.searchTable("messages_fts", query, opts)
	}
}
```

Add helpers in `search.go`:

```go
// withoutPaging returns a copy of opts with Limit/Offset cleared so each
// table query returns its full candidate set for cross-table merging.
func withoutPaging(o models.SearchOptions) models.SearchOptions {
	o.Limit = 200
	o.Offset = 0
	return o
}

// mergeNormalized min-max normalizes each slice's BM25 scores into [0,1]
// (BM25 is negative; higher is better) and returns the union sorted by the
// normalized score descending. Cross-index ordering is approximate by design.
func mergeNormalized(a, b []SearchResult) []SearchResult {
	normalize(a)
	normalize(b)
	out := append(append([]SearchResult{}, a...), b...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func normalize(rs []SearchResult) {
	if len(rs) == 0 {
		return
	}
	min, max := rs[0].Score, rs[0].Score
	for _, r := range rs {
		if r.Score < min {
			min = r.Score
		}
		if r.Score > max {
			max = r.Score
		}
	}
	span := max - min
	for i := range rs {
		if span == 0 {
			rs[i].Score = 1
		} else {
			rs[i].Score = (rs[i].Score - min) / span
		}
	}
}

func paginate(rs []SearchResult, limit, offset int) []SearchResult {
	if limit == 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(rs) {
		return nil
	}
	end := offset + limit
	if end > len(rs) {
		end = len(rs)
	}
	return rs[offset:end]
}
```

Add `"sort"` to the import block.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -run TestSearchEverythingReturnsBothProseAndTool ./internal/storage/ -v`
Expected: PASS.

- [ ] **Step 6: Run the full storage + CLI suites**

Run: `just check && go test ./internal/storage/... ./cmd/...`
Expected: PASS. If CLI search tests assert exact ordering against the old single-table path, update them to the merged semantics (document the change in the test comment).

- [ ] **Step 7: Commit**

```bash
git add internal/storage/search.go internal/storage/unit_test.go
git commit -m "feat(storage): merge tool_fts and messages_fts for unfiltered search

No --content-type now queries both indexes and merges by per-table
min-max normalized BM25. Cross-index ordering is approximate; scoped
queries stay exactly ranked.

Claude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy"
```

---

### Task 4: Maintenance — `Validate` and `OptimizeFTS` cover `tool_fts`

**Files:**
- Modify: `internal/storage/queries.go` (`Validate` checks `tool_fts` exists; `OptimizeFTS` optimizes both tables)
- Test: `internal/storage/unit_test.go`

**Interfaces:**
- Consumes: `tool_fts` from Task 1.
- Produces: `Validate` returns an error when `tool_fts` is missing; `OptimizeFTS` runs `optimize` on both `messages_fts` and `tool_fts`.

- [ ] **Step 1: Write the failing test**

```go
func TestOptimizeFTSCoversToolIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opt.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.SetupSchema(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Must not error now that two FTS tables exist.
	if err := db.OptimizeFTS(); err != nil {
		t.Fatalf("optimize: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails or is incomplete**

Run: `go test -run TestOptimizeFTSCoversToolIndex ./internal/storage/ -v`
Expected: PASS for messages_fts but `tool_fts` is not optimized yet — proceed to make the coverage explicit (this test guards against a regression where the second optimize is dropped).

- [ ] **Step 3: Optimize both tables**

In `internal/storage/queries.go`, change `OptimizeFTS`:

```go
func (d *Database) OptimizeFTS() error {
	if _, err := d.db.Exec("INSERT INTO messages_fts(messages_fts, rank) VALUES('optimize', 0)"); err != nil {
		return fmt.Errorf("optimize messages_fts: %w", err)
	}
	if _, err := d.db.Exec("INSERT INTO tool_fts(tool_fts, rank) VALUES('optimize', 0)"); err != nil {
		return fmt.Errorf("optimize tool_fts: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Add `tool_fts` to the `Validate` existence check**

In `Validate` (queries.go ~516-524), after the `messages_fts` existence check, add the same check for `tool_fts`:

```go
	var toolFTSExists int
	err = d.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='tool_fts'
	`).Scan(&toolFTSExists)
	if err != nil || toolFTSExists == 0 {
		return fmt.Errorf("FTS5 virtual table tool_fts does not exist")
	}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -run TestOptimizeFTSCoversToolIndex ./internal/storage/ -v`
Expected: PASS.

- [ ] **Step 6: Full suite + coverage gate**

Run: `just check && just test && just coverage-check`
Expected: all PASS; `internal/storage` ≥85%.

- [ ] **Step 7: Commit**

```bash
git add internal/storage/queries.go internal/storage/unit_test.go
git commit -m "feat(storage): optimize and validate tool_fts alongside messages_fts

Claude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy"
```

---

### Task 5: Document the two-index model and rebuild

**Files:**
- Modify: `CLAUDE.md` (Key Design Decisions + storage description)

**Interfaces:** none (docs only).

- [ ] **Step 1: Update the storage description and Key Design Decisions**

In `CLAUDE.md`, in the `internal/storage/` line of the Module Layout and in **Key Design Decisions**, add an entry:

> - **Split FTS by retrieval semantics**: tool content (`content_type='tool'`) lives in a separate FTS5 index `tool_fts` (tokenizer `trigram`, substring/exact match for paths/commands/errors); text+code live in `messages_fts` (`porter unicode61`). `content_type`-branched triggers route each row. `--content-type tool` queries `tool_fts`; prose queries `messages_fts`; an unfiltered query merges both by per-table min-max-normalized BM25 (cross-index ordering is approximate). Introduced in migration v4. Reasoning/`thinking` indexing is deferred (Slice 2).

- [ ] **Step 2: Verify the docs build / pre-push doc gate is satisfied**

Run: `git diff --stat CLAUDE.md`
Expected: CLAUDE.md shows the new design entry. No package was added/deleted, so the Module/Package Layout gate needs no structural change.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document tool_fts split-index design (migration v4)

Claude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy"
```

---

## Self-Review

**1. Spec coverage (Slice 1 only):**
- Spec "New FTS5 table `tool_fts` trigram" → Task 1. ✓
- Spec "move tool rows out of `messages_fts`" → Task 1 (branched triggers + repopulate). ✓
- Spec "new migration version 4 + full rebuild" → Task 1. ✓
- Spec "query routing: tool → tool_fts; prose → messages_fts" → Task 2. ✓
- Spec "cross-type UNION/merge with normalized rank" (Decision 2) → Task 3. ✓
- Spec "Decision 1 trigram tokenizer" → Task 1 (`tokenize='trigram'`) + Task 2 (trigram sanitizer). ✓
- Spec testing "exact path ranks first", "prose not crowded" → Task 2 + Task 1 leak assertion. ✓
- Spec "rebuild/validate cover both tables" → Task 4. ✓
- Spec "document the index model" → Task 5. ✓
- Spec Slice 2 (reasoning) → intentionally OUT OF SCOPE (gated on thinking-block decision); noted in plan goal and Task 5 doc text.

**2. Placeholder scan:** No TBD/“handle edge cases”/“write tests for the above” — every code step shows the code; every test step shows the test. ✓

**3. Type consistency:** `searchTable(ftsTable, query, opts)`, `sanitizeFTS5QueryTrigram`, `withoutPaging`, `mergeNormalized`, `normalize`, `paginate` are defined in Task 3 and referenced consistently; `applyV4Migration`, `sqlV4ToolFTS`, `sqlV4Triggers`, `sqlV4Repopulate` defined and used in Task 1; `tool_fts`/`tool_vocab` table names consistent across Tasks 1–5. ✓

## Known implementation risks to watch during execution

- **Trigram + external content:** confirm `modernc.org/sqlite` accepts `tokenize='trigram'` at Task 1 Step 5 (red→green). If a token <3 chars is required for a tool query, trigram cannot match it — note in the result, do not silently drop.
- **`'delete-all'` command:** valid only for contentless/external-content FTS5 tables; `messages_fts` is external-content (`content=search_items`), so it is valid. Verify no error at Task 1 Step 5.
- **CLI ordering tests:** Task 3 changes unfiltered-search ordering; existing `cmd/backscroll` tests asserting exact order must be updated to merged semantics (Task 3 Step 6).
