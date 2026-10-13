package storage

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

// A failed append must not publish its new hash, partially enrich a retained
// call, or leave satellites that consumers could mistake for committed events.
func TestOMPPerennialAppendStorageFailuresAreAtomic(t *testing.T) {
	for _, tc := range []struct{ table, operation, condition string }{
		{"search_items", "INSERT", "NEW.source_path='/omp'"},
		{"search_items", "UPDATE", "NEW.source_path='/omp' AND NEW.search_echo=1"},
		{"search_items", "UPDATE", "NEW.source_path='/omp' AND NEW.was_interrupted=1"},
		{"search_items", "UPDATE", "NEW.source_path='/omp' AND NEW.origin='assistant'"},
		{"tool_events", "INSERT", "NEW.source_path='/omp'"},
		{"tool_events", "UPDATE", "NEW.source_path='/omp' AND NEW.message_uuid='call'"},
		{"session_tags", "DELETE", "OLD.source_path='/omp'"},
		{"session_tags", "INSERT", "NEW.source_path='/omp'"},
		{"indexed_files", "INSERT", "NEW.path='/omp'"},
		{"message_templates", "INSERT", "1"},
		{"template_matches", "INSERT", "NEW.source_path='/omp'"},
		{"correction_signals", "DELETE", "OLD.source_path='/omp'"},
		{"correction_signals", "INSERT", "NEW.source_path='/omp'"},
		{"dynamic_stopwords", "DELETE", "1"},
		{"dynamic_stopwords", "INSERT", "1"},
	} {
		t.Run(tc.table+"/"+tc.operation+"/"+tc.condition, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "atomic.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			original := IndexedFile{Source: "session", SourcePath: "/omp", Hash: "original", Tags: []string{"testing"}, Messages: []IndexedMessage{{UUID: "call", Ordinal: 0, Role: "assistant", Text: "bash command=go test ./...", ContentType: "tool", ToolName: "bash", CommandHead: "go"}}}
			if err := db.SyncFiles([]IndexedFile{original}); err != nil {
				t.Fatal(err)
			}
			// The DELETE trigger needs a real old signal to reject, just as a detector
			// epoch upgrade would encounter when reparsing a retained native session.
			if _, err := db.db.Exec(`INSERT INTO correction_signals(item_uuid,source_path,ordinal,detector,confidence,extraction_version) VALUES('call','/omp',0,'lexicon',1,0)`); err != nil {
				t.Fatal(err)
			}
			// DELETE triggers fire per row. Seed a prior vocabulary entry so
			// clearing the old snapshot actually encounters the rejection.
			if tc.table == "dynamic_stopwords" && tc.operation == "DELETE" {
				if _, err := db.db.Exec(`INSERT INTO dynamic_stopwords(term) VALUES('retained')`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_append BEFORE %s ON %s WHEN %s BEGIN SELECT RAISE(ABORT,'append denied'); END`, tc.operation, tc.table, tc.condition)); err != nil {
				t.Fatal(err)
			}
			isError, code := true, 7
			incoming := original
			incoming.Hash = "appended"
			incoming.Messages = append([]IndexedMessage(nil), original.Messages...)
			incoming.Messages[0].Origin = models.OriginAssistant
			incoming.Messages[0].SearchEcho = true
			incoming.Messages[0].WasInterrupted = true
			incoming.Messages[0].IsError = &isError
			incoming.Messages[0].ExitCode = &code
			incoming.Messages = append(incoming.Messages,
				IndexedMessage{UUID: "result", Ordinal: 0, Role: "tool", Text: "error: connection refused 127.0.0.1:9000", ContentType: "tool", IsError: &isError},
				IndexedMessage{UUID: "next", Ordinal: 1, Role: "assistant", Text: "bash command=make", ContentType: "tool", ToolName: "bash", CommandHead: "make"},
				IndexedMessage{UUID: "correction", Ordinal: 2, Role: "user", Origin: models.OriginHuman, Text: "No, that's wrong. Use the other file instead.", ContentType: "text"})
			prefix := IndexedFile{Source: "session", SourcePath: "/prefix", Hash: "prefix", Messages: []IndexedMessage{{UUID: "prefix", Text: "prefix", ContentType: "text"}}}
			err = db.SyncFiles([]IndexedFile{prefix, incoming})
			if err == nil || !strings.Contains(err.Error(), "append denied") {
				t.Fatalf("expected storage rejection, got %v", err)
			}
			var hash string
			var count, interrupted, echo int
			var origin string
			if err := db.db.QueryRow(`SELECT hash FROM indexed_files WHERE path='/omp'`).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			if err := db.db.QueryRow(`SELECT was_interrupted,search_echo,origin FROM search_items WHERE uuid='call'`).Scan(&interrupted, &echo, &origin); err != nil {
				t.Fatal(err)
			}
			if hash != "original" || interrupted != 0 || echo != 0 || origin != "unknown" {
				t.Fatalf("partial append committed: %s %d %d %s", hash, interrupted, echo, origin)
			}
			for _, query := range []string{
				`SELECT COUNT(*) FROM search_items WHERE uuid!='call'`,
				`SELECT COUNT(*) FROM indexed_files WHERE path='/prefix'`,
				`SELECT COUNT(*) FROM tool_events WHERE message_uuid!='call' OR is_error IS NOT NULL OR exit_code IS NOT NULL`,
				`SELECT COUNT(*) FROM template_matches`,
				`SELECT COUNT(*) FROM message_templates`,
			} {
				if err := db.db.QueryRow(query).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("partial rows survived: %s = %d", query, count)
				}
			}
			if _, err := db.db.Exec(`DROP TRIGGER reject_append`); err != nil {
				t.Fatal(err)
			}
			if err := db.SyncFiles([]IndexedFile{prefix, incoming}); err != nil {
				t.Fatalf("retry failed: %v", err)
			}
			failures, err := db.AggregateFailures(AggregateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(failures) != 1 || failures[0].ExitCode == nil || *failures[0].ExitCode != 7 || failures[0].Count != 1 {
				t.Fatalf("retry failure consumer: %+v", failures)
			}
		})
	}
}
