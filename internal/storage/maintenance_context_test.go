package storage

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/pablontiv/backscroll/internal/templates"
	"modernc.org/sqlite"
)

const cancelMaintenanceFunction = "backscroll_test_cancel_maintenance_context"

var (
	cancelMaintenanceMu sync.Mutex
	cancelMaintenance   context.CancelFunc
)

func init() {
	sqlite.MustRegisterScalarFunction(cancelMaintenanceFunction, 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
		cancelMaintenanceMu.Lock()
		cancel := cancelMaintenance
		cancelMaintenanceMu.Unlock()
		if cancel != nil {
			cancel()
		}
		return int64(0), nil
	})
}

func setMaintenanceCancellation(t *testing.T, cancel context.CancelFunc) {
	t.Helper()
	cancelMaintenanceMu.Lock()
	cancelMaintenance = cancel
	cancelMaintenanceMu.Unlock()
	t.Cleanup(func() {
		cancelMaintenanceMu.Lock()
		cancelMaintenance = nil
		cancelMaintenanceMu.Unlock()
	})
}

func TestMaintenanceContextPreCanceled(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		run  func() error
	}{
		{"GetFileMetadata", func() error { _, err := db.GetFileMetadataContext(ctx); return err }},
		{"StalePaths", func() error { _, err := db.StalePathsContext(ctx, CurrentExtractionVersion); return err }},
		{"EmptyIndexedPaths", func() error { _, err := db.EmptyIndexedPathsContext(ctx); return err }},
		{"PendingSearchEchoPaths", func() error { _, err := db.PendingSearchEchoPathsContext(ctx); return err }},
		{"PendingOriginPaths", func() error { _, err := db.PendingOriginPathsContext(ctx, CurrentOriginVersion, 10); return err }},
		{"StaleTemplatePaths", func() error { _, err := db.StaleTemplatePathsContext(ctx, CurrentNormalizationVersion); return err }},
		{"LoadMessagesForPath", func() error { _, err := db.LoadMessagesForPathContext(ctx, "/none"); return err }},
		{"BackfillTemplatesForFile", func() error {
			_, err := db.BackfillTemplatesForFileContext(ctx, templates.NewMiner(), "/none", nil)
			return err
		}},
		{"RederiveSupersededCorrections", func() error {
			_, err := db.RederiveSupersededCorrectionsContext(ctx, 10)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestQueryPathsContextCancellationDuringIterationReturnsNoPartialResults(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	setMaintenanceCancellation(t, cancel)
	paths, err := db.queryPathsContext(ctx, fmt.Sprintf(`
		WITH RECURSIVE seq(x) AS (VALUES(1) UNION ALL SELECT x + 1 FROM seq WHERE x < 3)
		SELECT CASE WHEN x = 2 THEN CAST(%s() AS TEXT) ELSE CAST(x AS TEXT) END FROM seq
	`, cancelMaintenanceFunction))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("queryPathsContext error = %v, want context.Canceled", err)
	}
	if paths != nil {
		t.Fatalf("queryPathsContext returned partial paths: %v", paths)
	}
}

func TestBackfillTemplatesForFileContextCancellationRollsBackActivePath(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	const path = "/cancel/template.jsonl"
	if _, err := db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count)
		VALUES ('old-template', 1, 'old template', 1)
	`); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO template_matches (template_id, item_uuid, source_path, ordinal)
		VALUES ((SELECT id FROM message_templates WHERE signature = 'old-template'), 'old#0', ?, 0)
	`, path); err != nil {
		t.Fatalf("seed match: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	setMaintenanceCancellation(t, cancel)
	if _, err := db.db.Exec(fmt.Sprintf(`
		CREATE TRIGGER cancel_template_backfill AFTER DELETE ON template_matches
		WHEN OLD.source_path = '%s'
		BEGIN SELECT %s(); END
	`, path, cancelMaintenanceFunction)); err != nil {
		t.Fatalf("create cancellation trigger: %v", err)
	}

	_, err := db.BackfillTemplatesForFileContext(ctx, templates.NewMiner(), path, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("BackfillTemplatesForFileContext error = %v, want context.Canceled", err)
	}
	var matches, staleTemplates int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches WHERE source_path = ?`, path).Scan(&matches); err != nil {
		t.Fatalf("count rolled-back matches: %v", err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates WHERE signature = 'old-template' AND normalization_version = 1`).Scan(&staleTemplates); err != nil {
		t.Fatalf("count rolled-back template: %v", err)
	}
	if matches != 1 || staleTemplates != 1 {
		t.Fatalf("active path was not atomic: matches=%d stale_templates=%d, want 1 and 1", matches, staleTemplates)
	}
}

func TestRederiveSupersededCorrectionsContextCancellationPreservesPriorPathAndRollsBackActivePath(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	for i, path := range []string{"/a/committed.jsonl", "/b/canceled.jsonl", "/c/not-started.jsonl"} {
		uuid := fmt.Sprintf("maintenance-%d", i)
		if _, err := db.db.Exec(`
			INSERT INTO search_items (source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
			VALUES (?, 'session', 0, 'user', 'no, eso no es un bug', '2026-01-01T00:00:00Z', ?, 'proj', 'text', ?)
		`, path, uuid, CurrentExtractionVersion); err != nil {
			t.Fatalf("seed search item %s: %v", path, err)
		}
		if _, err := db.db.Exec(`
			INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
			VALUES (?, ?, 0, 'legacy', 0.8, 0)
		`, uuid, path); err != nil {
			t.Fatalf("seed stale signal %s: %v", path, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	setMaintenanceCancellation(t, cancel)
	if _, err := db.db.Exec(fmt.Sprintf(`
		CREATE TRIGGER cancel_active_rederivation AFTER DELETE ON correction_signals
		WHEN OLD.source_path = '/b/canceled.jsonl'
		BEGIN SELECT %s(); END
	`, cancelMaintenanceFunction)); err != nil {
		t.Fatalf("create cancellation trigger: %v", err)
	}

	processed, err := db.RederiveSupersededCorrectionsContext(ctx, 10)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RederiveSupersededCorrectionsContext error = %v, want context.Canceled", err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1 prior committed path", processed)
	}

	assertStale := func(path string, want int) {
		t.Helper()
		var got int
		if err := db.db.QueryRow(`
			SELECT COUNT(*) FROM correction_signals
			WHERE source_path = ? AND (extraction_version IS NULL OR extraction_version < ?)
		`, path, CurrentExtractionVersion).Scan(&got); err != nil {
			t.Fatalf("count stale signals for %s: %v", path, err)
		}
		if got != want {
			t.Fatalf("stale signals for %s = %d, want %d", path, got, want)
		}
	}
	assertStale("/a/committed.jsonl", 0)   // prior path remains committed
	assertStale("/b/canceled.jsonl", 1)    // active path DELETE was rolled back
	assertStale("/c/not-started.jsonl", 1) // cancellation stopped iteration immediately
}

func TestMaintenanceContextLegacyWrappersEquivalent(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedMaintenanceContextQueries(t, db)

	type call func() (any, error)
	checks := []struct {
		name    string
		context call
		legacy  call
	}{
		{"GetFileMetadata", func() (any, error) { return db.GetFileMetadataContext(context.Background()) }, func() (any, error) { return db.GetFileMetadata() }},
		{"StalePaths", func() (any, error) { return db.StalePathsContext(context.Background(), CurrentExtractionVersion) }, func() (any, error) { return db.StalePaths(CurrentExtractionVersion) }},
		{"EmptyIndexedPaths", func() (any, error) { return db.EmptyIndexedPathsContext(context.Background()) }, func() (any, error) { return db.EmptyIndexedPaths() }},
		{"PendingSearchEchoPaths", func() (any, error) { return db.PendingSearchEchoPathsContext(context.Background()) }, func() (any, error) { return db.PendingSearchEchoPaths() }},
		{"PendingOriginPaths", func() (any, error) {
			return db.PendingOriginPathsContext(context.Background(), CurrentOriginVersion, 10)
		}, func() (any, error) { return db.PendingOriginPaths(CurrentOriginVersion, 10) }},
		{"StaleTemplatePaths", func() (any, error) {
			return db.StaleTemplatePathsContext(context.Background(), CurrentNormalizationVersion)
		}, func() (any, error) { return db.StaleTemplatePaths(CurrentNormalizationVersion) }},
		{"LoadMessagesForPath", func() (any, error) {
			return db.LoadMessagesForPathContext(context.Background(), "/maintenance/stale.jsonl")
		}, func() (any, error) { return db.LoadMessagesForPath("/maintenance/stale.jsonl") }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			got, err := check.context()
			if err != nil {
				t.Fatalf("context variant: %v", err)
			}
			want, err := check.legacy()
			if err != nil {
				t.Fatalf("legacy wrapper: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("context = %#v, legacy = %#v", got, want)
			}
		})
	}
}

func TestMaintenanceMutationContextLegacyWrappersEquivalent(t *testing.T) {
	t.Run("BackfillTemplatesForFile", func(t *testing.T) {
		contextDB, contextCleanup := newTestDB(t)
		defer contextCleanup()
		legacyDB, legacyCleanup := newTestDB(t)
		defer legacyCleanup()

		msgs := []IndexedMessage{{
			Ordinal: 0, UUID: "equivalent-template", Role: "assistant",
			Text: "error: cannot open /tmp/equivalent.go", ContentType: "tool",
		}}
		got, err := contextDB.BackfillTemplatesForFileContext(context.Background(), templates.NewMiner(), "/equivalent/template.jsonl", msgs)
		if err != nil {
			t.Fatalf("context variant: %v", err)
		}
		want, err := legacyDB.BackfillTemplatesForFile(templates.NewMiner(), "/equivalent/template.jsonl", msgs)
		if err != nil {
			t.Fatalf("legacy wrapper: %v", err)
		}
		if got != want {
			t.Fatalf("deleted count: context=%d legacy=%d", got, want)
		}
		if contextRows, legacyRows := maintenanceDerivedRows(t, contextDB), maintenanceDerivedRows(t, legacyDB); !reflect.DeepEqual(contextRows, legacyRows) {
			t.Fatalf("context rows = %v, legacy rows = %v", contextRows, legacyRows)
		}
	})

	t.Run("RederiveSupersededCorrections", func(t *testing.T) {
		contextDB, contextCleanup := newTestDB(t)
		defer contextCleanup()
		legacyDB, legacyCleanup := newTestDB(t)
		defer legacyCleanup()
		for _, db := range []*Database{contextDB, legacyDB} {
			if _, err := db.db.Exec(`
				INSERT INTO search_items (source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
				VALUES ('/equivalent/correction.jsonl', 'session', 0, 'user', 'no, eso no es un bug',
				'2026-01-01T00:00:00Z', 'equivalent-correction', 'proj', 'text', ?)
			`, CurrentExtractionVersion); err != nil {
				t.Fatalf("seed search item: %v", err)
			}
			if _, err := db.db.Exec(`
				INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
				VALUES ('equivalent-correction', '/equivalent/correction.jsonl', 0, 'legacy', 0.8, 0)
			`); err != nil {
				t.Fatalf("seed correction: %v", err)
			}
		}
		got, err := contextDB.RederiveSupersededCorrectionsContext(context.Background(), 10)
		if err != nil {
			t.Fatalf("context variant: %v", err)
		}
		want, err := legacyDB.RederiveSupersededCorrections(10)
		if err != nil {
			t.Fatalf("legacy wrapper: %v", err)
		}
		if got != want {
			t.Fatalf("processed count: context=%d legacy=%d", got, want)
		}
		if contextRows, legacyRows := maintenanceDerivedRows(t, contextDB), maintenanceDerivedRows(t, legacyDB); !reflect.DeepEqual(contextRows, legacyRows) {
			t.Fatalf("context rows = %v, legacy rows = %v", contextRows, legacyRows)
		}
	})
}

func maintenanceDerivedRows(t *testing.T, db *Database) []string {
	t.Helper()
	var out []string
	rows, err := db.db.Query(`
		SELECT 'template', signature, normalization_version FROM message_templates
		UNION ALL
		SELECT 'correction', source_path || ':' || detector, COALESCE(extraction_version, -1) FROM correction_signals
		ORDER BY 1, 2, 3
	`)
	if err != nil {
		t.Fatalf("query derived rows: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind, identity string
		var version int
		if err := rows.Scan(&kind, &identity, &version); err != nil {
			t.Fatalf("scan derived row: %v", err)
		}
		out = append(out, fmt.Sprintf("%s|%s|%d", kind, identity, version))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate derived rows: %v", err)
	}
	return out
}

func seedMaintenanceContextQueries(t *testing.T, db *Database) {
	t.Helper()
	if _, err := db.db.Exec(`
		INSERT INTO indexed_files (path, hash, last_indexed) VALUES
		('/maintenance/stale.jsonl', 'stale-hash', '2026-01-01T00:00:00Z'),
		('/maintenance/empty.jsonl', 'empty-hash', '2026-01-02T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed indexed files: %v", err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO search_items
		(source_path, source, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, search_echo, origin_version)
		VALUES ('/maintenance/stale.jsonl', 'session', 0, 'user', 'no, eso no', '2026-01-01T00:00:00Z',
		'maintenance-stale', 'proj', 'text', 0, NULL, NULL)
	`); err != nil {
		t.Fatalf("seed search item: %v", err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count)
		VALUES ('maintenance-template', 1, 'maintenance template', 1)
	`); err != nil {
		t.Fatalf("seed message template: %v", err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO template_matches (template_id, item_uuid, source_path, ordinal)
		VALUES ((SELECT id FROM message_templates WHERE signature = 'maintenance-template'),
		'maintenance-stale', '/maintenance/stale.jsonl', 0)
	`); err != nil {
		t.Fatalf("seed template match: %v", err)
	}
}
