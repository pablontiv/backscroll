package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/templates"
)

// A disposed database must never look like an empty successful corpus. Each
// public consumer returns no publishable payload and an actionable error.
func TestDisposedIndexConsumersReportFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BACKSCROLL_CONFIG_DIR", t.TempDir())
	db, err := Open(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	uuid := "omp-anchor"
	checks := map[string]func() error{
		"stats": func() error {
			r, e := db.GetStats()
			if r != (Stats{}) {
				t.Errorf("partial stats: %+v", r)
			}
			return e
		},
		"topics": func() error {
			r, e := db.GetTopics("omp", 10)
			if r != nil {
				t.Errorf("partial topics: %+v", r)
			}
			return e
		},
		"list": func() error {
			r, e := db.ListItemsV2(ListOptions{})
			if r != nil {
				t.Errorf("partial list: %+v", r)
			}
			return e
		},
		"sessions": func() error {
			r, e := db.ListSessions("omp", 1)
			if r != nil {
				t.Errorf("partial sessions: %+v", r)
			}
			return e
		},
		"commands": func() error {
			r, e := db.AggregateCommands(AggregateOptions{})
			if r != nil {
				t.Errorf("partial commands: %+v", r)
			}
			return e
		},
		"command trend": func() error {
			r, e := db.AggregateCommandsTrend(AggregateOptions{}, nil)
			if r != nil {
				t.Errorf("partial trends: %+v", r)
			}
			return e
		},
		"failures": func() error {
			r, e := db.AggregateFailures(AggregateOptions{})
			if r != nil {
				t.Errorf("partial failures: %+v", r)
			}
			return e
		},
		"failure trend": func() error {
			r, e := db.AggregateFailuresTrend(AggregateOptions{}, nil)
			if r != nil {
				t.Errorf("partial trends: %+v", r)
			}
			return e
		},
		"templates": func() error {
			r, e := db.AggregateTemplates(TemplateQueryOpts{})
			if r != nil {
				t.Errorf("partial templates: %+v", r)
			}
			return e
		},
		"corrections": func() error {
			r, e := db.AggregateCorrections(CorrectionAggOpts{})
			if r != nil {
				t.Errorf("partial corrections: %+v", r)
			}
			return e
		},
		"sequences": func() error {
			r, e := db.LoadToolSequences(LoadSequencesOpts{})
			if r != nil {
				t.Errorf("partial sequences: %+v", r)
			}
			return e
		},
		"context": func() error {
			r, e := db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: &uuid})
			if r != nil {
				t.Errorf("partial context: %+v", r)
			}
			return e
		},
		"records": func() error {
			r, e := db.QueryIndexedRecords(IndexedRecordQuery{})
			if r != nil {
				t.Errorf("partial records: %+v", r)
			}
			return e
		},
		"echo queue": func() error {
			r, e := db.PendingSearchEchoPaths()
			if r != nil {
				t.Errorf("partial queue: %+v", r)
			}
			return e
		},
		"origin queue": func() error {
			r, e := db.PendingOriginPaths(CurrentOriginVersion, 200)
			if r != nil {
				t.Errorf("partial queue: %+v", r)
			}
			return e
		},
		"stale queue": func() error {
			r, e := db.StalePaths(CurrentExtractionVersion)
			if r != nil {
				t.Errorf("partial queue: %+v", r)
			}
			return e
		},
		"message reload": func() error {
			r, e := db.LoadMessagesForPath("/omp")
			if r != nil {
				t.Errorf("partial messages: %+v", r)
			}
			return e
		},
		"annotate uuid":       func() error { return db.UpsertAnnotation(uuid, "", -1, "correction", "confirmed") },
		"annotate coordinate": func() error { return db.UpsertAnnotation("", "/omp", 0, "correction", "confirmed") },
		"sync": func() error {
			return db.SyncFiles([]IndexedFile{{SourcePath: "/omp", Source: "session", Hash: "hash", Messages: []IndexedMessage{{UUID: uuid, Text: "retained", ContentType: "text"}}}})
		},
		"rebuild":  db.RebuildFTS,
		"optimize": db.OptimizeFTS,
		"backfill": func() error {
			return db.BackfillDerived(BackfillDerivedOpts{OnProgress: func(int, int, int, int) { t.Error("published progress after close") }})
		},
		"template re-mine": func() error {
			n, e := db.BackfillTemplatesForFile(templates.NewMiner(), "/omp", nil)
			if n != 0 {
				t.Errorf("deleted templates: %d", n)
			}
			return e
		},
		"correction re-derive": func() error {
			n, e := db.RederiveSupersededCorrections(200)
			if n != 0 {
				t.Errorf("processed: %d", n)
			}
			return e
		},
		"search": func() error {
			r, e := db.Search("retained", models.SearchOptions{Limit: 10})
			if r != nil {
				t.Errorf("partial search: %+v", r)
			}
			return e
		},
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			err := check()
			if err == nil || !strings.Contains(err.Error(), "database is closed") {
				t.Fatalf("lost database failure: %v", err)
			}
		})
	}
}

func TestExactContextMissingAndAmbiguousDiagnostics(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "context.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	uuid := "missing-omp"
	_, err = db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: &uuid})
	if !errors.Is(err, ErrContextRecordNotFound) || !strings.Contains(err.Error(), `uuid "missing-omp"`) {
		t.Fatalf("missing uuid: %v", err)
	}
	path := "/omp"
	ordinal := int64(4)
	_, err = db.QueryContextRecords(context.Background(), ContextRecordQuery{SourcePath: &path, Ordinal: &ordinal})
	if !errors.Is(err, ErrContextRecordNotFound) || !strings.Contains(err.Error(), `source_path "/omp" ordinal 4`) {
		t.Fatalf("missing coordinate: %v", err)
	}
	for _, id := range []string{"a", "b"} {
		if _, err := db.db.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,content_type) VALUES('session','/omp',4,'user','retained',?,'text')`, id); err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.QueryContextRecords(context.Background(), ContextRecordQuery{SourcePath: &path, Ordinal: &ordinal})
	if !errors.Is(err, ErrContextRecordAmbiguous) || !strings.Contains(err.Error(), `source_path "/omp" ordinal 4`) {
		t.Fatalf("ambiguous coordinate: %v", err)
	}
}

// SQLite's non-STRICT integer columns can contain malformed recovered data.
// Consumers must reject it rather than publish a partial page or move anchors.
func TestMalformedPerennialOrdinalRejectsConsumersAndSync(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "malformed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.db.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,project,content_type) VALUES('session','/omp','broken','user','retained','bad','omp','text')`); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryIndexedRecords(IndexedRecordQuery{})
	if err == nil || rows != nil || !strings.Contains(err.Error(), "ordinal") {
		t.Fatalf("records: %+v %v", rows, err)
	}
	messages, err := db.LoadMessagesForPath("/omp")
	if err == nil || messages != nil {
		t.Fatalf("reload: %+v %v", messages, err)
	}
	id := "bad"
	contextRows, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: &id})
	if err == nil || contextRows != nil {
		t.Fatalf("context: %+v %v", contextRows, err)
	}
	err = db.BackfillDerived(BackfillDerivedOpts{})
	if err == nil || !strings.Contains(err.Error(), "scan message") {
		t.Fatalf("backfill: %v", err)
	}
	err = db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "/omp", Hash: "new", Messages: []IndexedMessage{{UUID: "new", Text: "new text", ContentType: "text"}}}})
	if err == nil || !strings.Contains(err.Error(), "reconcile ordinals") {
		t.Fatalf("append: %v", err)
	}
	var n int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE uuid='new'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("malformed anchor allowed partial append")
	}
}

func TestMalformedToolSignalsRejectFailurePages(t *testing.T) {
	for _, trend := range []bool{false, true} {
		t.Run(fmtBool(trend), func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "signals.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			bad := true
			file := IndexedFile{Source: "session", SourcePath: "/omp", Hash: "hash", Project: "omp", Messages: []IndexedMessage{{UUID: "call", Role: "assistant", Text: "bash command=go test", ContentType: "tool", ToolName: "bash", CommandHead: "go", IsError: &bad, Timestamp: "2026-01-01T00:00:00Z"}}}
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(`UPDATE tool_events SET exit_code='broken'`); err != nil {
				t.Fatal(err)
			}
			if trend {
				rows, err := db.AggregateFailuresTrend(AggregateOptions{}, nil)
				if err == nil || rows != nil || !strings.Contains(err.Error(), "scan") {
					t.Fatalf("trend: %+v %v", rows, err)
				}
			} else {
				rows, err := db.AggregateFailures(AggregateOptions{})
				if err == nil || rows != nil || !strings.Contains(err.Error(), "scan") {
					t.Fatalf("page: %+v %v", rows, err)
				}
			}
		})
	}
}

func fmtBool(b bool) string {
	if b {
		return "trend"
	}
	return "page"
}

func TestStatsOptionalTablesDoNotHideMandatoryIndexFailure(t *testing.T) {
	t.Parallel()
	for _, missing := range []string{"chunks", "embedding_metadata", "search_items"} {
		t.Run(missing, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "stats.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "/omp", Hash: "hash", Messages: []IndexedMessage{{UUID: "a", Text: "retained", ContentType: "text"}}}}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec("DROP TABLE " + missing); err != nil {
				t.Fatal(err)
			}
			stats, err := db.GetStats()
			if missing == "search_items" {
				if err == nil || stats != (Stats{}) || !strings.Contains(err.Error(), "count messages") {
					t.Fatalf("mandatory data failure: %+v %v", stats, err)
				}
			} else {
				if err != nil || stats.TotalFiles != 1 || stats.TotalMessages != 1 || stats.TotalChunks != 0 || stats.TotalEmbeddings != 0 || stats.TotalVectors != 0 {
					t.Fatalf("optional data failure: %+v %v", stats, err)
				}
			}
		})
	}
}

// NULL project and command metadata are possible in retained legacy rows.
// Required project fields still reject partial pages; optional command heads
// normalize to complete empty-string identities without dropping retained rows.
func TestNullLegacyMetadataIsNotPublishedAsPartialPage(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "null.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.db.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,content_type,timestamp) VALUES('session','/omp',0,'assistant','retained','call','tool','2026-09-01T00:00:00Z'); INSERT INTO tool_events(source_path,ordinal,tool_name,extraction_version) VALUES('/omp',0,'bash',0)`); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListItemsV2(ListOptions{})
	if err == nil || list != nil || !strings.Contains(err.Error(), "scan item") {
		t.Fatalf("list: %+v %v", list, err)
	}
	sessions, err := db.ListSessions("", 0)
	if err == nil || sessions != nil || !strings.Contains(err.Error(), "scan session") {
		t.Fatalf("sessions: %+v %v", sessions, err)
	}

	// Mix native NULL, empty, and named heads with tied counts. The input order
	// deliberately differs from the final logical order.
	for i, head := range []any{"go", "", "git", "go", "git"} {
		ordinal := i + 1
		if _, err := db.db.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,content_type,timestamp) VALUES('session','/omp',?,'assistant','retained',?,'tool','2026-09-01T00:00:00Z');`, ordinal, fmt.Sprintf("call-%d", ordinal)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.db.Exec(`INSERT INTO tool_events(source_path,ordinal,tool_name,command_head,extraction_version) VALUES('/omp',?,'bash',?,0)`, ordinal, head); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.db.Exec("UPDATE tool_events SET is_error=1,exit_code=1"); err != nil {
		t.Fatal(err)
	}
	want := []CommandPattern{{"bash", "", 2}, {"bash", "git", 2}, {"bash", "go", 2}}
	commands, err := db.AggregateCommands(AggregateOptions{})
	if err != nil || !reflect.DeepEqual(commands, want) {
		t.Fatalf("complete commands: %+v %v", commands, err)
	}
	trends, err := db.AggregateCommandsTrend(AggregateOptions{}, nil)
	if err != nil || len(trends) != len(want) {
		t.Fatalf("complete trends: %+v %v", trends, err)
	}
	for i, expected := range want {
		if trends[i].Week != "2026-W35" || trends[i].ToolName != expected.ToolName || trends[i].CommandHead != expected.CommandHead || trends[i].Count != expected.Count {
			t.Fatalf("trend %d: %+v", i, trends[i])
		}
		page, err := db.AggregateCommands(AggregateOptions{Limit: 1, Offset: i})
		if err != nil || !reflect.DeepEqual(page, want[i:i+1]) {
			t.Fatalf("command page %d: %+v %v", i, page, err)
		}
		weekly, err := db.AggregateCommandsTrend(AggregateOptions{Limit: 1, Offset: i}, nil)
		if err != nil || !reflect.DeepEqual(weekly, trends[i:i+1]) {
			t.Fatalf("weekly page %d: %+v %v", i, weekly, err)
		}
	}
	tail, err := db.AggregateCommands(AggregateOptions{Offset: 1})
	if err != nil || !reflect.DeepEqual(tail, want[1:]) {
		t.Fatalf("unlimited tail: %+v %v", tail, err)
	}
	failures, err := db.AggregateFailures(AggregateOptions{})
	if err != nil || len(failures) != 1 || failures[0].ToolName != "bash" || !failures[0].IsError || failures[0].ExitCode == nil || *failures[0].ExitCode != 1 || failures[0].Count != 6 || failures[0].SignalledEvents != 6 {
		t.Fatalf("complete failures: %+v %v", failures, err)
	}
	weeklyFailures, err := db.AggregateFailuresTrend(AggregateOptions{}, nil)
	if err != nil || len(weeklyFailures) != 1 || weeklyFailures[0].Count != 6 || weeklyFailures[0].SignalledEvents != 6 {
		t.Fatalf("complete failure trends: %+v %v", weeklyFailures, err)
	}
	var nulls int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM tool_events WHERE command_head IS NULL").Scan(&nulls); err != nil || nulls != 1 {
		t.Fatalf("persisted NULL proof changed: %d %v", nulls, err)
	}
	// tool_name remains required by the persisted contract: reject malformed
	// proof at the write boundary instead of weakening aggregate consumers.
	if _, err := db.db.Exec("UPDATE tool_events SET tool_name=NULL WHERE ordinal=5"); err == nil {
		t.Fatal("required tool_name accepted NULL")
	}
	commands, err = db.AggregateCommands(AggregateOptions{})
	if err != nil || !reflect.DeepEqual(commands, want) {
		t.Fatalf("rejected malformed write changed complete records: %+v %v", commands, err)
	}

}

// Optional embedding counts may tolerate an absent table, but must never
// swallow cancellation as if the caller received a complete status snapshot.
func TestStatsCancellationDuringOptionalCountsReturnsNoSnapshot(t *testing.T) {
	for _, table := range []string{"chunks", "embedding_metadata"} {
		t.Run(table, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "cancel-stats.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "/omp", Hash: "hash", Messages: []IndexedMessage{{UUID: "a", Text: "retained", ContentType: "text"}}}}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			setSyncCancellation(t, cancel)
			if _, err := db.db.Exec("DROP TABLE " + table); err != nil {
				t.Fatal(err)
			}
			// Evaluate the cancellation function from a real SQLite read, after the
			// mandatory corpus counts have succeeded. No wall-clock race is involved.
			if _, err := db.db.Exec(fmt.Sprintf(`CREATE VIEW %s AS SELECT id,NULL AS embedding FROM search_items WHERE %s()=0`, table, cancelSyncFunction)); err != nil {
				t.Fatal(err)
			}
			stats, err := db.GetStatsContext(ctx)
			if !errors.Is(err, context.Canceled) || stats != (Stats{}) {
				t.Fatalf("partial status on cancellation: %+v %v", stats, err)
			}
		})
	}
}

func TestMetadataAndAnnotationRejectMalformedStoredCoordinates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "malformed-metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.db.Exec(`INSERT INTO indexed_files(path,hash,file_size) VALUES('/omp','hash','broken'); INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,content_type) VALUES('session','/omp','broken','user','retained','bad','text')`); err != nil {
		t.Fatal(err)
	}
	metadata, err := db.GetFileMetadata()
	if err == nil || metadata != nil || !strings.Contains(err.Error(), "scan file metadata") {
		t.Fatalf("metadata: %+v %v", metadata, err)
	}
	err = db.UpsertAnnotation("bad", "", -1, "correction", "confirmed")
	if err == nil || !strings.Contains(err.Error(), "resolve uuid") {
		t.Fatalf("annotation: %v", err)
	}
	var n int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM annotations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("malformed annotation persisted")
	}
}

func TestMissingTagSatelliteDoesNotPublishUnfilteredSessionPage(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "tags.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "/omp", Hash: "hash", Project: "omp", Tags: []string{"testing"}, Messages: []IndexedMessage{{UUID: "a", Text: "retained", ContentType: "text"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("DROP TABLE session_tags"); err != nil {
		t.Fatal(err)
	}
	items, err := db.ListItemsV2(ListOptions{})
	if err == nil || items != nil || !strings.Contains(err.Error(), "query tags") {
		t.Fatalf("items: %+v %v", items, err)
	}
	sessions, err := db.ListSessions("", 0)
	if err == nil || sessions != nil || !strings.Contains(err.Error(), "query tags") {
		t.Fatalf("sessions: %+v %v", sessions, err)
	}
}

func TestMissingFTSIndexesAreActionableMaintenanceFailures(t *testing.T) {
	for _, table := range []string{"messages_fts", "tool_fts"} {
		t.Run(table, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "fts.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.db.Exec("DROP TABLE " + table); err != nil {
				t.Fatal(err)
			}
			if err := db.Validate(); err == nil || !strings.Contains(err.Error(), table) {
				t.Fatalf("validation hid missing index: %v", err)
			}
			if err := db.OptimizeFTS(); err == nil || !strings.Contains(err.Error(), table) {
				t.Fatalf("optimization hid missing index: %v", err)
			}
			if err := db.RebuildFTS(); err == nil || !strings.Contains(err.Error(), table) {
				t.Fatalf("rebuild hid missing index: %v", err)
			}
		})
	}
}
