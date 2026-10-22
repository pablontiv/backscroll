package storage

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestV21FTSUpdates(t *testing.T) {
	t.Parallel()
	db, cleanup := newTestDB(t)
	defer cleanup()
	for i, kind := range []string{"text", "code", "reasoning", "tool"} {
		if _, err := db.db.Exec(`INSERT INTO search_items(id,source_path,ordinal,role,text,content_type,uuid) VALUES(?, '/fts', ?, 'assistant', 'quartzmarker', ?, ?)`, i+1, i, kind, fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	match := func(index, term string, want int) {
		t.Helper()
		var got int
		if err := db.db.QueryRow(`SELECT COUNT(*) FROM `+index+` WHERE `+index+` MATCH ?`, term).Scan(&got); err != nil || got != want {
			t.Fatalf("%s MATCH %s = %d, want %d: %v", index, term, got, want, err)
		}
	}
	integrity := func() {
		t.Helper()
		for _, index := range []string{"messages_fts", "tool_fts"} {
			if _, err := db.db.Exec(`INSERT INTO ` + index + `(` + index + `) VALUES('integrity-check')`); err != nil {
				t.Fatal(err)
			}
		}
	}
	match("messages_fts", "quartzmarker", 3)
	match("tool_fts", "quartzmarker", 1)
	searchUUIDs := func() string {
		t.Helper()
		results, _, err := db.Search("quartzmarker", models.SearchOptions{AllProjects: true, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		var uuids []string
		for _, result := range results {
			uuids = append(uuids, *result.UUID)
		}
		return strings.Join(uuids, ",")
	}
	beforeUUIDs := searchUUIDs()
	var before, after int
	if err := db.db.QueryRow("SELECT total_changes()").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET project='proven',project_evidence='registry',origin='assistant',origin_version=1,search_echo=0,extraction_version=3,was_interrupted=1,session_id='native'`); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow("SELECT total_changes()").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after-before != 4 {
		t.Fatalf("metadata update changed %d rows including derived writes, want exactly 4", after-before)
	}
	match("messages_fts", "quartzmarker", 3)
	match("tool_fts", "quartzmarker", 1)
	if got := searchUUIDs(); got != beforeUUIDs {
		t.Fatalf("metadata update changed ranked search UUIDs: %s -> %s", beforeUUIDs, got)
	}
	integrity()
	for _, step := range []struct {
		sql                                  string
		proseOld, toolOld, proseNew, toolNew int
	}{
		{`UPDATE search_items SET text='cobaltmarker' WHERE id=1`, 2, 1, 1, 0},
		{`UPDATE search_items SET content_type='tool' WHERE id=1`, 2, 1, 0, 1},
		{`UPDATE search_items SET content_type='reasoning',text='cobaltmarker' WHERE id=4`, 2, 0, 1, 1},
		{`UPDATE search_items SET id=40 WHERE id=4`, 2, 0, 1, 1},
		{`UPDATE search_items SET content_type='other' WHERE id=40`, 2, 0, 0, 1},
		{`UPDATE search_items SET content_type='code' WHERE id=40`, 2, 0, 1, 1},
	} {
		if _, err := db.db.Exec(step.sql); err != nil {
			t.Fatal(err)
		}
		match("messages_fts", "quartzmarker", step.proseOld)
		match("tool_fts", "quartzmarker", step.toolOld)
		match("messages_fts", "cobaltmarker", step.proseNew)
		match("tool_fts", "cobaltmarker", step.toolNew)
		integrity()
	}
	if _, err := db.db.Exec(`DELETE FROM search_items`); err != nil {
		t.Fatal(err)
	}
	match("messages_fts", "quartzmarker OR cobaltmarker", 0)
	match("tool_fts", "quartzmarker OR cobaltmarker", 0)
	integrity()
}

func TestStartupMutationAccessPaths(t *testing.T) {
	t.Parallel()
	db, cleanup := newTestDB(t)
	defer cleanup()
	for _, tc := range []struct {
		name, query, access string
		args                []any
	}{
		{"legacy", legacySearchCleanupSQL, "idx_search_items_source_path (source_path=?)", []any{"/path"}},
		{"project", `UPDATE search_items INDEXED BY sqlite_autoindex_search_items_1 SET project=?,project_evidence=? WHERE source_path=? AND uuid=? AND text=? AND content_type=? AND (session_id IS NULL OR session_id IS ?) AND COALESCE(project_evidence,'unknown') IN ('unknown','path_fallback') AND ? IN ('path_fallback','registry','native_cwd')`, "sqlite_autoindex_search_items_1 (uuid=?)", []any{"p", "registry", "/path", "uuid", "text", "text", nil, "registry"}},
		{"origin", `UPDATE search_items INDEXED BY sqlite_autoindex_search_items_1 SET origin=CASE WHEN origin='unknown' THEN ? ELSE origin END,origin_version=? WHERE uuid=?`, "sqlite_autoindex_search_items_1 (uuid=?)", []any{"assistant", 1, "uuid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := db.db.Query("EXPLAIN QUERY PLAN "+tc.query, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			found := false
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				t.Log(detail)
				found = found || strings.Contains(detail, tc.access)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatalf("want bounded access %s", tc.access)
			}
		})
	}
}

func TestMetadataReplayPreservesStopwords(t *testing.T) {
	t.Parallel()
	db, cleanup := newTestDB(t)
	defer cleanup()
	file := IndexedFile{SourcePath: "/perennial", Source: "session", Hash: "one", Messages: []IndexedMessage{{UUID: "retained", Text: "quartzmarker", ContentType: "text", Role: "assistant"}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`CREATE TEMP TRIGGER observe_stopword_refresh BEFORE DELETE ON dynamic_stopwords BEGIN SELECT RAISE(ABORT,'prose vocabulary refresh'); END`); err != nil {
		t.Fatal(err)
	}
	file.Hash = "two"
	if err := db.SyncFilesContext(context.Background(), []IndexedFile{file}); err != nil {
		t.Fatalf("unchanged prose replay: %v", err)
	}
	file.Messages = append(file.Messages, IndexedMessage{UUID: "tool", Ordinal: 1, Text: "toolmarker", ContentType: "tool", Role: "assistant"})
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("tool-only addition: %v", err)
	}
	file.Messages = append(file.Messages, IndexedMessage{UUID: "new-prose", Ordinal: 2, Text: "cobaltmarker", ContentType: "text", Role: "assistant"})
	if err := db.SyncFiles([]IndexedFile{file}); err == nil || !strings.Contains(err.Error(), "prose vocabulary refresh") {
		t.Fatalf("new prose must refresh atomically: %v", err)
	}
	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE uuid='new-prose'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("refresh failure rollback: count=%d err=%v", count, err)
	}
}
