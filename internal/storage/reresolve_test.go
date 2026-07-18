package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestReresolveProjects_NoUnknownRows(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert a row with a known project
	_, err = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, project, content_type)
		VALUES ('session', '/p/s1.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', 'known', 'text')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	resolved, err := db.ReresolveProjects(context.Background(), func(path string) string {
		return "new-project"
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != 0 {
		t.Errorf("expected 0 resolved rows, got %d", resolved)
	}
}

func TestReresolveProjects_ResolveUnknown(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert rows with 'unknown' project
	_, err = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, project, content_type)
		VALUES ('session', '/p/s1.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', 'unknown', 'text'),
		       ('session', '/p/s1.jsonl', 1, 'user', 'text', '2026-01-01T00:00:00Z', 'unknown', 'text'),
		       ('session', '/p/s2.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', 'unknown', 'text')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Resolve with a function that returns project based on source_path
	resolved, err := db.ReresolveProjects(context.Background(), func(path string) string {
		if path == "/p/s1.jsonl" {
			return "project-a"
		}
		if path == "/p/s2.jsonl" {
			return "project-b"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// Should return 2 (distinct source_paths resolved), not 3 (row count)
	if resolved != 2 {
		t.Errorf("expected 2 resolved source_paths, got %d", resolved)
	}

	// Verify the updates (all rows from each path should be updated)
	var proj1, proj2 string
	row := db.db.QueryRow("SELECT project FROM search_items WHERE source_path = '/p/s1.jsonl' LIMIT 1")
	if err := row.Scan(&proj1); err != nil {
		t.Fatalf("query project for s1: %v", err)
	}
	if proj1 != "project-a" {
		t.Errorf("expected s1 project 'project-a', got %q", proj1)
	}

	row = db.db.QueryRow("SELECT project FROM search_items WHERE source_path = '/p/s2.jsonl' LIMIT 1")
	if err := row.Scan(&proj2); err != nil {
		t.Fatalf("query project for s2: %v", err)
	}
	if proj2 != "project-b" {
		t.Errorf("expected s2 project 'project-b', got %q", proj2)
	}

	// Verify all rows from s1 are updated (both rows should have project-a)
	var allProjectA int
	db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path = '/p/s1.jsonl' AND project = 'project-a'").Scan(&allProjectA)
	if allProjectA != 2 {
		t.Errorf("expected 2 rows with project-a for s1, got %d", allProjectA)
	}
}

func TestReresolveProjects_EmptyResolver(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert rows with 'unknown' project
	_, err = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, project, content_type)
		VALUES ('session', '/p/s1.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', 'unknown', 'text')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Resolver returns empty string for all paths
	resolved, err := db.ReresolveProjects(context.Background(), func(path string) string {
		return ""
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != 0 {
		t.Errorf("expected 0 resolved rows (empty resolver), got %d", resolved)
	}

	// Verify project is still 'unknown'
	var proj string
	row := db.db.QueryRow("SELECT project FROM search_items WHERE source_path = '/p/s1.jsonl'")
	if err := row.Scan(&proj); err != nil {
		t.Fatalf("query project: %v", err)
	}
	if proj != "unknown" {
		t.Errorf("expected project still 'unknown', got %q", proj)
	}
}

func TestReresolveProjects_ResolverReturnsUnknown(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert rows with 'unknown' project
	_, err = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, project, content_type)
		VALUES ('session', '/p/s1.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', 'unknown', 'text')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Resolver returns "unknown" (skipped)
	resolved, err := db.ReresolveProjects(context.Background(), func(path string) string {
		return "unknown"
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != 0 {
		t.Errorf("expected 0 resolved rows (resolver returns unknown), got %d", resolved)
	}
}

func TestReresolveProjects_WithNULLProject(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert rows with NULL project
	_, err = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, project, content_type)
		VALUES ('session', '/p/s1.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', NULL, 'text')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	resolved, err := db.ReresolveProjects(context.Background(), func(path string) string {
		return "resolved-from-null"
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved != 1 {
		t.Errorf("expected 1 resolved source_path from NULL, got %d", resolved)
	}

	// Verify the update
	var proj string
	row := db.db.QueryRow("SELECT project FROM search_items WHERE source_path = '/p/s1.jsonl'")
	if err := row.Scan(&proj); err != nil {
		t.Fatalf("query project: %v", err)
	}
	if proj != "resolved-from-null" {
		t.Errorf("expected project 'resolved-from-null', got %q", proj)
	}
}
