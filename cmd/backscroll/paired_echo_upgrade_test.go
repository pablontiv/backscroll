package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPairedEchoV14UpgradeReparsesUnchangedFiles(t *testing.T) {
	e := newQueryEchoE2E(t)
	e.writeCoreProse()
	baseline := queryEchoShape(e.searchJSON(queryEchoText, "", 20))
	budgets := []int{150, 180, 220, 300}
	baselineBudgets := e.budgetReachability(queryEchoText, budgets)
	output := e.run("search", "--text", queryEchoText, "--all-projects", "--robot", "--fields", "minimal", "--max-tokens", "0", "--limit", "2")
	path := e.writeRecord("upgrade.jsonl", "upgrade-use", "active", 4, queryEchoToolBlock("upgrade-call", "backscroll search --text '"+queryEchoText+"' --robot"), false)
	e.appendToolResult(path, "upgrade-result", "upgrade-call", output)
	oldTime := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	e.run("status", "--json")
	// Materialize the retained v14 schema with the actual CLI's original records,
	// IDs, text and hash/size/mtime cache, omitting only the new provenance column.
	// The native pre-correction binary upgrade was separately observed in the PoC.
	oldPath := filepath.Join(e.root, "v14.db")
	db, err := sql.Open("sqlite", oldPath)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "compat", "testdata", "release-schemas", "v14.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ATTACH DATABASE ? AS seed`, e.database); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO search_items(id,source,source_path,ordinal,role,text,timestamp,uuid,project,content_type,extraction_version,was_interrupted)
 SELECT id,source,source_path,ordinal,role,text,timestamp,uuid,project,content_type,extraction_version,was_interrupted FROM seed.search_items;
 INSERT INTO indexed_files SELECT * FROM seed.indexed_files;`); err != nil {
		t.Fatal(err)
	}
	// Capture stable identities before any migration or replay.
	identity := func() string {
		t.Helper()
		var value string
		if err := db.QueryRow(`SELECT group_concat(id || ':' || uuid || ':' || text, '|') FROM (SELECT * FROM search_items ORDER BY id)`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := identity()
	if _, err := db.Exec(`DETACH DATABASE seed`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	e.database = oldPath
	e.environment = queryEchoOverrideEnv(e.environment, map[string]string{"BACKSCROLL_CONFIG_DIR": e.config, "BACKSCROLL_DATABASE_PATH": oldPath, "BACKSCROLL_SESSION_DIRS": ""})
	if got := queryEchoShape(e.searchJSON(queryEchoText, "", 20)); !reflect.DeepEqual(got, baseline) {
		t.Fatalf("upgrade did not restore baseline: %v want %v", got, baseline)
	}
	if got := e.budgetReachability(queryEchoText, budgets); !reflect.DeepEqual(got, baselineBudgets) {
		t.Errorf("budget baseline changed: %v want %v", got, baselineBudgets)
	}
	db, err = sql.Open("sqlite", "file:"+oldPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := identity(); got != before {
		t.Error("migration/reparse replaced perennial ID/UUID/text")
	}
	var unknown, marked int
	if err := db.QueryRow(`SELECT count(*) FROM search_items WHERE search_echo IS NULL`).Scan(&unknown); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM search_items WHERE search_echo=1`).Scan(&marked); err != nil {
		t.Fatal(err)
	}
	if unknown != 0 || marked != 2 {
		t.Fatalf("provenance unknown=%d marked=%d want 0/2", unknown, marked)
	}
	if got := e.searchJSON(queryEchoText, "tool", 20); len(got) != 2 {
		t.Errorf("explicit tool pair lost: %v", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	e.run("rebuild")
	if got := queryEchoShape(e.searchJSON(queryEchoText, "", 20)); !reflect.DeepEqual(got, baseline) {
		t.Errorf("rebuild/expiry lost proven pairing: %v", got)
	}
	e.assertIntegrity()
}

func TestPairedEchoControlsKeepIdenticalUnrelatedOutputs(t *testing.T) {
	e := newQueryEchoE2E(t)
	e.writeCoreProse()
	output := e.run("search", "--text", queryEchoText, "--all-projects", "--robot", "--fields", "minimal", "--max-tokens", "0", "--limit", "2")
	e.writeControls()
	e.run("status", "--json")
	e.assertControlBoundaries()
	// All controls return the exact same bytes as the filtered direct search.
	for i, command := range []string{"rg '" + queryEchoText + "'", "/opt/bin/backscroll search '" + queryEchoText + "'", "env backscroll search '" + queryEchoText + "'", "bash -lc \"backscroll search '" + queryEchoText + "'\""} {
		id := []string{"rg", "absolute", "env", "shell"}[i]
		path := e.writeRecord(id+"-paired.jsonl", id+"-use", "active", 12+i, queryEchoToolBlock(id, command), false)
		e.appendToolResult(path, id+"-result", id, output)
	}
	orphan := e.writeRecord("orphan.jsonl", "same-session-prose", "active", 18, "A genuine same-session explanation.", false)
	e.appendToolResult(orphan, "orphan-result", "unmatched", output)
	results := e.searchJSON(queryEchoText, "", 30)
	counts := map[string]int{}
	for _, r := range results {
		counts[filepath.Base(r.FilePath)]++
	}
	for _, name := range []string{"rg-paired.jsonl", "absolute-paired.jsonl", "env-paired.jsonl", "shell-paired.jsonl"} {
		if counts[name] != 2 {
			t.Errorf("control %s got %d rows, want use+result", name, counts[name])
		}
	}
	if counts["orphan.jsonl"] != 1 {
		t.Error("orphan result was guessed from its output shape")
	}
	if got := e.searchJSON("genuine same-session explanation", "", 10); len(got) != 1 {
		t.Error("same-session prose excluded")
	}
}
