package input_config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionDirsToManifest(t *testing.T) {
	dirs := []string{"/home/user/.claude/projects"}
	def := SessionDirsToManifest(dirs)

	if def.ID != "legacy-session-dirs" {
		t.Errorf("id = %q", def.ID)
	}
	if !def.Active {
		t.Error("should be active")
	}
	if def.Decode.Format != "jsonl" {
		t.Errorf("format = %q", def.Decode.Format)
	}
	if len(def.Discover.Include) == 0 {
		t.Error("no include patterns")
	}
	found := false
	for _, excl := range def.Discover.Exclude {
		if excl == "**/subagents/**" {
			found = true
		}
	}
	if !found {
		t.Error("expected subagents exclude pattern")
	}
	if def.Map.Role != "$.message.role" {
		t.Errorf("map.role = %q", def.Map.Role)
	}
	if !def.Text.DropEmpty {
		t.Error("drop_empty should be true")
	}
}

func TestActiveInputs_declarative(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir)

	// Write a preset
	data, err := os.ReadFile("../../inputs/claude.inputs.toml")
	if err != nil {
		t.Fatalf("read preset: %v", err)
	}
	inputsDir := filepath.Join(dir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputsDir, "claude.inputs.toml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	defs, mode, err := ActiveInputs([]string{"/some/dir"})
	if err != nil {
		t.Fatalf("ActiveInputs: %v", err)
	}
	if mode != ModeDeclarative {
		t.Errorf("mode = %v, want Declarative", mode)
	}
	if len(defs) == 0 {
		t.Error("expected at least one declarative input")
	}
	if defs[0].ID == "legacy-session-dirs" {
		t.Error("should not use legacy manifest when declarative inputs exist")
	}
}

func TestActiveInputs_legacy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir) // empty inputs dir

	defs, mode, err := ActiveInputs([]string{"/home/user/.claude/projects"})
	if err != nil {
		t.Fatalf("ActiveInputs: %v", err)
	}
	if mode != ModeLegacy {
		t.Errorf("mode = %v, want Legacy", mode)
	}
	if len(defs) != 1 {
		t.Fatalf("expected 1 legacy def, got %d", len(defs))
	}
	if defs[0].ID != "legacy-session-dirs" {
		t.Errorf("id = %q", defs[0].ID)
	}
}

func TestActiveInputs_empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir)

	defs, mode, err := ActiveInputs(nil)
	if err != nil {
		t.Fatalf("ActiveInputs: %v", err)
	}
	if mode != ModeUnknown {
		t.Errorf("mode = %v, want Unknown", mode)
	}
	if len(defs) != 0 {
		t.Errorf("expected empty, got %d defs", len(defs))
	}
}
