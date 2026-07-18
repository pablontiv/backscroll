package storage

import (
	"path/filepath"
	"testing"
)

func TestAggregateTemplatesMinSupport(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync messages that will mine into templates.
	msgs := []IndexedMessage{
		{Ordinal: 0, UUID: "u1", ToolName: "Bash", IsError: boolPtr(true), Text: "error: database locked 1", ExtractionVersion: 1},
		{Ordinal: 1, UUID: "u2", ToolName: "Bash", IsError: boolPtr(true), Text: "error: database locked 2", ExtractionVersion: 1},
		{Ordinal: 2, UUID: "u3", ToolName: "Bash", IsError: boolPtr(true), Text: "error: timeout", ExtractionVersion: 1},
	}
	files := []IndexedFile{{SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj", Messages: msgs}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Query with min_support=3: should NOT return the "timeout" template (only 1 occurrence).
	// Should return "database locked" template (2 occurrences, but below 3, so NOT returned either).
	rows, err := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 3})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 templates with min_support=3, got %d: %v", len(rows), rows)
	}

	// Query with min_support=1: should return both.
	rows, err = db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(rows) < 2 {
		t.Errorf("want >=2 templates with min_support=1, got %d", len(rows))
	}
}

func TestAggregateTemplatesQuery(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	msgs := []IndexedMessage{
		{Ordinal: 0, UUID: "u1", ToolName: "Bash", IsError: boolPtr(true),
			Text: "error: timeout", ExtractionVersion: 1},
	}
	files := []IndexedFile{
		{SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj", Messages: msgs},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	rows, err := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("want 1 template, got %d", len(rows))
	}
}

func TestAggregateTemplatesProjectFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	msgs := []IndexedMessage{
		{Ordinal: 0, UUID: "u1", ToolName: "Bash", IsError: boolPtr(true), Text: "error: thing failed", ExtractionVersion: 1},
	}
	files := []IndexedFile{
		{SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "proj_a", Messages: msgs},
		{SourcePath: "/q/s2.jsonl", Source: "session", Hash: "h2", Project: "proj_b", Messages: msgs},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	rows, err := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1, Project: "proj_a"})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected templates for proj_a")
	}

	// Sample UUIDs should only include u1.
	if len(rows[0].SampleUUIDs) > 1 || (len(rows[0].SampleUUIDs) == 1 && rows[0].SampleUUIDs[0] != "u1") {
		t.Errorf("project filter failed: got uuids %v", rows[0].SampleUUIDs)
	}
}

func TestAggregateTemplatesDerivedCount(t *testing.T) {
	// Verify that occurrence_count is derived from template_matches at query time
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	msgs := []IndexedMessage{
		{Ordinal: 0, UUID: "u1", ToolName: "Bash", IsError: boolPtr(true), Text: "error: broken", ExtractionVersion: 1},
		{Ordinal: 1, UUID: "u2", ToolName: "Bash", IsError: boolPtr(true), Text: "error: broken", ExtractionVersion: 1},
		{Ordinal: 2, UUID: "u3", ToolName: "Bash", IsError: boolPtr(true), Text: "error: broken", ExtractionVersion: 1},
	}
	files := []IndexedFile{
		{SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj", Messages: msgs},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	rows, err := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expect 1 template, got %d", len(rows))
	}

	if rows[0].OccurrenceCount != 3 {
		t.Errorf("derived occurrence_count = %d, want 3", rows[0].OccurrenceCount)
	}
}

func TestPurgeDeletesOrphanedTemplates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	msgs := []IndexedMessage{
		{Ordinal: 0, UUID: "u1", ToolName: "Bash", IsError: boolPtr(true),
			Text: "error: failed", Timestamp: "2026-01-01T00:00:00Z", ExtractionVersion: 1},
	}
	files := []IndexedFile{
		{SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj", Messages: msgs},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Verify template exists
	var tmplCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates`).Scan(&tmplCount); err != nil {
		t.Fatal(err)
	}
	if tmplCount != 1 {
		t.Fatalf("expect 1 template before purge, got %d", tmplCount)
	}

	// Purge everything
	if _, err := db.Purge("2026-02-01T00:00:00Z"); err != nil {
		t.Fatalf("purge: %v", err)
	}

	// Orphaned template should be gone
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates`).Scan(&tmplCount); err != nil {
		t.Fatal(err)
	}
	if tmplCount != 0 {
		t.Errorf("expect 0 templates after purge, got %d", tmplCount)
	}
}

func TestAggregateTemplatesEmpty(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1})
	if err != nil {
		t.Fatalf("aggregate empty: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expect 0 templates from empty db, got %d", len(rows))
	}
}

func TestAggregateTemplatesTagFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync two sessions with same template, one tagged, one not
	msgs := []IndexedMessage{
		{Ordinal: 0, UUID: "u1", ToolName: "Bash", IsError: boolPtr(true), Text: "error: connection failed", ExtractionVersion: 1},
	}
	files := []IndexedFile{
		{SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "proj", Messages: msgs, Tags: []string{"debugging"}},
		{SourcePath: "/q/s2.jsonl", Source: "session", Hash: "h2", Project: "proj", Messages: msgs},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Query without tag filter: should include both sessions
	rows, err := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected templates without filter")
	}
	// Both sessions should be in projects_affected
	if rows[0].OccurrenceCount != 2 {
		t.Errorf("unfiltered query: occurrence_count = %d, want 2", rows[0].OccurrenceCount)
	}

	// Query with tag filter: should only include tagged session
	rows, err = db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1, Tag: "debugging"})
	if err != nil {
		t.Fatalf("aggregate with tag filter: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected templates with tag filter")
	}
	if rows[0].OccurrenceCount != 1 {
		t.Errorf("tag-filtered query: occurrence_count = %d, want 1", rows[0].OccurrenceCount)
	}

	// Query with different tag: should return no results
	rows, err = db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1, Tag: "refactoring"})
	if err != nil {
		t.Fatalf("aggregate with different tag: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 templates with non-existent tag, got %d", len(rows))
	}
}
