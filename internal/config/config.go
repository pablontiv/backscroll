package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// EmbeddingConfig contains settings for the vector embedding system.
type EmbeddingConfig struct {
	Enabled             bool    `toml:"enabled"`
	ModelName           string  `toml:"model_name"`           // e.g., "all-MiniLM-L6-v2"
	ModelPath           string  `toml:"model_path"`           // local path or "" for auto-download
	SimilarityThreshold float32 `toml:"similarity_threshold"` // default: 0.7
	TopK                int     `toml:"top_k"`                // default: 10
}

// SourcesConfig contains paths for different source types.
type SourcesConfig struct {
	KE        []string `toml:"ke"`
	Decisions []string `toml:"decisions"`
	Memories  []string `toml:"memories"`
	Rules     []string `toml:"rules"`
	Specs     []string `toml:"specs"`
	Backlog   []string `toml:"backlog"`
}

// Config represents the complete application configuration.
type Config struct {
	DatabasePath        string          `toml:"database_path"`
	SessionDirs         []string        `toml:"session_dirs"`
	SessionDir          string          `toml:"session_dir"` // backward compat: single string
	Sources             SourcesConfig   `toml:"sources"`
	Embedding           EmbeddingConfig `toml:"embedding"`
	SessionDirsExplicit bool            `toml:"-"` // true if session_dirs was set in config
}

// Load loads the configuration from files and environment variables.
// Resolution order: ./backscroll.toml -> ~/.config/backscroll/config.toml -> env vars -> defaults
func Load() (*Config, error) {
	cfg := &Config{
		DatabasePath: defaultDatabasePath(),
		SessionDirs:  []string{"."},
		Embedding: EmbeddingConfig{
			Enabled:             false,
			ModelName:           "all-MiniLM-L6-v2",
			SimilarityThreshold: 0.7,
			TopK:                10,
		},
	}

	// Load global config from ~/.config/backscroll/config.toml
	globalPath := globalConfigPath()
	if err := loadConfigFile(globalPath, cfg); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	// Load local config from ./backscroll.toml (overrides global)
	if err := loadConfigFile("./backscroll.toml", cfg); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load local config: %w", err)
	}

	// Override with environment variables
	if dbPath := os.Getenv("BACKSCROLL_DATABASE_PATH"); dbPath != "" {
		cfg.DatabasePath = dbPath
	}

	if sessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS"); sessionDirs != "" {
		// Colon-separated paths
		cfg.SessionDirs = strings.Split(sessionDirs, ":")
		cfg.SessionDirsExplicit = true
	}

	// Handle backward compat: session_dir (singular string) -> session_dirs
	if cfg.SessionDir != "" && !cfg.SessionDirsExplicit {
		cfg.SessionDirs = []string{cfg.SessionDir}
	}

	// Ensure defaults are applied if nothing was set
	if len(cfg.SessionDirs) == 0 {
		cfg.SessionDirs = []string{"."}
	}

	return cfg, nil
}

// loadConfigFile loads and merges a TOML configuration file.
func loadConfigFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// First, unmarshal into a temporary config to detect what was explicitly set
	var tempCfg Config
	if err := toml.Unmarshal(data, &tempCfg); err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}

	// Now merge into cfg, being careful about what was explicitly set
	if tempCfg.DatabasePath != "" {
		cfg.DatabasePath = tempCfg.DatabasePath
	}

	// Handle session_dirs (array) or session_dir (single string)
	if len(tempCfg.SessionDirs) > 0 {
		cfg.SessionDirs = tempCfg.SessionDirs
		cfg.SessionDirsExplicit = true
	} else if tempCfg.SessionDir != "" {
		cfg.SessionDirs = []string{tempCfg.SessionDir}
		cfg.SessionDirsExplicit = true
	}

	// Merge sources config
	if len(tempCfg.Sources.KE) > 0 {
		cfg.Sources.KE = tempCfg.Sources.KE
	}
	if len(tempCfg.Sources.Decisions) > 0 {
		cfg.Sources.Decisions = tempCfg.Sources.Decisions
	}
	if len(tempCfg.Sources.Memories) > 0 {
		cfg.Sources.Memories = tempCfg.Sources.Memories
	}
	if len(tempCfg.Sources.Rules) > 0 {
		cfg.Sources.Rules = tempCfg.Sources.Rules
	}
	if len(tempCfg.Sources.Specs) > 0 {
		cfg.Sources.Specs = tempCfg.Sources.Specs
	}
	if len(tempCfg.Sources.Backlog) > 0 {
		cfg.Sources.Backlog = tempCfg.Sources.Backlog
	}

	// Merge embedding config if enabled
	if tempCfg.Embedding.Enabled {
		cfg.Embedding.Enabled = true
	}
	if tempCfg.Embedding.ModelName != "" {
		cfg.Embedding.ModelName = tempCfg.Embedding.ModelName
	}
	if tempCfg.Embedding.ModelPath != "" {
		cfg.Embedding.ModelPath = tempCfg.Embedding.ModelPath
	}
	if tempCfg.Embedding.SimilarityThreshold > 0 {
		cfg.Embedding.SimilarityThreshold = tempCfg.Embedding.SimilarityThreshold
	}
	if tempCfg.Embedding.TopK > 0 {
		cfg.Embedding.TopK = tempCfg.Embedding.TopK
	}

	return nil
}

// DiscoverSessionDirs returns immediate subdirectories of ~/.claude/projects/
func DiscoverSessionDirs() ([]string, error) {
	projectsDir := filepath.Join(homeDir(), ".claude", "projects")

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		// If directory doesn't exist, return empty slice, not an error
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(projectsDir, entry.Name()))
		}
	}

	return dirs, nil
}

// Helper functions

func defaultDatabasePath() string {
	return filepath.Join(homeDir(), ".backscroll.db")
}

func globalConfigPath() string {
	return filepath.Join(homeDir(), ".config", "backscroll", "config.toml")
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current user's home from environment
		if home = os.Getenv("HOME"); home == "" {
			home = "."
		}
	}
	return home
}
