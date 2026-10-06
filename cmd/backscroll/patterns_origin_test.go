package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestPatternsCorrectionsOriginOutputFormats(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SyncFiles([]storage.IndexedFile{{SourcePath: "/p/origin.jsonl", Source: "session", Hash: "h", Project: "p", Messages: []storage.IndexedMessage{
		{Ordinal: 0, UUID: "human-u", Role: "user", Origin: models.OriginHuman, Text: "no, use the human request", ContentType: "text", ExtractionVersion: storage.CurrentExtractionVersion},
		{Ordinal: 1, UUID: "auto-u", Role: "user", Origin: models.OriginAutomation, Text: "no, injected automation", ContentType: "text", ExtractionVersion: storage.CurrentExtractionVersion},
	}}}); err != nil {
		t.Fatal(err)
	}
	for ordinal := 0; ordinal < 2; ordinal++ {
		if _, err := db.DB().Exec(`INSERT OR REPLACE INTO correction_signals(source_path, ordinal, detector, confidence, extraction_version) VALUES('/p/origin.jsonl', ?, 'test', 0.9, ?)`, ordinal, storage.CurrentExtractionVersion); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	text, _, err := runCmd("patterns", "--kind", "corrections", "--origin", "human", "--all-projects")
	if err != nil || !strings.Contains(text, "Origin: human") || strings.Contains(text, "auto-u") {
		t.Fatalf("text=%q err=%v", text, err)
	}
	robot, _, err := runCmd("patterns", "--kind", "corrections", "--origin", "human", "--all-projects", "--robot")
	if err != nil || !strings.Contains(robot, "result_0_origin=human") || strings.Contains(robot, "auto-u") {
		t.Fatalf("robot=%q err=%v", robot, err)
	}
	jsonOutput, _, err := runCmd("patterns", "--kind", "corrections", "--origin", "human", "--all-projects", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(jsonOutput), &decoded); err != nil {
		t.Fatalf("JSON=%q err=%v", jsonOutput, err)
	}
	patterns, ok := decoded["patterns"].([]any)
	if !ok || len(patterns) != 1 || patterns[0].(map[string]any)["Origin"] != "human" {
		t.Fatalf("decoded JSON=%#v", decoded)
	}

	unfiltered, _, err := runCmd("patterns", "--kind", "corrections", "--all-projects", "--robot")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(unfiltered, "_origin=") {
		t.Fatalf("unfiltered output changed shape: %q", unfiltered)
	}
}

func TestPatternsOriginValidationPrecedesDatabaseOpen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "must-not-exist.db")
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	t.Setenv("BACKSCROLL_SESSION_DIRS", filepath.Join(dir, "sessions"))

	for _, args := range [][]string{
		{"patterns", "--kind", "corrections", "--origin", "person"},
		{"patterns", "--kind", "commands", "--origin", "human"},
	} {
		if _, _, err := runCmd(args...); err == nil {
			t.Fatalf("args %v unexpectedly succeeded", args)
		}
		if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
			t.Fatalf("invalid args %v opened database: %v", args, err)
		}
	}
}
