package input_config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// TestClaudePresetDiscovery verifies that the claude.inputs.toml discover config
// finds the same files as the legacy WalkSessionDirs behavior.
func TestClaudePresetDiscovery(t *testing.T) {
	// Set up a mock ~/.claude/projects structure
	root := t.TempDir()
	proj1 := filepath.Join(root, "project-a")
	proj2 := filepath.Join(root, "project-b")
	sub := filepath.Join(proj1, "subagents", "sub")

	for _, d := range []string{proj1, proj2, sub} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeFile := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(`{"type":"user"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := filepath.Join(proj1, "session1.jsonl")
	b := filepath.Join(proj2, "session2.jsonl")
	subFile := filepath.Join(sub, "sub.jsonl")
	writeFile(a)
	writeFile(b)
	writeFile(subFile)
	// non-jsonl file — should not be discovered
	writeFile(filepath.Join(proj1, "notes.txt") + ".dummy")
	_ = os.WriteFile(filepath.Join(proj1, "notes.txt"), []byte("text"), 0o644)

	cfg := DiscoverConfig{
		Roots:   []string{root},
		Include: []string{"**/*.jsonl"},
		Exclude: []string{"**/subagents/**"},
	}

	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}

	sort.Strings(files)
	want := []string{a, b}
	sort.Strings(want)

	if len(files) != len(want) {
		t.Fatalf("got %v, want %v", files, want)
	}
	for i := range files {
		if files[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, files[i], want[i])
		}
	}

	// Verify the subagent file was excluded
	for _, f := range files {
		if filepath.Dir(f) == sub {
			t.Errorf("subagent file %q should have been excluded", f)
		}
	}
}

// TestPiPresetParses verifies the Pi preset TOML parses without error.
func TestPiPresetParses(t *testing.T) {
	data, err := os.ReadFile("../../tests/fixtures/pi.inputs.toml")
	if err != nil {
		t.Fatalf("read pi preset: %v", err)
	}
	var f InputFile
	if err := toml.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal pi preset: %v", err)
	}
	if len(f.Inputs) == 0 {
		t.Fatal("no inputs in pi preset")
	}
	def := f.Inputs[0]
	if def.ID != "pi" {
		t.Errorf("id = %q, want pi", def.ID)
	}
}
