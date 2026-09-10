package main

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestEchoProvenanceBackfillDrainsAtMost200UnchangedFiles(t *testing.T) {
	e := newQueryEchoE2E(t)
	oldTime := time.Now().Add(-24 * time.Hour)
	for i := 0; i < 201; i++ {
		id := fmt.Sprintf("cap-%03d", i)
		path := e.writeRecord(id+".jsonl", id, "cap", 0, "ordinary archived prose", false)
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}
	e.run("status", "--json")
	db, err := sql.Open("sqlite", e.database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Same pending evidence state as v15 migration, with current extraction epoch
	// and unchanged cached hashes/size/mtime. Count actual rows, not progress logs.
	if _, err := db.Exec(`UPDATE search_items SET search_echo=NULL`); err != nil {
		t.Fatal(err)
	}
	unknown := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT count(DISTINCT source_path) FROM search_items WHERE search_echo IS NULL`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	e.run("status", "--json")
	if got := unknown(); got != 1 {
		t.Fatalf("first replay left %d unknown files, want 1 (200-file bound)", got)
	}
	e.run("status", "--json")
	if got := unknown(); got != 0 {
		t.Fatalf("second replay left %d unknown files, want converged", got)
	}
	// Older perennial extraction epochs remain unchanged after enrichment. They
	// must not repeatedly spend the echo budget and starve the final pending file.
	if _, err := db.Exec(`UPDATE search_items SET search_echo=NULL, extraction_version=1`); err != nil {
		t.Fatal(err)
	}
	e.run("status", "--json")
	if got := unknown(); got != 1 {
		t.Fatalf("legacy epoch replay left %d unknown files, want 1", got)
	}
	e.run("status", "--json")
	if got := unknown(); got != 0 {
		t.Fatalf("legacy epoch rows starved echo replay: %d still pending", got)
	}
}
