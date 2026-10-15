package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Expired input files cannot repair a partial derived-data write. Abort at
// each persistent stage and verify retry converges without publishing progress.
func TestExpiredSessionBackfillRejectionIsAtomic(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ table, operation string }{
		{"message_templates", "INSERT"}, {"message_templates", "UPDATE"}, {"template_matches", "INSERT"}, {"template_matches", "DELETE"}, {"message_templates", "DELETE"}, {"correction_signals", "DELETE"}, {"correction_signals", "INSERT"}, {"tool_events", "INSERT"},
	} {
		t.Run(tc.table+tc.operation, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "backfill.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for i, msg := range []struct{ role, text, kind string }{{"tool", "error: connection refused 123", "tool"}, {"user", "No, that's wrong. Use the other file instead.", "text"}, {"assistant", "bash command=go test", "tool"}} {
				if _, err := db.db.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,content_type) VALUES('session','/expired',?,?,?,?,?)`, i, msg.role, msg.text, fmt.Sprintf("u%d", i), msg.kind); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.db.Exec(`INSERT INTO message_templates(id,signature,template_text,normalization_version,occurrence_count) VALUES(99,'old','obsolete template',1,1); INSERT INTO template_matches(template_id,source_path,ordinal,item_uuid) VALUES(99,'/expired',0,'u0'); INSERT INTO correction_signals(source_path,ordinal,item_uuid,detector,confidence,extraction_version) VALUES('/expired',0,'u0','lexicon',1,0)`); err != nil {
				t.Fatal(err)
			}
			// UPDATE is conditional; retain a current signature at an old epoch so
			// normalization enrichment must execute against an existing row.
			if tc.operation == "UPDATE" {
				if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
					t.Fatal(err)
				}
				if _, err := db.db.Exec(`UPDATE message_templates SET normalization_version=1; DELETE FROM tool_events; UPDATE correction_signals SET extraction_version=0`); err != nil {
					t.Fatal(err)
				}
			}
			counts := map[string]int{}
			for _, table := range []string{"message_templates", "template_matches", "correction_signals", "tool_events"} {
				var n int
				if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
					t.Fatal(err)
				}
				counts[table] = n
			}
			if _, err := db.db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_backfill BEFORE %s ON %s BEGIN SELECT RAISE(ABORT,'backfill denied'); END`, tc.operation, tc.table)); err != nil {
				t.Fatal(err)
			}
			progress := 0
			err = db.BackfillDerived(BackfillDerivedOpts{OnProgress: func(int, int, int, int) { progress++ }})
			if err == nil || !strings.Contains(err.Error(), "backfill denied") || progress != 0 {
				t.Fatalf("backfill: %v progress=%d", err, progress)
			}
			for table, want := range counts {
				var got int
				if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Fatalf("partial %s: %d want %d", table, got, want)
				}
			}
			if _, err := db.db.Exec("DROP TRIGGER reject_backfill"); err != nil {
				t.Fatal(err)
			}
			if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
				t.Fatal(err)
			}
			commands, err := db.AggregateCommands(AggregateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(commands) != 1 || commands[0].CommandHead != "go" || commands[0].Count != 1 {
				t.Fatalf("retry commands: %+v", commands)
			}
			signals, err := db.AggregateCorrections(CorrectionAggOpts{})
			if err != nil {
				t.Fatal(err)
			}
			if len(signals) != 1 || signals[0].UUID != "u1" {
				t.Fatalf("retry corrections: %+v", signals)
			}
			templates, err := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1})
			if err != nil {
				t.Fatal(err)
			}
			if len(templates) != 1 || templates[0].NormalizationVersion != CurrentNormalizationVersion {
				t.Fatalf("retry templates: %+v", templates)
			}
		})
	}
}

func TestPerennialProjectResolutionFailureRollsBackEarlierPaths(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, path := range []string{"/a", "/b"} {
		if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: path, Project: "unknown", Hash: path, Messages: []IndexedMessage{{UUID: path, Text: "retained", ContentType: "text"}}}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.db.Exec(`CREATE TRIGGER reject_project BEFORE UPDATE ON search_items WHEN NEW.source_path='/b' BEGIN SELECT RAISE(ABORT,'project denied'); END`); err != nil {
		t.Fatal(err)
	}
	n, err := db.ReresolveProjects(context.Background(), func(string) string { return "omp" })
	if err == nil || !strings.Contains(err.Error(), "project denied") || n != 0 {
		t.Fatalf("resolution: %d %v", n, err)
	}
	var nUnknown int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE project='unknown'`).Scan(&nUnknown); err != nil {
		t.Fatal(err)
	}
	if nUnknown != 2 {
		t.Fatalf("partial resolution: %d", nUnknown)
	}
	if _, err := db.db.Exec("DROP TRIGGER reject_project"); err != nil {
		t.Fatal(err)
	}
	n, err = db.ReresolveProjects(context.Background(), func(string) string { return "omp" })
	if err != nil || n != 2 {
		t.Fatalf("retry: %d %v", n, err)
	}
	rows, err := db.ListSessions("omp", 0)
	if err != nil || len(rows) != 2 {
		t.Fatalf("resolved consumer: %+v %v", rows, err)
	}
}

// Closing replay backlogs must be atomic even when the native source no
// longer emits the retained call. A failed closure cannot advance its epoch.
func TestOmittedPerennialProvenanceClosureFailureCanRetry(t *testing.T) {
	for _, condition := range []string{"NEW.search_echo=1", "NEW.search_echo=0", "NEW.origin_version=1"} {
		t.Run(condition, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "closure.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.db.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,content_type,search_echo,origin_version) VALUES('session','/omp',0,'assistant','bash command=backscroll search retained','call','tool',NULL,0),('session','/omp',1,'user','retained','human','text',NULL,0); INSERT INTO indexed_files(path,hash) VALUES('/omp','old')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_closure BEFORE UPDATE ON search_items WHEN %s BEGIN SELECT RAISE(ABORT,'closure denied'); END`, condition)); err != nil {
				t.Fatal(err)
			}
			file := IndexedFile{Source: "session", SourcePath: "/omp", Hash: "new"}
			err = db.SyncFiles([]IndexedFile{file})
			if err == nil || !strings.Contains(err.Error(), "closure denied") {
				t.Fatalf("closure: %v", err)
			}
			var n int
			var hash string
			if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE search_echo IS NULL AND origin_version=0`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 2 {
				t.Fatalf("partial closure: %d", n)
			}
			if err := db.db.QueryRow(`SELECT hash FROM indexed_files WHERE path='/omp'`).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			if hash != "old" {
				t.Fatalf("advanced failed hash: %s", hash)
			}
			if _, err := db.db.Exec("DROP TRIGGER reject_closure"); err != nil {
				t.Fatal(err)
			}
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE origin_version=1 AND ((uuid='call' AND search_echo=1) OR (uuid='human' AND search_echo=0))`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 2 {
				t.Fatalf("retry lost closure: %d", n)
			}
			paths, err := db.PendingSearchEchoPaths()
			if err != nil || len(paths) != 0 {
				t.Fatalf("closed queue: %+v %v", paths, err)
			}
		})
	}
}

// A transition from identity-less history must not delete its old records if
// replacement fails. The same rollback contract applies to legacy reloads.
func TestLegacyReloadAndIdentityTransitionDeletionFailures(t *testing.T) {
	for _, perennial := range []bool{false, true} {
		for _, table := range []string{"template_matches", "search_items", "tool_events", "correction_signals"} {
			if perennial && table != "search_items" && table != "tool_events" {
				continue
			}
			t.Run(fmt.Sprintf("%t/%s", perennial, table), func(t *testing.T) {
				db, err := Open(filepath.Join(t.TempDir(), "transition.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				bad := true
				old := IndexedFile{Source: "session", SourcePath: "/omp", Hash: "old", Messages: []IndexedMessage{{Role: "user", Text: "No, that's wrong. Use the other file instead.", ContentType: "text"}, {Ordinal: 1, Role: "assistant", Text: "bash command=go test", ContentType: "tool", ToolName: "bash", CommandHead: "go"}, {Ordinal: 2, Role: "tool", Text: "error: connection refused 123", ContentType: "tool", IsError: &bad}}}
				if err := db.SyncFiles([]IndexedFile{old}); err != nil {
					t.Fatal(err)
				}
				var before int
				if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&before); err != nil {
					t.Fatal(err)
				}
				if before == 0 {
					t.Fatalf("missing rejection fixture: %s", table)
				}
				if _, err := db.db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_transition BEFORE DELETE ON %s BEGIN SELECT RAISE(ABORT,'transition denied'); END`, table)); err != nil {
					t.Fatal(err)
				}
				incoming := IndexedFile{Source: "session", SourcePath: "/omp", Hash: "new", Messages: []IndexedMessage{{Role: "user", Text: "replacement", ContentType: "text"}}}
				if perennial {
					incoming.Messages[0].UUID = "identified"
				}
				err = db.SyncFiles([]IndexedFile{incoming})
				if err == nil || !strings.Contains(err.Error(), "transition denied") {
					t.Fatalf("transition: %v", err)
				}
				var n int
				if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path='/omp'`).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n != 3 {
					t.Fatalf("old history lost: %d", n)
				}
				var hash string
				if err := db.db.QueryRow(`SELECT hash FROM indexed_files WHERE path='/omp'`).Scan(&hash); err != nil {
					t.Fatal(err)
				}
				if hash != "old" {
					t.Fatalf("failed replacement hash: %s", hash)
				}
				if _, err := db.db.Exec("DROP TRIGGER reject_transition"); err != nil {
					t.Fatal(err)
				}
				if err := db.SyncFiles([]IndexedFile{incoming}); err != nil {
					t.Fatal(err)
				}
				records, err := db.QueryIndexedRecords(IndexedRecordQuery{})
				if err != nil || len(records) != 1 || records[0].Text != "replacement" {
					t.Fatalf("retry consumer: %+v %v", records, err)
				}
			})
		}
	}
}

func TestPerennialPatternTimeBoundariesAndTagScope(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "filters.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bad := true
	code := 7
	for i, stamp := range []string{"2026-09-01T00:00:00Z", "2026-09-02T00:00:00Z", "2026-09-03T00:00:00Z"} {
		if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: fmt.Sprintf("/omp/%d", i), Hash: "hash", Project: "omp", Tags: []string{"testing"}, Messages: []IndexedMessage{{UUID: fmt.Sprintf("call%d", i), Role: "assistant", Text: "bash command=go test", ContentType: "tool", ToolName: "bash", CommandHead: "go", IsError: &bad, ExitCode: &code, Timestamp: stamp}}}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		tag  string
		want int
	}{{"testing", 1}, {"absent", 0}} {
		opts := AggregateOptions{Project: "omp", Tag: tc.tag, StartDate: "2026-09-02T00:00:00Z", EndDate: "2026-09-03T00:00:00Z", Limit: 10}
		commands, err := db.AggregateCommands(opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(commands) != tc.want || (len(commands) > 0 && commands[0].Count != 1) {
			t.Fatalf("commands %s: %+v", tc.tag, commands)
		}
		failures, err := db.AggregateFailures(opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(failures) != tc.want || (len(failures) > 0 && (failures[0].Count != 1 || failures[0].SignalledEvents != 1)) {
			t.Fatalf("failures %s: %+v", tc.tag, failures)
		}
		excluded := 0
		trends, err := db.AggregateCommandsTrend(opts, &excluded)
		if err != nil {
			t.Fatal(err)
		}
		if len(trends) != tc.want || excluded != 0 || (len(trends) > 0 && trends[0].Count != 1) {
			t.Fatalf("command trend %s: %+v excluded=%d", tc.tag, trends, excluded)
		}
		failureTrends, err := db.AggregateFailuresTrend(opts, &excluded)
		if err != nil {
			t.Fatal(err)
		}
		if len(failureTrends) != tc.want || excluded != 0 || (len(failureTrends) > 0 && (failureTrends[0].Count != 1 || failureTrends[0].SignalledEvents != 1)) {
			t.Fatalf("failure trend %s: %+v excluded=%d", tc.tag, failureTrends, excluded)
		}
	}
}

// Cancellation is delivered by a real SQLite trigger at durable stage
// boundaries, not by sleeps or a context that counts implementation calls.
func TestExpiredBackfillCancellationAtDerivedWriteBoundaries(t *testing.T) {
	for _, table := range []string{"message_templates", "template_matches", "correction_signals", "tool_events"} {
		t.Run(table, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "cancel.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for i, msg := range []struct{ role, text, kind string }{{"tool", "error: connection refused 123", "tool"}, {"user", "No, that's wrong. Use the other file instead.", "text"}, {"assistant", "bash command=go test", "tool"}} {
				if _, err := db.db.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,content_type) VALUES('session','/expired',?,?,?,?,?)`, i, msg.role, msg.text, fmt.Sprintf("c%d", i), msg.kind); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			setSyncCancellation(t, cancel)
			if _, err := db.db.Exec(fmt.Sprintf(`CREATE TRIGGER cancel_backfill AFTER INSERT ON %s BEGIN SELECT %s(); END`, table, cancelSyncFunction)); err != nil {
				t.Fatal(err)
			}
			progress := 0
			err = db.BackfillDerivedContext(ctx, BackfillDerivedOpts{OnProgress: func(int, int, int, int) { progress++ }})
			if !errors.Is(err, context.Canceled) || progress != 0 {
				t.Fatalf("cancel: %v progress=%d", err, progress)
			}
			for _, table := range []string{"message_templates", "template_matches", "correction_signals", "tool_events"} {
				var n int
				if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n != 0 {
					t.Fatalf("canceled batch retained %s: %d", table, n)
				}
			}
			if _, err := db.db.Exec("DROP TRIGGER cancel_backfill"); err != nil {
				t.Fatal(err)
			}
			if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
				t.Fatal(err)
			}
			commands, err := db.AggregateCommands(AggregateOptions{})
			if err != nil || len(commands) != 1 || commands[0].Count != 1 {
				t.Fatalf("retry commands: %+v %v", commands, err)
			}
		})
	}
}
