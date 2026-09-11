package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/models"
)

func TestV15EchoBackfillPreservesPerennialIdentity(t *testing.T) {
	path := createFixtureDatabase(t, "v14.sql")
	db, err := openWithoutSetup(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct{ path, uuid, text, kind string }{
		{"live.jsonl", "result", "orchard result", "tool"},
		{"expired.jsonl", "expired", "orchard result", "tool"},
		{"prose.jsonl", "prose", "orchard decision", "text"},
	} {
		exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,content_type,extraction_version) VALUES('session',?,0,'user',?,?,?,?)`, row.path, row.text, row.uuid, row.kind, CurrentExtractionVersion)
	}
	var originalID int
	if err := db.db.QueryRow(`SELECT id FROM search_items WHERE uuid='result'`).Scan(&originalID); err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(context.Background(), db.db)
	if err != nil || diag != nil {
		t.Fatalf("inspect: %v %v", err, diag)
	}
	if err := db.ApplyMigrationPlan(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	paths, err := db.StalePaths(CurrentExtractionVersion)
	if err != nil || len(paths) != 3 {
		t.Fatalf("backlog=%v err=%v", paths, err)
	}
	// An unproven expired output remains visible; no adjacency/output guessing.
	got, err := db.Search("orchard", models.SearchOptions{AllProjects: true})
	if err != nil || len(got) != 3 {
		t.Fatalf("migration lost rows: %v %v", got, err)
	}
	file := IndexedFile{SourcePath: "live.jsonl", Source: "session", Hash: "same", Messages: []IndexedMessage{{Ordinal: 0, Role: "user", Text: "orchard result", UUID: "result", ContentType: "tool", ExtractionVersion: CurrentExtractionVersion, SearchEcho: true}}}
	for i := 0; i < 2; i++ {
		if err := db.SyncFiles([]IndexedFile{file}); err != nil {
			t.Fatal(err)
		}
	}
	var id, echo, version int
	var text string
	if err := db.db.QueryRow(`SELECT id,text,search_echo,extraction_version FROM search_items WHERE uuid='result'`).Scan(&id, &text, &echo, &version); err != nil {
		t.Fatal(err)
	}
	if id != originalID || text != "orchard result" || echo != 1 || version != CurrentExtractionVersion {
		t.Fatalf("identity/provenance changed: %d %q %d %d", id, text, echo, version)
	}
	paths, err = db.StalePaths(CurrentExtractionVersion)
	if err != nil || !reflect.DeepEqual(paths, []string{"expired.jsonl", "prose.jsonl"}) {
		t.Fatalf("backfill failed to converge: %v %v", paths, err)
	}
	got, err = db.Search("orchard", models.SearchOptions{AllProjects: true})
	if err != nil || len(got) != 2 {
		t.Fatalf("unfiltered echo leak: %v %v", got, err)
	}
	got, err = db.Search("orchard", models.SearchOptions{AllProjects: true, ContentType: "tool"})
	if err != nil || len(got) != 2 {
		t.Fatalf("explicit tool lost data: %v %v", got, err)
	}
	// Partial future parses must not erase prior positive pairing evidence.
	file.Messages[0].SearchEcho = false
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT search_echo FROM search_items WHERE uuid='result'`).Scan(&echo); err != nil || echo != 1 {
		t.Fatalf("lost proven pairing: %d %v", echo, err)
	}
	var unknown sql.NullBool
	if err := db.db.QueryRow(`SELECT search_echo FROM search_items WHERE uuid='expired'`).Scan(&unknown); err != nil || unknown.Valid {
		t.Fatalf("expired evidence was guessed: %v %v", unknown, err)
	}
}

func TestPendingSearchEchoPathsRequeuesZeroValuedDirectCalls(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files := []IndexedFile{
		{Source: "session", SourcePath: "codex.jsonl", Hash: "h1", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "exec_command cmd=backscroll search --text orchard", ContentType: "tool", SearchEcho: true},
			{Ordinal: 1, Role: "tool", Text: "result_0_snippet=orchard", ContentType: "tool", SearchEcho: true},
		}},
		{Source: "session", SourcePath: "opencode.db", Hash: "h2", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "bash command=backscroll search --text orchard", ContentType: "tool", SearchEcho: true},
			{Ordinal: 1, Role: "assistant", Text: "result_0_snippet=orchard", ContentType: "tool", SearchEcho: true},
		}},
		{Source: "session", SourcePath: "rg.jsonl", Hash: "h3", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "exec_command cmd=rg orchard", ContentType: "tool"},
		}},
		{Source: "session", SourcePath: "unknown.jsonl", Hash: "h4", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "result_0_snippet=orchard", ContentType: "tool"},
		}},
	}
	files[3].Messages[0].SearchEcho = false
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET search_echo=0 WHERE source_path IN ('codex.jsonl','opencode.db','rg.jsonl')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET search_echo=NULL WHERE source_path='unknown.jsonl'`); err != nil {
		t.Fatal(err)
	}
	got, err := db.PendingSearchEchoPaths()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"codex.jsonl", "opencode.db", "unknown.jsonl"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pending=%v want %v", got, want)
	}
}

func TestEchoProvenanceDoesNotAffectProse(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "p", Hash: "h", Messages: []IndexedMessage{{Text: "orchard", UUID: "u", Role: "assistant", ContentType: "text", SearchEcho: true}}}}); err != nil {
		t.Fatal(err)
	}
	got, err := db.Search("orchard", models.SearchOptions{AllProjects: true})
	if err != nil || len(got) != 1 {
		t.Fatalf("prose filtered: %v %v", got, err)
	}
}
