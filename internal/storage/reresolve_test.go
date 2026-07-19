package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/projects"
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

func TestReresolveProjectsWithRegistry_EmptyRegistry(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert a session labeled with a fallback ID
	fallbackID := "mycode"
	sessionPath := "/Users/pones/.claude/sessions/2026-07-17T12-00-00_mycode.jsonl"
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, uuid, project, content_type)
		VALUES ('session', ?, 0, 'user', 'test', 'u1', ?, 'text')
	`, sessionPath, fallbackID)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}

	// Create a registry entry (empty registry for now)
	registry := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{},
	}

	// Re-resolve with registry (should run without error)
	updated, err := db.ReresolveProjectsWithRegistry(context.Background(), registry)
	if err != nil {
		t.Fatalf("reresolve: %v", err)
	}

	// With empty registry, should return 0 updates
	if updated != 0 {
		t.Errorf("expected 0 updates with empty registry, got %d", updated)
	}

	// Verify that rows remain in search_items (no deletions)
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM search_items").Scan(&count)
	if err != nil {
		t.Fatalf("query row count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row in search_items, got %d", count)
	}
}

func TestReresolveProjectsWithRegistry_NoNullProjects(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert sessions with no project (NULL)
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, content_type)
		VALUES ('session', '/p/s1.jsonl', 0, 'user', 'text', 'text')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	registry := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{},
	}

	updated, err := db.ReresolveProjectsWithRegistry(context.Background(), registry)
	if err != nil {
		t.Fatalf("reresolve: %v", err)
	}
	if updated != 0 {
		t.Errorf("expected 0 updates for NULL projects, got %d", updated)
	}
}

func TestReresolveProjectsWithRegistry_NoDecodeablePathsSkipped(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert sessions with paths that cannot be decoded (no /.claude/projects/ marker)
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, uuid, project, content_type)
		VALUES ('session', '/some/path.jsonl', 0, 'user', 'text', 'u1', 'fallback-proj', 'text')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	registry := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{},
	}

	updated, err := db.ReresolveProjectsWithRegistry(context.Background(), registry)
	if err != nil {
		t.Fatalf("reresolve: %v", err)
	}
	if updated != 0 {
		t.Errorf("expected 0 updates for undecodeable paths, got %d", updated)
	}
}

func TestReresolveProjectsWithRegistry_SkipFallbackOnly(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert sessions with non-registry projects (fallback-only IDs)
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, uuid, project, content_type)
		VALUES ('session', '/home/user/mycode/file.jsonl', 0, 'user', 'text', 'u1', 'mycode', 'text'),
		       ('session', '/home/user/mycode/file.jsonl', 1, 'user', 'text', 'u2', 'mycode', 'text')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Registry with no entries that match the fallback-only paths
	registry := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:    "other-proj",
				Roots: []string{"/home/user/other"},
			},
		},
	}

	updated, err := db.ReresolveProjectsWithRegistry(context.Background(), registry)
	if err != nil {
		t.Fatalf("reresolve: %v", err)
	}
	// No registry match, so no updates
	if updated != 0 {
		t.Errorf("expected 0 updates for fallback-only paths with no registry match, got %d", updated)
	}

	// Verify projects unchanged
	var proj1 string
	row := db.db.QueryRow("SELECT project FROM search_items WHERE source_path = '/home/user/mycode/file.jsonl' AND ordinal = 0")
	if err := row.Scan(&proj1); err != nil {
		t.Fatalf("query project: %v", err)
	}
	if proj1 != "mycode" {
		t.Errorf("expected project 'mycode', got %q", proj1)
	}
}

func TestReresolveProjectsWithRegistry_ContextHandling(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert a row with a valid project
	_, err = db.db.Exec(`
		INSERT INTO search_items
		(source, source_path, ordinal, role, text, uuid, project, content_type)
		VALUES ('session', '/some/path.jsonl', 0, 'user', 'text', 'u1', 'proj1', 'text')
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Create a registry
	registry := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{},
	}

	// Test with context
	updated, err := db.ReresolveProjectsWithRegistry(context.Background(), registry)
	if err != nil {
		t.Fatalf("reresolve: %v", err)
	}
	// With empty registry, no updates
	if updated != 0 {
		t.Errorf("expected 0 updates, got %d", updated)
	}
}
