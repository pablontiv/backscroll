package storage

import (
	"database/sql"
	"reflect"
	"testing"
)

func TestPendingOriginPathsIsDeterministicAndBounded(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	indexed := []struct {
		path        string
		lastIndexed string
	}{
		{path: "/p/a.jsonl", lastIndexed: "2026-01-03T00:00:00Z"},
		{path: "/p/b.jsonl", lastIndexed: "2026-01-03T00:00:00Z"},
		{path: "/p/c.jsonl", lastIndexed: "2026-01-02T00:00:00Z"},
		{path: "/p/current.jsonl", lastIndexed: "2026-01-04T00:00:00Z"},
	}
	for _, file := range indexed {
		if _, err := db.db.Exec(`
			INSERT INTO indexed_files (path, hash, last_indexed) VALUES (?, 'hash', ?)
		`, file.path, file.lastIndexed); err != nil {
			t.Fatalf("insert indexed file %q: %v", file.path, err)
		}
	}

	for _, path := range []string{"/p/a.jsonl", "/p/b.jsonl", "/p/c.jsonl"} {
		if _, err := db.db.Exec(`
			INSERT INTO search_items
				(source, source_path, ordinal, role, text, content_type, origin, origin_version)
			VALUES ('session', ?, 0, 'user', 'historical', 'text', 'unknown', NULL)
		`, path); err != nil {
			t.Fatalf("insert pending row %q: %v", path, err)
		}
	}
	if _, err := db.db.Exec(`
		INSERT INTO search_items
			(source, source_path, ordinal, role, text, content_type, origin, origin_version)
		VALUES ('session', '/p/current.jsonl', 0, 'user', 'current', 'text', 'human', 1)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO search_items
			(source, source_path, ordinal, role, text, content_type, origin, origin_version)
		VALUES ('session', '/p/orphan.jsonl', 0, 'user', 'orphan', 'text', 'unknown', NULL)
	`); err != nil {
		t.Fatal(err)
	}

	got, err := db.PendingOriginPaths(2)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/p/a.jsonl", "/p/b.jsonl"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pending origin paths = %v, want %v", got, want)
	}

	again, err := db.PendingOriginPaths(2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, got) {
		t.Fatalf("pending queue changed without writes: first=%v second=%v", got, again)
	}
}

func TestPendingOriginPathsPreservesHistoricalEvidenceAndConverges(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	const path = "/p/historical.jsonl"
	if _, err := db.db.Exec(`
		INSERT INTO indexed_files (path, hash, last_indexed)
		VALUES (?, 'hash', '2026-01-01T00:00:00Z')
	`, path); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO search_items
			(source, source_path, ordinal, role, text, content_type, origin, origin_version)
		VALUES ('session', ?, 0, 'assistant', 'role is not origin evidence', 'text', 'unknown', NULL)
	`, path); err != nil {
		t.Fatal(err)
	}

	if got, err := db.PendingOriginPaths(1); err != nil || !reflect.DeepEqual(got, []string{path}) {
		t.Fatalf("initial pending queue = %v, err=%v", got, err)
	}
	var origin string
	var version sql.NullInt64
	if err := db.db.QueryRow(`
		SELECT origin, origin_version FROM search_items WHERE source_path = ?
	`, path).Scan(&origin, &version); err != nil {
		t.Fatal(err)
	}
	if origin != "unknown" || version.Valid {
		t.Fatalf("queue query altered historical provenance: origin=%q version=%+v", origin, version)
	}

	if _, err := db.db.Exec(`
		UPDATE search_items SET origin = 'assistant', origin_version = 1 WHERE source_path = ?
	`, path); err != nil {
		t.Fatal(err)
	}
	if got, err := db.PendingOriginPaths(1); err != nil || len(got) != 0 {
		t.Fatalf("processed path remained pending: %v, err=%v", got, err)
	}

	for _, limit := range []int{0, -1} {
		if got, err := db.PendingOriginPaths(limit); err != nil || got != nil {
			t.Fatalf("PendingOriginPaths(%d) = %v, err=%v; want nil, nil", limit, got, err)
		}
	}
}
