package main

import (
	"bytes"
	"path/filepath"
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

	// Test: annotate via CLI
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
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

	// Test: annotate non-existent message
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	var stdout, stderr bytes.Buffer
	err = run(&stdout, &stderr, []string{"annotate", "--uuid", "nonexistent", "--kind", "correction", "--label", "label"})
	if err == nil {
		t.Fatal("expected error for missing message")
	}
}
