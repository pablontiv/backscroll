package storage

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pablontiv/backscroll/internal/templates"
)

// This is a real SQLite sync -> query -> backfill smoke, including historical
// same-signature rows and error proof that cannot be recovered from stored text.
func TestTemplateEpochSyncBackfillParity(t *testing.T) {
	for _, text := range []string{"error: database locked", "connection unavailable"} {
		t.Run(text, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "smoke.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			file := IndexedFile{SourcePath: "/epoch.jsonl", Source: "session", Hash: "h1", Project: "epoch", Messages: []IndexedMessage{
				{Ordinal: 0, UUID: "result", ContentType: "tool", Text: text, IsError: boolPtr(true), ExtractionVersion: CurrentExtractionVersion},
			}}
			sig := templates.NewMiner().ProcessLine(text).Signature
			if _, err := db.db.Exec(`INSERT INTO message_templates(signature,normalization_version,template_text,occurrence_count,first_seen,last_seen) VALUES(?,1,?,7,'2001-01-01','2002-01-01')`, sig, text); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(`INSERT INTO template_matches(template_id,item_uuid,source_path,ordinal) SELECT id,'retained','/expired.jsonl',0 FROM message_templates WHERE signature=?`, sig); err != nil {
				t.Fatal(err)
			}
			assert := func() []TemplateRow {
				t.Helper()
				rows, err := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1})
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 1 || rows[0].NormalizationVersion != CurrentNormalizationVersion || rows[0].OccurrenceCount != 2 {
					t.Fatalf("query: %+v", rows)
				}
				var count, matches, occurrences int
				var first, last string
				if err := db.db.QueryRow(`SELECT COUNT(*),SUM(occurrence_count),MIN(first_seen),MAX(last_seen) FROM message_templates`).Scan(&count, &occurrences, &first, &last); err != nil {
					t.Fatal(err)
				}
				if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches`).Scan(&matches); err != nil {
					t.Fatal(err)
				}
				if count != 1 || matches != 2 || occurrences != 7 || first != "2001-01-01" || last != "2002-01-01" {
					t.Fatalf("persistence drift: %d %d %d %s %s", count, matches, occurrences, first, last)
				}
				paths, err := db.StaleTemplatePaths(CurrentNormalizationVersion)
				if err != nil || len(paths) != 0 {
					t.Fatalf("stale paths: %v %v", paths, err)
				}
				return rows
			}
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			before := assert()
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			if after := assert(); !reflect.DeepEqual(before, after) {
				t.Fatalf("re-sync drift: %+v -> %+v", before, after)
			}
			if text == "error: database locked" {
				if _, err := db.db.Exec(`UPDATE message_templates SET normalization_version=1`); err != nil {
					t.Fatal(err)
				}
			}
			// Expire source discovery so BackfillDerived actually processes stored
			// text even when the sync observation has already advanced the epoch.
			if _, err := db.db.Exec(`DELETE FROM indexed_files WHERE path=?`, file.SourcePath); err != nil {
				t.Fatal(err)
			}
			// No ToolName was supplied, so there is no satellite error proof for the
			// structured-only result. Backfill must preserve but cannot recreate it.
			if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
				t.Fatal(err)
			}
			if after := assert(); !reflect.DeepEqual(before, after) {
				t.Fatalf("backfill drift: %+v -> %+v", before, after)
			}
			if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
				t.Fatal(err)
			}
			assert()
			if text == "connection unavailable" {
				if _, err := db.db.Exec(`DELETE FROM template_matches; DELETE FROM message_templates`); err != nil {
					t.Fatal(err)
				}
				if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
					t.Fatal(err)
				}
				var count int
				if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatal("backfill fabricated structured error proof")
				}
			}
		})
	}
}
