package input_config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestOMPPresetDiscovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	raw, err := os.ReadFile("../../inputs/omp.inputs.toml")
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
	if def.ID != "omp" || def.Source != "session" || !def.Active || def.Decode.Format != "omp" || def.Decode.IndexReasoning || def.Discover.FollowSymlinks {
		t.Fatalf("definition: %+v", def)
	}
	if !reflect.DeepEqual(def.Discover.Roots, []string{"~/.omp/agent/sessions"}) || !reflect.DeepEqual(def.Discover.Include, []string{"*/*.jsonl"}) {
		t.Fatalf("canonical discovery: %+v", def.Discover)
	}
	if got, err := DiscoverFiles(def.Discover); err != nil || len(got) != 0 {
		t.Fatalf("absent root: %v %v", got, err)
	}
	root := filepath.Join(home, ".omp", "agent", "sessions")
	want := filepath.Join(root, "project", "native.jsonl")
	for _, path := range []string{want, filepath.Join(root, "project", "native", "child.jsonl"), filepath.Join(root, "root.jsonl"), filepath.Join(root, "project", "notes.txt"), filepath.Join(home, ".pi", "agent", "sessions", "project", "pi.jsonl")} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(want, filepath.Join(root, "project", "link.jsonl")); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverFiles(def.Discover)
	if err != nil || !reflect.DeepEqual(got, []string{want}) {
		t.Fatalf("direct native files only: %v %v", got, err)
	}
}
