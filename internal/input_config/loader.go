package input_config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	if f.Version != 1 {
		return nil, fmt.Errorf("version: expected 1, got %d; set version = 1", f.Version)
	}
	for i := range f.Inputs {
		def := &f.Inputs[i]
		def.manifestPath, def.manifestIndex = path, i
		if !def.Active {
			continue
		}
		if err := validateDefinition(def); err != nil {
			return nil, fmt.Errorf("%s: %w", def.Location(), err)
		}
	}
	return f.Inputs, nil
}

// validateDefinition validates even patterns with no matching files or roots.
// Source and decoder defaults match the historical runtime defaults.
func validateDefinition(def *InputDefinition) error {
	if strings.TrimSpace(def.ID) == "" {
		return fmt.Errorf("id: an active input requires a non-empty name")
	}
	if def.Source == "" {
		def.Source = "session"
	}
	if def.Decode.Format == "" {
		def.Decode.Format = "claude"
	}
	if len(def.Discover.Roots) == 0 {
		return fmt.Errorf("discover.roots: an active input requires at least one root")
	}
	for i, root := range def.Discover.Roots {
		if strings.TrimSpace(root) == "" {
			return fmt.Errorf("discover.roots[%d]: root must not be empty", i)
		}
	}
	if len(def.Discover.Include) == 0 {
		return fmt.Errorf("discover.include: an active input requires at least one pattern")
	}
	for _, field := range []struct {
		name     string
		patterns []string
	}{{"include", def.Discover.Include}, {"exclude", def.Discover.Exclude}} {
		for i, pattern := range field.patterns {
			if strings.TrimSpace(pattern) == "" {
				return fmt.Errorf("discover.%s[%d]: pattern must not be empty", field.name, i)
			}
			// Validate every segment independently: matching against one sample path
			// would short circuit before malformed later segments.
			for _, segment := range strings.Split(filepath.ToSlash(pattern), "/") {
				if segment == "**" {
					continue
				}
				if _, err := filepath.Match(segment, ""); err != nil {
					return fmt.Errorf("discover.%s[%d]: invalid pattern %q: %w", field.name, i, pattern, err)
				}
			}
		}
	}
	return nil
}
