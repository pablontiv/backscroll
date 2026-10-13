package storage

import (
	"path/filepath"
	"testing"
)

func TestUnlimitedOffsetContract(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	yes := true
	for _, name := range []string{"c", "b", "a"} {
		err := db.SyncFiles([]IndexedFile{{SourcePath: "/" + name, Source: "session", Project: "p", Hash: name, Messages: []IndexedMessage{{UUID: name, Ordinal: 0, Role: "assistant", Text: "error", Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool", ToolName: name, CommandHead: name, IsError: &yes, ExtractionVersion: 2}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Run("list", func(t *testing.T) {
		rows, e := db.ListItemsV2(ListOptions{Offset: 1})
		if e != nil {
			t.Fatal(e)
		}
		if len(rows) != 2 || rows[0].Path != "/b" {
			t.Fatalf("rows: %+v", rows)
		}
	})
	t.Run("commands", func(t *testing.T) {
		rows, e := db.AggregateCommands(AggregateOptions{Offset: 1})
		if e != nil {
			t.Fatal(e)
		}
		if len(rows) != 2 {
			t.Fatalf("rows: %+v", rows)
		}
	})
	t.Run("failures", func(t *testing.T) {
		rows, e := db.AggregateFailures(AggregateOptions{Offset: 1})
		if e != nil {
			t.Fatal(e)
		}
		if len(rows) != 2 {
			t.Fatalf("rows: %+v", rows)
		}
	})
	t.Run("commands trend", func(t *testing.T) {
		rows, e := db.AggregateCommandsTrend(AggregateOptions{Offset: 1}, nil)
		if e != nil {
			t.Fatal(e)
		}
		if len(rows) != 2 {
			t.Fatalf("rows: %+v", rows)
		}
	})
	t.Run("failures trend", func(t *testing.T) {
		rows, e := db.AggregateFailuresTrend(AggregateOptions{Offset: 1}, nil)
		if e != nil {
			t.Fatal(e)
		}
		if len(rows) != 2 {
			t.Fatalf("rows: %+v", rows)
		}
	})
}

func TestLogicalPatternOrderingAndCorrectionsOffset(t *testing.T) {
	for _, names := range [][]string{{"c", "b", "a"}, {"a", "b", "c"}} {
		db, e := Open(filepath.Join(t.TempDir(), "index.db"))
		if e != nil {
			t.Fatal(e)
		}
		for _, name := range names {
			if e := db.SyncFiles([]IndexedFile{{SourcePath: "/" + name, Source: "session", Project: "p", Hash: name, Messages: []IndexedMessage{{UUID: name, Text: "no, correct this", Ordinal: 0, Role: "user", ContentType: "text"}}}}); e != nil {
				t.Fatal(e)
			}
			if _, e := db.db.Exec(`INSERT INTO correction_signals(item_uuid,source_path,ordinal,detector,confidence,extraction_version) VALUES (?, ?, 0, 'lexicon', 0.9, 1)`, name, "/"+name); e != nil {
				t.Fatal(e)
			}
			r, e := db.db.Exec(`INSERT INTO message_templates(signature,normalization_version,template_text,first_seen,last_seen) VALUES (?,1,?,'2026-01-01','2026-01-01')`, name, name)
			if e != nil {
				t.Fatal(e)
			}
			id, _ := r.LastInsertId()
			if _, e := db.db.Exec(`INSERT INTO template_matches(template_id,item_uuid,source_path,ordinal) VALUES (?,?,?,0)`, id, name, "/"+name); e != nil {
				t.Fatal(e)
			}
		}
		rows, e := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1})
		if e != nil {
			t.Fatal(e)
		}
		if len(rows) != 3 || rows[0].Signature != "a" || rows[1].Signature != "b" {
			t.Fatalf("templates: %+v", rows)
		}
		corrections, e := db.AggregateCorrections(CorrectionAggOpts{Offset: 1})
		if e != nil {
			t.Fatal(e)
		}
		if len(corrections) != 2 || corrections[0].SourcePath != "/b" {
			t.Fatalf("corrections: %+v", corrections)
		}
		sessions, e := db.ListItemsV2(ListOptions{Limit: 1, Offset: 1})
		if e != nil {
			t.Fatal(e)
		}
		if len(sessions) != 1 || sessions[0].Path != "/b" {
			t.Fatalf("sessions: %+v", sessions)
		}
		db.Close()
	}
}
