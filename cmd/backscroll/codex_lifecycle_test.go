package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexPresetIncrementalAndPerennialRecall(t *testing.T) {
	testEnv(t)
	home := os.Getenv("HOME")
	root := filepath.Join(home, ".codex", "sessions", "2026", "09")
	archived := filepath.Join(home, ".codex", "archived_sessions")
	inputs := filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs")
	for _, dir := range []string{root, archived, inputs} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	preset, err := os.ReadFile(filepath.Join("..", "..", "inputs", "codex.inputs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	// Opt in before initial ingestion; this is a fresh isolated database.
	preset = []byte(strings.ReplaceAll(string(preset), "index_reasoning = false", "index_reasoning = true"))
	if err := os.WriteFile(filepath.Join(inputs, "codex.inputs.toml"), preset, 0600); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join(fixturesDir(), "codex-rollout-v1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(root, "active.jsonl")
	if err := os.WriteFile(activePath, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	archiveRecord := `{"type":"response_item","timestamp":"2026-09-01T12:00:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"codexarchivequartz"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(archived, "archived.jsonl"), []byte(archiveRecord), 0600); err != nil {
		t.Fatal(err)
	}
	recall := func(marker string) {
		t.Helper()
		out, stderr, err := runCmd("search", marker, "--all-projects", "--lexical-only", "--json")
		if err != nil {
			t.Fatalf("recall %s: %v %s", marker, err, stderr)
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s: want one row, got %s", marker, out)
		}
	}
	recall("codexuserquartz")
	recall("codexarchivequartz")
	recall("codexreasonquartz")
	recall("codexuserquartz") // unchanged input: no duplicate rows
	appendRecord := strings.ReplaceAll(archiveRecord, "codexarchivequartz", "codexappendquartz")
	if err := os.WriteFile(activePath, append(fixture, []byte(appendRecord)...), 0600); err != nil {
		t.Fatal(err)
	}
	recall("codexappendquartz")
	recall("codexuserquartz") // changed input: no duplicated earlier records
	// Removing only our temporary source exercises the perennial store boundary.
	if err := os.Remove(activePath); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runCmd("rebuild"); err != nil {
		t.Fatalf("rebuild: %v %s", err, stderr)
	}
	recall("codexappendquartz")
	recall("codexuserquartz")
	recall("codexarchivequartz")
}
