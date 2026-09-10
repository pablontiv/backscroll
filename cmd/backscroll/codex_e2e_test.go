package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Versioned E2E oracle, derived from the structural probe recorded in
// docs/research/codex-input-evidence.md, not from the production parser.
func TestCodexRolloutV1E2E(t *testing.T) {
	testEnv(t)
	root := t.TempDir()
	source := filepath.Join(root, "2026", "09", "rollout.jsonl")
	if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join(fixturesDir(), "codex-rollout-v1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	inputs := filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs")
	if err := os.MkdirAll(inputs, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`version = 1
[[inputs]]
id = "codex"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]
[inputs.decode]
format = "codex"
`, root)
	if err := os.WriteFile(filepath.Join(inputs, "codex.inputs.toml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ marker, role, kind string }{
		{"codexuserquartz", "user", "text"},
		{"codexanswerquartz", "assistant", "text"},
		{"codextoolquartz", "assistant", "tool"},
		{"codexoutputquartz", "tool", "tool"},
		{"codexpatchquartz", "assistant", "tool"},
		{"codexblockquartz", "tool", "tool"},
		{"codexafterquartz", "assistant", "text"},
	} {
		t.Run(tc.marker, func(t *testing.T) {
			out, stderr, err := runCmd("search", tc.marker, "--project", "codex-fixture-project", "--source", "session", "--source-path", source, "--role", tc.role, "--content-type", tc.kind, "--after", "2026-09-01", "--before", "2026-09-02", "--lexical-only", "--json")
			if err != nil {
				t.Fatalf("search: %v stderr=%s", err, stderr)
			}
			var rows []map[string]any
			if err := json.Unmarshal([]byte(out), &rows); err != nil {
				t.Fatalf("JSON %s: %v", out, err)
			}
			if len(rows) != 1 {
				t.Fatalf("want exactly one %s recall, got %s; stderr=%s", tc.marker, out, stderr)
			}
		})
	}
	for _, marker := range []string{
		"codexignoredinstructions", "codexignoredimage", "codexignoredencrypted",
		"codexignoredcompact", "codexignoredfuture", "codexignoredmalformed", "codexreasonquartz",
	} {
		out, stderr, err := runCmd("search", marker, "--all-projects", "--lexical-only", "--json")
		if err != nil {
			t.Fatalf("negative search: %v %s", err, stderr)
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("excluded %s leaked: %s", marker, out)
		}
	}
}
