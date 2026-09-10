package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The tag names come from the independent structural review of Codex 0.153.4;
// all payloads and searchable markers in this fixture are synthetic.
func TestCodexInjectedWrappersV1E2E(t *testing.T) {
	testEnv(t)
	sourceRoot := t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "rollout.jsonl")
	fixture, err := os.ReadFile(filepath.Join(fixturesDir(), "codex-wrappers-v1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	preset, err := os.ReadFile(filepath.Join("..", "..", "inputs", "codex.inputs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := replaceManifestRoots(t, "inputs/codex.inputs.toml", string(preset), sourceRoot)
	writeFile(t, filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs", "codex.inputs.toml"), manifest)

	for _, tc := range []struct {
		marker string
		role   string
		count  int
	}{
		{"codexpluginboilerquartz", "user", 0},
		{"codexenvironmentboilerquartz", "user", 0},
		{"codexheartbeatboilerquartz", "user", 0},
		{"codexabortboilerquartz", "user", 0},
		{"codexmixedboilerquartz", "user", 0},
		{"codexblockboilerquartz", "user", 0},
		{"codextaskkeepquartz", "user", 1},
		{"codexplainkeepquartz", "user", 1},
		{"codexquotedkeepquartz", "user", 1},
		{"codexassistantkeepquartz", "assistant", 1},
		{"codexmixedkeepquartz", "user", 1},
		{"codexblockkeepquartz", "user", 1},
	} {
		t.Run(tc.marker, func(t *testing.T) {
			out, stderr, err := runCmd("search", tc.marker, "--all-projects", "--source-path", sourcePath, "--role", tc.role, "--content-type", "text", "--lexical-only", "--json")
			if err != nil {
				t.Fatalf("search: %v stderr=%s", err, stderr)
			}
			var rows []map[string]any
			if err := json.Unmarshal([]byte(out), &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != tc.count {
				t.Fatalf("want %d recall matches, got %s", tc.count, out)
			}
		})
	}
}
