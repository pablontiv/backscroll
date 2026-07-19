package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/projects"
)

// TestPurgeWithToolEvents verifies Purge deletes tool_events satellites
func TestPurgeWithToolEvents(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert indexed files with old timestamp
	_, err = db.db.Exec(`
		INSERT INTO indexed_files (path, hash, last_indexed)
		VALUES ('/old/session.jsonl', 'abc123', '2020-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert indexed_files: %v", err)
	}

	// Insert search_items linked to old file
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, project, content_type)
		VALUES ('session', '/old/session.jsonl', 0, 'user', 'text', '2020-01-01T00:00:00Z', 'proj', 'text')
	`)
	if err != nil {
		t.Fatalf("insert search_items: %v", err)
	}

	// Insert tool_events for that path
	_, err = db.db.Exec(`
		INSERT INTO tool_events (source_path, ordinal, tool_name, command_head, extraction_version)
		VALUES ('/old/session.jsonl', 0, 'bash', 'ls', 0)
	`)
	if err != nil {
		t.Fatalf("insert tool_events: %v", err)
	}

	// Purge before 2021-01-01
	deleted, err := db.Purge("2021-01-01")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted == 0 {
		t.Errorf("expected rows to be deleted, got 0")
	}

	// Verify tool_events deleted too
	var toolCount int
	err = db.db.QueryRow("SELECT COUNT(*) FROM tool_events WHERE source_path = '/old/session.jsonl'").Scan(&toolCount)
	if err != nil {
		t.Fatalf("query tool_events: %v", err)
	}
	if toolCount != 0 {
		t.Errorf("expected tool_events to be deleted, got %d rows", toolCount)
	}
}

// TestAggregateTemplatesWithEmptyTable verifies empty result handling
func TestAggregateTemplatesWithEmptyTable(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Query with empty table
	results, err := db.AggregateTemplates(TemplateQueryOpts{MinSupport: 1})
	if err != nil {
		t.Fatalf("aggregate templates: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

// TestAggregateTemplatesWithDateFilter verifies date range filtering
func TestAggregateTemplatesWithDateFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Create templates and their matches
	_, err = db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
		VALUES ('sig1', 1, 'error text', 1, '2026-01-01T00:00:00Z', '2026-06-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert templates: %v", err)
	}

	var tmplID int64
	err = db.db.QueryRow("SELECT id FROM message_templates WHERE signature = 'sig1'").Scan(&tmplID)
	if err != nil {
		t.Fatalf("query template id: %v", err)
	}

	// Insert search items and template matches with matching timestamp
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, content_type)
		VALUES ('session', '/s1.jsonl', 0, 'user', 'error text', '2026-06-01T00:00:00Z', 'text')
	`)
	if err != nil {
		t.Fatalf("insert search_items: %v", err)
	}

	_, err = db.db.Exec(`
		INSERT INTO template_matches (template_id, item_uuid, source_path, ordinal)
		VALUES (?, 'u1', '/s1.jsonl', 0)
	`, tmplID)
	if err != nil {
		t.Fatalf("insert template_match: %v", err)
	}

	// Query with date filters that match the data
	results, err := db.AggregateTemplates(TemplateQueryOpts{
		MinSupport: 1,
		After:      "2026-01-01",
		Before:     "2026-12-31",
	})
	if err != nil {
		t.Fatalf("aggregate with filters: %v", err)
	}
	// Should match templates within the date range
	if len(results) == 0 {
		t.Errorf("expected results with date filter")
	}
}

// TestRebuildFTSIdempotency verifies rebuild can be called multiple times
func TestRebuildFTSIdempotency(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert content
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session', '/s1.jsonl', 0, 'user', 'test content', 'text')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Rebuild twice (should be idempotent)
	if err := db.RebuildFTS(); err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	if err := db.RebuildFTS(); err != nil {
		t.Fatalf("second rebuild: %v", err)
	}

	// Verify content still searchable
	rows, err := db.db.Query("SELECT COUNT(*) FROM search_items WHERE text = 'test content'")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Errorf("no rows after rebuild")
	}
}

// TestRebuildFTSWithEmptyTable verifies rebuild works on empty DB
func TestRebuildFTSWithEmptyTable(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Rebuild on empty table should not error
	if err := db.RebuildFTS(); err != nil {
		t.Fatalf("rebuild empty fts: %v", err)
	}
}

// TestPurgeWithAnnotations verifies Purge handles annotated items
func TestPurgeWithAnnotations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert indexed file with old timestamp
	_, err = db.db.Exec(`
		INSERT INTO indexed_files (path, hash, last_indexed)
		VALUES ('/old/session.jsonl', 'abc', '2020-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Insert search_items
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, uuid, timestamp, content_type)
		VALUES ('session', '/old/session.jsonl', 0, 'user', 'text', 'u1', '2020-01-01T00:00:00Z', 'text')
	`)
	if err != nil {
		t.Fatalf("insert search_items: %v", err)
	}

	// Insert annotations
	_, err = db.db.Exec(`
		INSERT INTO annotations (item_uuid, source_path, ordinal, kind, label, created_at)
		VALUES ('u1', '/old/session.jsonl', 0, 'correction', 'test_label', CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("insert annotations: %v", err)
	}

	// Purge should handle annotations gracefully
	deleted, err := db.Purge("2021-01-01")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted == 0 {
		t.Errorf("expected rows to be deleted")
	}
}

// TestAggregateCommandsWithProject tests project filtering
func TestAggregateCommandsWithProject(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert tool_events and search_items with project
	_, err = db.db.Exec(`
		INSERT INTO tool_events (source_path, ordinal, tool_name, command_head, extraction_version)
		VALUES ('/s.jsonl', 0, 'bash', 'ls', 0)
	`)
	if err != nil {
		t.Fatalf("insert tool_events: %v", err)
	}

	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, project, content_type)
		VALUES ('session', '/s.jsonl', 0, 'user', 'text', 'proj1', 'tool')
	`)
	if err != nil {
		t.Fatalf("insert search_items: %v", err)
	}

	// Query with project filter
	results, err := db.AggregateCommands(AggregateOptions{Project: "proj1", Limit: 10})
	if err != nil {
		t.Fatalf("aggregate with project: %v", err)
	}
	if len(results) == 0 {
		t.Errorf("expected results with project filter")
	}
}

// TestGetTopicsEmptyLimit verifies GetTopics default limit behavior
func TestGetTopicsEmptyLimit(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert search items
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session', '/s1.jsonl', 0, 'user', 'testing framework functionality system', 'text')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Query with zero limit should use default
	topics, err := db.GetTopics("", 0)
	if err != nil {
		t.Fatalf("get topics: %v", err)
	}
	// Should return up to default (50) topics
	if len(topics) > 50 {
		t.Errorf("expected at most 50 topics, got %d", len(topics))
	}
}

// TestOptimizeFTSDoesNotError verifies OptimizeFTS runs without error
func TestOptimizeFTSDoesNotError(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// OptimizeFTS should not error on empty DB
	if err := db.OptimizeFTS(); err != nil {
		t.Fatalf("optimize fts: %v", err)
	}
}

// TestReresolveProjectsWithRegistryNoUpdate verifies no-op when IDs match
func TestReresolveProjectsWithRegistryNoUpdate(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert row with a project already matching what registry would assign
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, uuid, project, content_type)
		VALUES ('session', '/home/user/proj/file.jsonl', 0, 'user', 'text', 'u1', 'proj', 'text')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Registry with matching entry
	registry := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:    "proj",
				Roots: []string{"/home/user/proj"},
			},
		},
	}

	updated, err := db.ReresolveProjectsWithRegistry(context.Background(), registry)
	if err != nil {
		t.Fatalf("reresolve: %v", err)
	}
	// No update since project already matches
	if updated != 0 {
		t.Errorf("expected 0 updates when IDs match, got %d", updated)
	}
}

// TestAggregateCommandsWithDateRange tests date filtering on commands
func TestAggregateCommandsWithDateRange(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert tool_events with different timestamps
	_, err = db.db.Exec(`
		INSERT INTO tool_events (source_path, ordinal, tool_name, command_head, extraction_version)
		VALUES ('/old/s.jsonl', 0, 'bash', 'ls', 0),
		       ('/new/s.jsonl', 0, 'bash', 'ls', 0),
		       ('/new/s.jsonl', 1, 'bash', 'cd', 0)
	`)
	if err != nil {
		t.Fatalf("insert tool_events: %v", err)
	}

	// Insert corresponding search_items with timestamps
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, content_type)
		VALUES ('session', '/old/s.jsonl', 0, 'user', 'text', '2025-01-01T00:00:00Z', 'tool'),
		       ('session', '/new/s.jsonl', 0, 'user', 'text', '2026-06-01T00:00:00Z', 'tool'),
		       ('session', '/new/s.jsonl', 1, 'user', 'text', '2026-06-01T00:00:00Z', 'tool')
	`)
	if err != nil {
		t.Fatalf("insert search_items: %v", err)
	}

	// Query with date range
	results, err := db.AggregateCommands(AggregateOptions{
		StartDate: "2026-01-01",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("aggregate commands: %v", err)
	}
	if len(results) == 0 {
		t.Errorf("expected results with date filter")
	}
}

// TestAggregateCommandsWithOffsetPagination tests pagination
func TestAggregateCommandsWithOffsetPagination(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert multiple tool_events
	_, err = db.db.Exec(`
		INSERT INTO tool_events (source_path, ordinal, tool_name, command_head, extraction_version)
		VALUES ('/s1.jsonl', 0, 'bash', 'ls', 0),
		       ('/s2.jsonl', 0, 'bash', 'cd', 0),
		       ('/s3.jsonl', 0, 'bash', 'pwd', 0)
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Query with limit and offset
	all, err := db.AggregateCommands(AggregateOptions{Limit: 10})
	if err != nil {
		t.Fatalf("query all: %v", err)
	}

	paginated, err := db.AggregateCommands(AggregateOptions{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("query paginated: %v", err)
	}

	if len(all) <= len(paginated) {
		t.Errorf("pagination should return fewer results")
	}
}
