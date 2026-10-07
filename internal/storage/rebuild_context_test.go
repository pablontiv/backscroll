package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/pablontiv/backscroll/internal/projects"
)

func TestRebuildFTSContextPreservesExactRoutingAndPerennialRows(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rebuild.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	messages := []IndexedMessage{
		{Ordinal: 0, UUID: "prose", Role: "user", Text: "proseonlytoken", ContentType: "text", Timestamp: "2026-01-01T00:00:00Z"},
		{Ordinal: 1, UUID: "code", Role: "assistant", Text: "codeonlytoken", ContentType: "code", Timestamp: "2026-01-01T00:00:01Z"},
		{Ordinal: 2, UUID: "reasoning", Role: "assistant", Text: "reasononlytoken", ContentType: "reasoning", Timestamp: "2026-01-01T00:00:02Z"},
		{Ordinal: 3, UUID: "tool", Role: "assistant", Text: "toolonlytoken", ContentType: "tool", Timestamp: "2026-01-01T00:00:03Z"},
	}
	if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "/expired/session.jsonl", Hash: "h", Messages: messages}}); err != nil {
		t.Fatal(err)
	}

	// Put every row in both indexes to prove rebuild repairs routing rather than
	// merely making the expected rows searchable.
	if _, err := db.db.Exec(`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`); err != nil {
		t.Fatalf("contaminate messages index: %v", err)
	}
	if _, err := db.db.Exec(`INSERT INTO tool_fts(tool_fts) VALUES('rebuild')`); err != nil {
		t.Fatalf("contaminate tool index: %v", err)
	}
	if err := db.RebuildFTSContext(context.Background()); err != nil {
		t.Fatalf("RebuildFTSContext: %v", err)
	}

	assertFTSHits := func(table, term string, want int) {
		t.Helper()
		var got int
		query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s MATCH ?`, table, table)
		if err := db.db.QueryRow(query, term).Scan(&got); err != nil {
			t.Fatalf("query %s for %s: %v", table, term, err)
		}
		if got != want {
			t.Fatalf("%s hits for %s = %d, want %d", table, term, got, want)
		}
	}
	for _, term := range []string{"proseonlytoken", "codeonlytoken", "reasononlytoken"} {
		assertFTSHits("messages_fts", term, 1)
		assertFTSHits("tool_fts", term, 0)
	}
	assertFTSHits("tool_fts", "toolonlytoken", 1)
	assertFTSHits("messages_fts", "toolonlytoken", 0)

	var rows int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = '/expired/session.jsonl'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != len(messages) {
		t.Fatalf("perennial rows after rebuild = %d, want %d", rows, len(messages))
	}
}

func TestRebuildStorageContextVariantsRejectPreCanceledContext(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		run  func() error
	}{
		{"RebuildFTSContext", func() error { return db.RebuildFTSContext(ctx) }},
		{"BackfillDerivedContext", func() error { return db.BackfillDerivedContext(ctx, BackfillDerivedOpts{}) }},
		{"ReresolveProjectsContext", func() error {
			_, err := db.ReresolveProjectsContext(ctx, func(string) string { return "project" })
			return err
		}},
		{"ReresolveProjectsWithRegistryContext", func() error {
			_, err := db.ReresolveProjectsWithRegistryContext(ctx, projects.ProjectRegistry{})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestBackfillDerivedContextCancellationPreservesPriorBatchAndRollsBackActiveBatch(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	for i := 0; i < 101; i++ {
		path := fmt.Sprintf("/backfill/%03d.jsonl", i)
		if _, err := db.db.Exec(`
			INSERT INTO search_items
			(source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
			VALUES (?, 'session', 0, 'user', 'no, eso no es un bug', '2026-01-01T00:00:00Z', ?, 'proj', 'text', 0)
		`, path, fmt.Sprintf("backfill-%03d", i)); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	setMaintenanceCancellation(t, cancel)
	if _, err := db.db.Exec(fmt.Sprintf(`
		CREATE TRIGGER cancel_second_backfill_batch AFTER INSERT ON correction_signals
		WHEN NEW.source_path = '/backfill/100.jsonl'
		BEGIN SELECT %s(); END
	`, cancelMaintenanceFunction)); err != nil {
		t.Fatalf("create cancellation trigger: %v", err)
	}
	var progress []int
	err := db.BackfillDerivedContext(ctx, BackfillDerivedOpts{OnProgress: func(processed, _, _, _ int) {
		progress = append(progress, processed)
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BackfillDerivedContext error = %v, want context.Canceled", err)
	}
	if len(progress) != 1 || progress[0] != 100 {
		t.Fatalf("progress = %v, want only committed batch [100]", progress)
	}
	var committed, active int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM correction_signals WHERE source_path < '/backfill/100.jsonl'`).Scan(&committed); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM correction_signals WHERE source_path = '/backfill/100.jsonl'`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if committed != 100 || active != 0 {
		t.Fatalf("signals after cancellation: prior=%d active=%d, want 100 and 0", committed, active)
	}
}

func TestReresolveProjectsContextCancellationRollsBackAllPaths(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	for i, path := range []string{"/a.jsonl", "/b.jsonl", "/c.jsonl"} {
		if _, err := db.db.Exec(`
			INSERT INTO search_items
			(source_path, source, ordinal, role, text, timestamp, uuid, project, content_type)
			VALUES (?, 'session', 0, 'user', 'text', '2026-01-01T00:00:00Z', ?, 'unknown', 'text')
		`, path, fmt.Sprintf("resolve-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	resolved, err := db.ReresolveProjectsContext(ctx, func(path string) string {
		if path == "/b.jsonl" {
			cancel()
		}
		return "resolved"
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if resolved != 0 {
		t.Fatalf("resolved = %d, want 0 after rollback", resolved)
	}
	var unknown int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE project = 'unknown'`).Scan(&unknown); err != nil {
		t.Fatal(err)
	}
	if unknown != 3 {
		t.Fatalf("unknown rows after cancellation = %d, want 3", unknown)
	}
}

type cancelAfterErrChecksContext struct {
	context.Context
	cancelAt int64
	checks   atomic.Int64
}

func (c *cancelAfterErrChecksContext) Done() <-chan struct{} { return nil }

func (c *cancelAfterErrChecksContext) Err() error {
	if c.checks.Add(1) >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

type ftsRow struct {
	rowID int64
	text  string
}

type ftsSnapshot struct {
	messages []ftsRow
	tools    []ftsRow
}

func TestRebuildFTSContextCancellationRestoresBothIndexesExactly(t *testing.T) {
	for _, test := range []struct {
		name     string
		cancelAt int64
	}{
		{name: "after_messages_repopulated", cancelAt: 4},
		{name: "at_commit_after_both_indexes_repopulated", cancelAt: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, cleanup := newTestDB(t)
			defer cleanup()
			seedRoutedFTSRows(t, db)

			// Deliberately index every content row in both indexes. A canceled
			// selective rebuild must restore this exact pre-transaction state.
			if _, err := db.db.Exec(`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(`INSERT INTO tool_fts(tool_fts) VALUES('rebuild')`); err != nil {
				t.Fatal(err)
			}
			before := snapshotFTSRows(t, db, "snapshotall")

			ctx := &cancelAfterErrChecksContext{Context: context.Background(), cancelAt: test.cancelAt}
			err := db.RebuildFTSContext(ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("RebuildFTSContext error = %v, want context.Canceled (checks=%d)", err, ctx.checks.Load())
			}
			after := snapshotFTSRows(t, db, "snapshotall")
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("FTS indexes changed after rollback:\nbefore=%#v\nafter=%#v", before, after)
			}
		})
	}
}

func TestRebuildFTSContextExactContentsRowIDsAndIdempotence(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedRoutedFTSRows(t, db)

	// Start from known cross-contamination so the first call has real repair work.
	if _, err := db.db.Exec(`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`INSERT INTO tool_fts(tool_fts) VALUES('rebuild')`); err != nil {
		t.Fatal(err)
	}
	if err := db.RebuildFTSContext(context.Background()); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	first := snapshotFTSRows(t, db, "snapshotall")
	wantMessages := searchItemRows(t, db, `content_type IN ('text', 'code', 'reasoning')`)
	wantTools := searchItemRows(t, db, `content_type = 'tool'`)
	if !reflect.DeepEqual(first.messages, wantMessages) {
		t.Fatalf("messages_fts rows = %#v, want routed search_items %#v", first.messages, wantMessages)
	}
	if !reflect.DeepEqual(first.tools, wantTools) {
		t.Fatalf("tool_fts rows = %#v, want routed search_items %#v", first.tools, wantTools)
	}
	assertUniqueFTSRowIDs(t, "messages_fts", first.messages)
	assertUniqueFTSRowIDs(t, "tool_fts", first.tools)

	if err := db.RebuildFTSContext(context.Background()); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	second := snapshotFTSRows(t, db, "snapshotall")
	if !reflect.DeepEqual(second, first) {
		t.Fatalf("second rebuild was not exactly idempotent:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func seedRoutedFTSRows(t *testing.T, db *Database) {
	t.Helper()
	messages := []IndexedMessage{
		{Ordinal: 0, UUID: "route-text", Role: "user", Text: "snapshotall proseunique", ContentType: "text", Timestamp: "2026-01-01T00:00:00Z"},
		{Ordinal: 1, UUID: "route-code", Role: "assistant", Text: "snapshotall codeunique", ContentType: "code", Timestamp: "2026-01-01T00:00:01Z"},
		{Ordinal: 2, UUID: "route-reasoning", Role: "assistant", Text: "snapshotall reasonunique", ContentType: "reasoning", Timestamp: "2026-01-01T00:00:02Z"},
		{Ordinal: 3, UUID: "route-tool-a", Role: "assistant", Text: "snapshotall tooluniquealpha", ContentType: "tool", Timestamp: "2026-01-01T00:00:03Z"},
		{Ordinal: 4, UUID: "route-tool-b", Role: "assistant", Text: "snapshotall tooluniquebeta", ContentType: "tool", Timestamp: "2026-01-01T00:00:04Z"},
	}
	if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "/expired/routed.jsonl", Hash: "route-hash", Messages: messages}}); err != nil {
		t.Fatal(err)
	}
}

func snapshotFTSRows(t *testing.T, db *Database, token string) ftsSnapshot {
	t.Helper()
	return ftsSnapshot{
		messages: matchedFTSRows(t, db, "messages_fts", token),
		tools:    matchedFTSRows(t, db, "tool_fts", token),
	}
}

func matchedFTSRows(t *testing.T, db *Database, table, token string) []ftsRow {
	t.Helper()
	query := fmt.Sprintf(`SELECT rowid, text FROM %s WHERE %s MATCH ? ORDER BY rowid, text`, table, table)
	rows, err := db.db.Query(query, token)
	if err != nil {
		t.Fatalf("query %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	var out []ftsRow
	for rows.Next() {
		var row ftsRow
		if err := rows.Scan(&row.rowID, &row.text); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s: %v", table, err)
	}
	return out
}

func searchItemRows(t *testing.T, db *Database, predicate string) []ftsRow {
	t.Helper()
	rows, err := db.db.Query(`SELECT id, text FROM search_items WHERE ` + predicate + ` ORDER BY id, text`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []ftsRow
	for rows.Next() {
		var row ftsRow
		if err := rows.Scan(&row.rowID, &row.text); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertUniqueFTSRowIDs(t *testing.T, table string, rows []ftsRow) {
	t.Helper()
	seen := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if _, duplicate := seen[row.rowID]; duplicate {
			t.Fatalf("%s contains duplicate rowid %d: %#v", table, row.rowID, rows)
		}
		seen[row.rowID] = struct{}{}
	}
}

func TestBackfillDerivedContextCancellationFromFinalProgressCallback(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	const path = "/backfill/final.jsonl"
	if _, err := db.db.Exec(`
		INSERT INTO search_items
		(source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
		VALUES (?, 'session', 0, 'user', 'no, eso no es un bug', '2026-01-01T00:00:00Z', 'final-progress', 'proj', 'text', 0)
	`, path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	progressCalls := 0
	err := db.BackfillDerivedContext(ctx, BackfillDerivedOpts{OnProgress: func(processed, _, _, _ int) {
		progressCalls++
		if processed != 1 {
			t.Errorf("processed = %d, want 1", processed)
		}
		cancel()
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BackfillDerivedContext error = %v, want context.Canceled", err)
	}
	if progressCalls != 1 {
		t.Fatalf("progress calls = %d, want 1", progressCalls)
	}
	var committed int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM correction_signals WHERE source_path = ?`, path).Scan(&committed); err != nil {
		t.Fatal(err)
	}
	if committed == 0 {
		t.Fatal("final batch did not remain committed after progress callback cancellation")
	}
}

func TestReresolveProjectsWithRegistryContextCancellationRollsBack(t *testing.T) {
	for _, test := range []struct {
		name     string
		cancelAt int64
	}{
		{name: "during_scan", cancelAt: 2},
		{name: "during_resolve_after_first_update", cancelAt: 7},
		{name: "at_commit", cancelAt: 9},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, cleanup := newTestDB(t)
			defer cleanup()
			registry := seedRegistryResolutionRows(t, db)
			before := projectRows(t, db)
			ctx := &cancelAfterErrChecksContext{Context: context.Background(), cancelAt: test.cancelAt}
			updated, err := db.ReresolveProjectsWithRegistryContext(ctx, registry)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled (updated=%d checks=%d)", err, updated, ctx.checks.Load())
			}
			if updated != 0 {
				t.Fatalf("updated = %d, want 0 after rollback", updated)
			}
			after := projectRows(t, db)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("projects changed after cancellation: before=%v after=%v", before, after)
			}
		})
	}
}

func TestReresolveLegacyWrappersMatchContextVariants(t *testing.T) {
	t.Run("resolver", func(t *testing.T) {
		contextDB, contextCleanup := newTestDB(t)
		defer contextCleanup()
		legacyDB, legacyCleanup := newTestDB(t)
		defer legacyCleanup()
		for _, db := range []*Database{contextDB, legacyDB} {
			seedUnknownResolutionRows(t, db)
		}
		resolver := func(path string) string { return "resolved-" + filepath.Base(path) }
		got, err := contextDB.ReresolveProjectsContext(context.Background(), resolver)
		if err != nil {
			t.Fatal(err)
		}
		want, err := legacyDB.ReresolveProjects(context.Background(), resolver)
		if err != nil {
			t.Fatal(err)
		}
		if got != want || !reflect.DeepEqual(projectRows(t, contextDB), projectRows(t, legacyDB)) {
			t.Fatalf("context count/rows = %d/%v, legacy = %d/%v", got, projectRows(t, contextDB), want, projectRows(t, legacyDB))
		}
	})

	t.Run("registry", func(t *testing.T) {
		contextDB, contextCleanup := newTestDB(t)
		defer contextCleanup()
		legacyDB, legacyCleanup := newTestDB(t)
		defer legacyCleanup()
		contextRegistry := seedRegistryResolutionRows(t, contextDB)
		legacyRegistry := seedRegistryResolutionRows(t, legacyDB)
		got, err := contextDB.ReresolveProjectsWithRegistryContext(context.Background(), contextRegistry)
		if err != nil {
			t.Fatal(err)
		}
		want, err := legacyDB.ReresolveProjectsWithRegistry(context.Background(), legacyRegistry)
		if err != nil {
			t.Fatal(err)
		}
		if got != want || !reflect.DeepEqual(projectRows(t, contextDB), projectRows(t, legacyDB)) {
			t.Fatalf("context count/rows = %d/%v, legacy = %d/%v", got, projectRows(t, contextDB), want, projectRows(t, legacyDB))
		}
	})
}

func seedUnknownResolutionRows(t *testing.T, db *Database) {
	t.Helper()
	for i, path := range []string{"/wrapper/a.jsonl", "/wrapper/b.jsonl"} {
		if _, err := db.db.Exec(`
			INSERT INTO search_items
			(source_path, source, ordinal, role, text, timestamp, uuid, project, content_type)
			VALUES (?, 'session', 0, 'user', 'text', '2026-01-01T00:00:00Z', ?, 'unknown', 'text')
		`, path, fmt.Sprintf("wrapper-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
}

func seedRegistryResolutionRows(t *testing.T, db *Database) projects.ProjectRegistry {
	t.Helper()
	for i, path := range []string{
		"/home/test/.claude/projects/-registryalpha/a.jsonl",
		"/home/test/.claude/projects/-registryalpha/b.jsonl",
	} {
		if _, err := db.db.Exec(`
			INSERT INTO search_items
			(source_path, source, ordinal, role, text, timestamp, uuid, project, content_type)
			VALUES (?, 'session', 0, 'user', 'text', '2026-01-01T00:00:00Z', ?, 'fallback', 'text')
		`, path, fmt.Sprintf("registry-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	return projects.ProjectRegistry{Projects: []projects.ProjectConfig{{
		ID: "canonical-registry", Roots: []string{"registryalpha"},
	}}}
}

func projectRows(t *testing.T, db *Database) []string {
	t.Helper()
	rows, err := db.db.Query(`SELECT source_path || '=' || COALESCE(project, '<null>') FROM search_items ORDER BY source_path, ordinal`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
