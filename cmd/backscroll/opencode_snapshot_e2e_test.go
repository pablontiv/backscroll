package main

import (
	"bytes"
	"database/sql"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual dev binary's manifest discovery, startup sync, and index
// writes with a live WAL writer, not a fake reader or synthetic watermark.
func TestOpenCodeSnapshotStartupSmoke(t *testing.T) {
	t.Parallel()
	e := newQueryEchoE2E(t)
	root := e.addReaderManifest("opencode", "opencode")
	source := openOpenCodeFixture(t, filepath.Join(root, "opencode.db"))
	defer source.Close()
	if _, err := source.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}
	insertOpenCodeToolPart(t, source, "native", 1767225600000, map[string]any{"type": "text", "text": "original orchard"})
	e.environment = append(e.environment, "BACKSCROLL_STARTUP_DIAGNOSTICS=1")
	startup := func() string {
		t.Helper()
		command := exec.Command(e.binary, "status")
		command.Dir = e.root
		command.Env = e.environment
		var stderr bytes.Buffer
		command.Stderr = &stderr
		if out, err := command.Output(); err != nil {
			t.Fatalf("startup: %v\n%s\n%s", err, out, stderr.String())
		}
		return stderr.String()
	}
	startup()
	index, err := sql.Open("sqlite", "file:"+e.database+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	assertContent := func(want string) {
		t.Helper()
		var got string
		if err := index.QueryRow("SELECT text FROM search_items WHERE source_path=?", filepath.Join(root, "opencode.db")).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("indexed text = %q, want %q", got, want)
		}
	}
	assertContent("original orchard")
	var originalHash string
	if err := index.QueryRow("SELECT hash FROM indexed_files WHERE path=?", filepath.Join(root, "opencode.db")).Scan(&originalHash); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`UPDATE part SET data='{"type":"text","text":"changed orchard"}' WHERE id='p-native'`); err != nil {
		t.Fatal(err)
	}
	startup()
	assertContent("original orchard")
	var changedHash string
	if err := index.QueryRow("SELECT hash FROM indexed_files WHERE path=?", filepath.Join(root, "opencode.db")).Scan(&changedHash); err != nil || changedHash == originalHash {
		t.Fatalf("part-only edit did not advance native snapshot hash: %q -> %q (%v)", originalHash, changedHash, err)
	}
	diagnostics := startup()
	if !strings.Contains(diagnostics, "Parsing:         0s") {
		t.Fatalf("unchanged snapshot was parsed:\n%s", diagnostics)
	}
	assertContent("original orchard")
	t.Log("actual startup: committed part-only WAL edit advances snapshot token while retaining native payload; unchanged run skips parsing")
}
