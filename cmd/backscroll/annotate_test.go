package main

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

func TestAnnotateCommand(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Setup: create DB with a session
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	files := []storage.IndexedFile{{
		SourcePath: "/p/s.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "proj",
		Messages: []storage.IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "test", UUID: "u1",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Test: annotate via CLI with hermetic startup inputs/config
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	var stdout, stderr bytes.Buffer
	err = run(&stdout, &stderr, []string{"annotate", "--uuid", "u1", "--kind", "correction", "--label", "fixable"})
	if err != nil {
		t.Fatalf("annotate failed: %v stderr: %s", err, stderr.String())
	}

	// Verify output
	if !bytes.Contains(stdout.Bytes(), []byte("u1")) || !bytes.Contains(stdout.Bytes(), []byte("fixable")) {
		t.Errorf("expected output to contain u1 and fixable, got: %s", stdout.String())
	}
}

func TestAnnotateCommandMissingMessage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Setup: empty DB
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Test: annotate non-existent message with hermetic startup inputs/config
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	var stdout, stderr bytes.Buffer
	err = run(&stdout, &stderr, []string{"annotate", "--uuid", "nonexistent", "--kind", "correction", "--label", "label"})
	if err == nil {
		t.Fatal("expected error for missing message")
	}
}

func TestAnnotateExactIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, u := range []string{"opaque one", "opaque two"} {
		if _, err := raw.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,content_type,uuid) VALUES ('session','/duplicate',7,'user','hello','text',?)`, u); err != nil {
			t.Fatal(err)
		}
	}
	setIndexPolicyEnv(t, path, t.TempDir())
	invoke := func(selectors []string) error {
		var out, stderr bytes.Buffer
		return run(&out, &stderr, append(append([]string{"annotate"}, selectors...), "--kind", "correction", "--label", "reviewed"))
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--path", "/duplicate", "--ordinal", "7"}, "ambiguous"},
		{[]string{"--uuid", "absent"}, "not found"},
		{[]string{"--path", "/absent", "--ordinal", "7"}, "not found"},
		{[]string{"--uuid", "opaque one", "--path", "/wrong"}, "different messages"},
		{[]string{"--uuid", "opaque one", "--ordinal", "8"}, "different messages"},
		{[]string{"--uuid", "opaque one", "--path", "/wrong", "--ordinal", "8"}, "different messages"},
		{nil, "must provide"},
	} {
		if err := invoke(c.args); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%v: %v", c.args, err)
		}
		var count int
		if err := raw.QueryRow(`SELECT COUNT(*) FROM annotations`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rejection wrote %d annotations", count)
		}
	}
	if err := invoke([]string{"--uuid", "opaque two"}); err != nil {
		t.Fatal(err)
	}
	var u, p string
	var n int
	if err := raw.QueryRow(`SELECT item_uuid,source_path,ordinal FROM annotations`).Scan(&u, &p, &n); err != nil {
		t.Fatal(err)
	}
	if u != "opaque two" || p != "/duplicate" || n != 7 {
		t.Fatalf("noncanonical: %s %s %d", u, p, n)
	}
}

func TestAnnotateExplicitNegativeOrdinal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "negative.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SyncFiles([]storage.IndexedFile{{SourcePath: "/canonical", Source: "session", Hash: "fixture", Messages: []storage.IndexedMessage{{UUID: "opaque", Ordinal: 7, Role: "user", Text: "fixture", ContentType: "text"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAnnotation("opaque", "", -1, "correction", "original"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	snapshot := func() string {
		var state string
		if err := raw.QueryRow(`SELECT json_group_array(json_array(id,item_uuid,source_path,ordinal,kind,label,source,created_at)) FROM annotations`).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	setIndexPolicyEnv(t, path, t.TempDir())
	before := snapshot()
	for _, n := range []string{"-1", "-2"} {
		var out, stderr bytes.Buffer
		err := run(&out, &stderr, []string{"annotate", "--uuid", "opaque", "--ordinal", n, "--kind", "correction", "--label", "rejected"})
		if err == nil {
			t.Errorf("explicit %s succeeded", n)
		}
		if out.Len() != 0 || snapshot() != before {
			t.Errorf("explicit %s changed annotations/output", n)
		}
	}
	var out, stderr bytes.Buffer
	if err := run(&out, &stderr, []string{"annotate", "--uuid", "opaque", "--kind", "correction", "--label", "accepted"}); err != nil {
		t.Fatal(err)
	}
	var label string
	if err := raw.QueryRow(`SELECT label FROM annotations`).Scan(&label); err != nil || label != "accepted" {
		t.Fatalf("omitted ordinal: %s %v", label, err)
	}
}
