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
	if in.Decode.Format != "claude" {
		t.Errorf("format = %q, want claude", in.Decode.Format)
	}
	if len(in.Discover.Include) == 0 {
		t.Error("no include patterns")
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
}

func TestDecodeConfig_IndexReasoning(t *testing.T) {
	tests := []struct {
		name      string
		toml      string
		wantValue bool
	}{
		{
			name: "default false when omitted",
			toml: `[inputs]
format = "pi"`,
			wantValue: false,
		},
		{
			name: "explicit false",
			toml: `[inputs]
format = "pi"
index_reasoning = false`,
			wantValue: false,
		},
		{
			name: "explicit true",
			toml: `[inputs]
format = "pi"
index_reasoning = true`,
			wantValue: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg struct {
				Inputs struct {
					Format         string `toml:"format"`
					IndexReasoning bool   `toml:"index_reasoning"`
				} `toml:"inputs"`
			}
			if err := toml.Unmarshal([]byte(tt.toml), &cfg); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if cfg.Inputs.IndexReasoning != tt.wantValue {
				t.Errorf("IndexReasoning = %v, want %v", cfg.Inputs.IndexReasoning, tt.wantValue)
			}
		})
	}
}
