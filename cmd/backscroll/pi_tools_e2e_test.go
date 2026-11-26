package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
)

const piToolsFixture = `{"type":"session","id":"pi-tools-session","cwd":"/workspace/pi-tools-project"}
{"type":"message","id":"user","message":{"role":"user","content":"pi native tools fixture"}}
{"type":"message","id":"early-result","message":{"role":"toolResult","toolCallId":"failure","isError":true,"content":[{"type":"text","text":"failure output"}]}}
{"type":"message","id":"calls","message":{"role":"assistant","content":[{"type":"toolCall","id":"success","name":"bash","arguments":{"command":"echo success"}},{"type":"toolCall","id":"failure","name":"bash","arguments":{"command":"false"}},{"type":"toolCall","id":"missing","name":"bash","arguments":{"command":"pwd"}},{"type":"toolCall","id":"unknown","name":"read","arguments":{"path":"fixture.txt"}},{"type":"toolCall","id":"empty","name":"bash","arguments":{"command":"true"}}]}}
{"type":"message","id":"unknown-result","message":{"role":"toolResult","toolCallId":"unknown","content":[{"type":"text","text":"not an error signal"}]}}
{"type":"message","id":"success-result","message":{"role":"toolResult","toolCallId":"success","isError":false,"content":[{"type":"text","text":"success output"}]}}
{"type":"message","id":"empty-result","message":{"role":"toolResult","toolCallId":"empty","isError":false}}
`

func TestPiNativeToolsPatternsAndEpochReplay(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()
	root := t.TempDir()
	source := filepath.Join(root, "session.jsonl")
	writeFile(t, source, piToolsFixture)
	manifest := "version = 1\n[[inputs]]\nid = \"pi-tools\"\nsource = \"session\"\nactive = true\n[inputs.discover]\nroots = [\"" + root + "\"]\ninclude = [\"*.jsonl\"]\n[inputs.decode]\nformat = \"pi\"\n"
	writeFile(t, filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs", "pi-tools.inputs.toml"), manifest)
	checkPatterns := func(signalled, unknown int) {
		t.Helper()
		out, stderr, err := runCmd("patterns", "--kind", "commands", "--all-projects", "--json")
		if err != nil {
			t.Fatalf("commands: %v %s", err, stderr)
		}
		var commands struct {
			Patterns []struct {
				Head  string `json:"command_head"`
				Count int    `json:"count"`
			} `json:"patterns"`
		}
		if err := json.Unmarshal([]byte(out), &commands); err != nil {
			t.Fatal(err)
		}
		heads := make(map[string]int)
		for _, pattern := range commands.Patterns {
			heads[pattern.Head] = pattern.Count
		}
		for _, head := range []string{"echo", "false", "pwd", "true", ""} {
			if heads[head] != 1 {
				t.Fatalf("command %q count in %s", head, out)
			}
		}
		out, stderr, err = runCmd("patterns", "--kind", "failures", "--all-projects", "--json")
		if err != nil {
			t.Fatalf("failures: %v %s", err, stderr)
		}
		var census struct {
			Total     int `json:"total_events"`
			Signalled int `json:"signalled_events"`
			Unknown   int `json:"unsignalled_events"`
			Patterns  []struct {
				Tool  string `json:"tool_name"`
				Count int    `json:"count"`
			} `json:"patterns"`
		}
		if err := json.Unmarshal([]byte(out), &census); err != nil {
			t.Fatal(err)
		}
		if census.Total != 5 || census.Signalled != signalled || census.Unknown != unknown || len(census.Patterns) != 1 || census.Patterns[0].Tool != "bash" || census.Patterns[0].Count != 1 {
			t.Fatalf("failure census: %s", out)
		}
	}
	checkPatterns(3, 2)
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for id, signal := range map[string]*int{"success": new(0), "failure": new(1), "missing": nil, "unknown": nil, "empty": new(0)} {
		var got *int
		if err := db.DB().QueryRow(`SELECT is_error FROM tool_events WHERE message_uuid=?`, "pi:pi-tools-session:call:"+id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (signal == nil) || got != nil && *got != *signal {
			t.Fatalf("%s signal=%v want %v", id, got, signal)
		}
	}
	// Simulate the persisted epoch-1 snapshot: available bytes and metadata stay
	// unchanged, while native metadata has not yet been captured.
	def := input_config.InputDefinition{Source: "session", Decode: input_config.DecodeConfig{Format: "pi"}}
	old := readers.IngestionFingerprint(&piEpochOneReader{}, def)
	if _, err := db.DB().Exec(`DELETE FROM tool_events; UPDATE search_items SET uuid=NULL; UPDATE indexed_files SET ingestion_fingerprint=? WHERE path=?`, old, source); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	checkPatterns(3, 2)
	checkPatterns(3, 2)
	// A later empty result supplies explicit success for the retained native call.
	writeFile(t, source, piToolsFixture+`{"type":"message","id":"late","message":{"role":"toolResult","toolCallId":"missing","isError":false,"content":""}}`+"\n")
	checkPatterns(4, 1)
	db, err = storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var events, calls int
	if err := db.DB().QueryRow(`SELECT COUNT(*),COUNT(DISTINCT message_uuid) FROM tool_events WHERE source_path=?`, source).Scan(&events, &calls); err != nil {
		t.Fatal(err)
	}
	if events != 5 || calls != 5 {
		t.Fatalf("events/identities=%d/%d", events, calls)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, source, strings.Replace(piToolsFixture, `"id":"user",`, "", 1))
	if _, stderr, err := runCmd("patterns", "--kind", "commands", "--all-projects", "--json"); err == nil || !strings.Contains(stderr+err.Error(), "without native identity") {
		t.Fatalf("incomplete identified replay: err=%v stderr=%s", err, stderr)
	}
	db, err = storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM tool_events WHERE source_path=?`, source).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 5 {
		t.Fatalf("failed replay replaced retained events: %d", events)
	}
}

type piEpochOneReader struct{ readers.PiReader }

func (*piEpochOneReader) IngestionEpoch() uint64 { return 1 }
