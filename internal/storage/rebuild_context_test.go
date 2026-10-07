package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
