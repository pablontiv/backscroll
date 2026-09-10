package input_config

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestCodexPresetDiscovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	raw, err := os.ReadFile("../../inputs/codex.inputs.toml")
	if err != nil {
		t.Fatal(err)
	}
	var preset InputFile
	if err := toml.Unmarshal(raw, &preset); err != nil {
		t.Fatal(err)
	}
	if preset.Version != 1 || len(preset.Inputs) != 1 {
		t.Fatalf("preset: %+v", preset)
	}
	def := preset.Inputs[0]
	if def.ID != "codex" || def.Source != "session" || !def.Active || def.Decode.Format != "codex" || def.Decode.IndexReasoning || def.Discover.FollowSymlinks {
		t.Fatalf("definition: %+v", def)
	}
	want := []string{filepath.Join(home, ".codex", "sessions", "2026", "09", "rollout.jsonl"), filepath.Join(home, ".codex", "archived_sessions", "archived.jsonl")}
	for _, path := range want {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "sessions", "notes.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverFiles(def.Discover)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roots: got %v want %v", got, want)
	}
}
