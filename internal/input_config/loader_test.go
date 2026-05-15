package input_config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadInputsFromDir(t *testing.T) {
	dir := t.TempDir()

	// Copy claude preset into temp dir
	data, err := os.ReadFile("../../inputs/claude.inputs.toml")
	if err != nil {
		t.Fatalf("read preset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "claude.inputs.toml"), data, 0o644); err != nil {
		t.Fatalf("write preset: %v", err)
	}

	defs, err := LoadInputsFromDir(dir)
	if err != nil {
		t.Fatalf("LoadInputsFromDir: %v", err)
	}
	if len(defs) == 0 {
		t.Fatal("no active inputs returned")
	}
	if defs[0].ID != "claude" {
		t.Errorf("id = %q, want %q", defs[0].ID, "claude")
	}
}

func TestLoadInputsFromDir_disabled(t *testing.T) {
	dir := t.TempDir()

	const tomlContent = `version = 1
[[inputs]]
id = "disabled"
active = false
`
	if err := os.WriteFile(filepath.Join(dir, "disabled.inputs.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	defs, err := LoadInputsFromDir(dir)
	if err != nil {
		t.Fatalf("LoadInputsFromDir: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("expected 0 active inputs, got %d", len(defs))
	}
}

func TestLoadInputsFromDir_missingDir(t *testing.T) {
	defs, err := LoadInputsFromDir("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("unexpected error for missing dir: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("expected empty result, got %d", len(defs))
	}
}

func TestLoadInputsFromDir_invalidTOML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.inputs.toml"), []byte("not valid toml }{"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadInputsFromDir(dir)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestInputsDirEnvOverride(t *testing.T) {
	want := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", want)

	got, err := InputsDir()
	if err != nil {
		t.Fatalf("InputsDir: %v", err)
	}
	if got != filepath.Join(want, "backscroll", "inputs") {
		t.Errorf("got %q, want %q", got, filepath.Join(want, "backscroll", "inputs"))
	}
}

func TestInputsDirNoEnv(t *testing.T) {
	t.Setenv("BACKSCROLL_CONFIG_DIR", "")

	got, err := InputsDir()
	if err != nil {
		t.Fatalf("InputsDir without env: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty dir from os.UserConfigDir()")
	}
}
