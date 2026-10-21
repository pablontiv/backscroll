package categories

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/pelletier/go-toml/v2"
)

//go:embed default_categories.toml
var defaultCategoriesFS embed.FS

type tomlFile struct {
	Version int `toml:"version"`
	Rules   []struct {
		Pattern  string `toml:"pattern"`
		Tool     string `toml:"tool"`
		Category string `toml:"category"`
	} `toml:"rule"`
}

// Load reads the category map from config dir, or falls back to embedded default.
// Consults BACKSCROLL_CONFIG_DIR env var (matching input_config pattern).
// If config file exists but is older than the embedded default, uses the embedded
// default and prints a notice to stderr.
func Load() (*Mapper, error) {
	// Decode the embedded default first; its version governs config selection.
	embeddedData, err := defaultCategoriesFS.ReadFile("default_categories.toml")
	if err != nil {
		return nil, fmt.Errorf("read embedded default categories: %w", err)
	}

	embedded, err := parseTOML(embeddedData)
	if err != nil {
		return nil, fmt.Errorf("parse embedded version: %w", err)
	}

	// Try to load from config dir
	configPath, err := configPath()
	if err == nil {
		if data, err := os.ReadFile(configPath); err == nil {
			config, err := parseTOML(data)
			if err == nil && config.Version < embedded.Version {
				// Config is stale; use embedded and print notice
				_, _ = fmt.Fprintf(os.Stderr, "categories: config version %d older than built-in %d; using built-in (update your categories.toml or set BACKSCROLL_FORCE_INPUTS=1 on install)\n",
					config.Version, embedded.Version)
				return buildMapper(embedded)
			}
			// Config is current or newer; use it
			if err == nil {
				return buildMapper(config)
			}
		}
	}

	// Fallback to embedded default
	return buildMapper(embedded)
}

func parseTOML(data []byte) (tomlFile, error) {
	var f tomlFile
	err := toml.Unmarshal(data, &f)
	return f, err
}

func configPath() (string, error) {
	base := os.Getenv("BACKSCROLL_CONFIG_DIR")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "backscroll", "inputs", "categories.toml"), nil
}

func buildMapper(f tomlFile) (*Mapper, error) {
	mapper := &Mapper{
		version: f.Version,
		rules:   make([]Rule, len(f.Rules)),
	}

	for i, r := range f.Rules {
		mapper.rules[i].Tool = r.Tool
		mapper.rules[i].Category = r.Category
		if r.Pattern != "" {
			compiled, err := regexp.Compile(r.Pattern)
			if err != nil {
				return nil, fmt.Errorf("compile pattern %q: %w", r.Pattern, err)
			}
			mapper.rules[i].Pattern = compiled
		}
	}

	return mapper, nil
}
