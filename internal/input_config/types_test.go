package input_config

import (
	"os"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestUnmarshalClaudePreset(t *testing.T) {
	data, err := os.ReadFile("../../inputs/claude.inputs.toml")
	if err != nil {
		t.Fatalf("read claude.inputs.toml: %v", err)
	}
	var f InputFile
	if err := toml.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.Version != 1 {
		t.Errorf("version = %d, want 1", f.Version)
	}
	if len(f.Inputs) == 0 {
		t.Fatal("no inputs parsed")
	}
	in := f.Inputs[0]
	if in.ID != "claude" {
		t.Errorf("id = %q, want %q", in.ID, "claude")
	}
	if !in.Active {
		t.Error("active should be true")
	}
	if in.Decode.Format != "jsonl" {
		t.Errorf("format = %q, want jsonl", in.Decode.Format)
	}
	if len(in.Discover.Include) == 0 {
		t.Error("no include patterns")
	}
	if len(in.Record.IncludeWhen) == 0 {
		t.Error("no include_when predicates")
	}
	if in.Map.Role == "" {
		t.Error("map.role is empty")
	}
	if in.Text.DropEmpty != true {
		t.Error("text.drop_empty should be true")
	}
}

func TestUnmarshalPiPreset(t *testing.T) {
	data, err := os.ReadFile("../../tests/fixtures/pi.inputs.toml")
	if err != nil {
		t.Fatalf("read pi.inputs.toml: %v", err)
	}
	var f InputFile
	if err := toml.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(f.Inputs) == 0 {
		t.Fatal("no inputs parsed")
	}
	in := f.Inputs[0]
	if in.ID != "pi" {
		t.Errorf("id = %q, want %q", in.ID, "pi")
	}
	if in.Map.UUID != "$.id" {
		t.Errorf("map.uuid = %q, want %q", in.Map.UUID, "$.id")
	}
}
