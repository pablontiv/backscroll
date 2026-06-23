package input_config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// InputsDir returns the canonical directory for *.inputs.toml files.
// Respects BACKSCROLL_CONFIG_DIR if set; otherwise uses ~/.config, matching
// the config.toml and projects.toml resolution (which hardcode ~/.config/backscroll)
// so all backscroll config lives under one directory on every OS.
func InputsDir() (string, error) {
	base := os.Getenv("BACKSCROLL_CONFIG_DIR")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "backscroll", "inputs"), nil
}

// LoadInputs loads all active *.inputs.toml files from InputsDir.
// Returns an empty slice (not an error) if the directory does not exist.
func LoadInputs() ([]InputDefinition, error) {
	dir, err := InputsDir()
	if err != nil {
		return nil, err
	}
	return LoadInputsFromDir(dir)
}

// LoadInputsFromDir loads all active input definitions from the given directory.
func LoadInputsFromDir(dir string) ([]InputDefinition, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.inputs.toml"))
	if err != nil {
		return nil, fmt.Errorf("glob inputs: %w", err)
	}

	var active []InputDefinition
	for _, path := range entries {
		defs, err := loadFile(path)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
		for _, def := range defs {
			if def.Active {
				active = append(active, def)
			}
		}
	}
	return active, nil
}

func loadFile(path string) ([]InputDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f InputFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}
	return f.Inputs, nil
}
