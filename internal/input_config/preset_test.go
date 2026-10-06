package input_config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
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

func TestPiPresetDiscoveryAndSubagentOptIn(t *testing.T) {
	data, err := os.ReadFile("../../inputs/pi.inputs.toml")
	if err != nil {
		t.Fatalf("read shipped Pi preset: %v", err)
	}
	var preset InputFile
	if err := toml.Unmarshal(data, &preset); err != nil {
		t.Fatalf("unmarshal shipped Pi preset: %v", err)
	}
	if preset.Version != 1 || len(preset.Inputs) != 2 {
		t.Fatalf("preset = %+v, want version 1 with two inputs", preset)
	}

	byID := make(map[string]InputDefinition, len(preset.Inputs))
	for _, def := range preset.Inputs {
		if _, duplicate := byID[def.ID]; duplicate {
			t.Fatalf("duplicate shipped input ID %q", def.ID)
		}
		byID[def.ID] = def
	}
	ordinary, ok := byID["pi"]
	if !ok {
		t.Fatal("shipped preset has no pi input")
	}
	subagents, ok := byID["pi-subagents"]
	if !ok {
		t.Fatal("shipped preset has no pi-subagents input")
	}

	wantRoots := []string{"~/.pi/agent/sessions", "~/.pi/agent/sessions-archive"}
	if !ordinary.Active || subagents.Active {
		t.Fatalf("active defaults: pi=%t pi-subagents=%t, want true/false", ordinary.Active, subagents.Active)
	}
	for _, def := range []InputDefinition{ordinary, subagents} {
		if def.Source != "session" || def.Decode.Format != "pi" || def.Decode.IndexReasoning || def.Discover.FollowSymlinks {
			t.Fatalf("unsafe definition %q: %+v", def.ID, def)
		}
		if !reflect.DeepEqual(def.Discover.Roots, wantRoots) {
			t.Fatalf("roots for %q = %v, want %v", def.ID, def.Discover.Roots, wantRoots)
		}
	}
	const childGlob = "**/run-*/session.jsonl"
	if !reflect.DeepEqual(ordinary.Discover.Include, []string{"**/*.jsonl"}) ||
		!reflect.DeepEqual(ordinary.Discover.Exclude, []string{childGlob}) ||
		!reflect.DeepEqual(subagents.Discover.Include, []string{childGlob}) ||
		len(subagents.Discover.Exclude) != 0 {
		t.Fatalf("Pi discovery globs are not complementary: pi=%+v subagents=%+v", ordinary.Discover, subagents.Discover)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionsRoot := filepath.Join(home, ".pi", "agent", "sessions")
	ordinaryPath := mkfile(t, sessionsRoot, "project/main.jsonl")
	unrelatedNestedPath := mkfile(t, sessionsRoot, "project/other/deep.jsonl")
	childPath := mkfile(t, sessionsRoot, "project/parent/child/run-7/session.jsonl")
	// Deliberately do not create sessions-archive: a missing configured root is non-fatal.

	ordinaryFiles, err := DiscoverFiles(ordinary.Discover)
	if err != nil {
		t.Fatalf("discover ordinary Pi files: %v", err)
	}
	sort.Strings(ordinaryFiles)
	wantOrdinary := []string{ordinaryPath, unrelatedNestedPath}
	sort.Strings(wantOrdinary)
	if !reflect.DeepEqual(ordinaryFiles, wantOrdinary) {
		t.Fatalf("ordinary discovery = %v, want %v", ordinaryFiles, wantOrdinary)
	}

	subagentFiles, err := DiscoverFiles(subagents.Discover)
	if err != nil {
		t.Fatalf("discover Pi subagent files: %v", err)
	}
	if !reflect.DeepEqual(subagentFiles, []string{childPath}) {
		t.Fatalf("subagent discovery = %v, want [%s]", subagentFiles, childPath)
	}
	ordinarySet := make(map[string]struct{}, len(ordinaryFiles))
	for _, path := range ordinaryFiles {
		ordinarySet[path] = struct{}{}
	}
	for _, path := range subagentFiles {
		if _, overlap := ordinarySet[path]; overlap {
			t.Fatalf("ordinary and subagent discovery overlap at %s", path)
		}
	}

	activated := strings.Replace(string(data), "id = \"pi-subagents\"\nsource = \"session\"\nactive = false", "id = \"pi-subagents\"\nsource = \"session\"\nactive = true", 1)
	if activated == string(data) {
		t.Fatal("could not activate pi-subagents in shipped preset")
	}
	inputsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputsDir, "pi.inputs.toml"), []byte(activated), 0o644); err != nil {
		t.Fatal(err)
	}
	active, err := LoadInputsFromDir(inputsDir)
	if err != nil {
		t.Fatalf("load activated Pi preset: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("active inputs after opt-in = %d, want 2", len(active))
	}
	var optedIn InputDefinition
	for _, def := range active {
		if def.ID == "pi-subagents" {
			optedIn = def
		}
	}
	if optedIn.ID == "" {
		t.Fatal("explicit activation did not load pi-subagents")
	}
	optedInFiles, err := DiscoverFiles(optedIn.Discover)
	if err != nil {
		t.Fatalf("discover opted-in Pi subagents: %v", err)
	}
	if !reflect.DeepEqual(optedInFiles, []string{childPath}) {
		t.Fatalf("opted-in discovery = %v, want [%s]", optedInFiles, childPath)
	}
}
