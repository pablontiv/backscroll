package projects

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type ProjectConfig struct {
	ID               string   `toml:"id"`
	Roots            []string `toml:"roots"`
	WorktreePatterns []string `toml:"worktree_patterns"`
	Aliases          []string `toml:"aliases"`
}

type ProjectRegistry struct {
	Projects []ProjectConfig `toml:"projects"`
}

type ProjectHint struct {
	ProjectID string `toml:"project_id"`
}

type Confidence string

const (
	ConfidenceExact     Confidence = "exact"
	ConfidencePattern   Confidence = "pattern"
	ConfidenceHint      Confidence = "hint"
	ConfidenceTruncated Confidence = "truncated"
	ConfidenceUnknown   Confidence = "unknown"
)

type Identification struct {
	ProjectID  string
	Confidence Confidence
}

func LoadGlobalRegistry() ProjectRegistry {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "backscroll", "projects.toml")
	return loadRegistryFrom(path)
}

func loadRegistryFrom(path string) ProjectRegistry {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectRegistry{}
	}
	var reg ProjectRegistry
	if err := toml.Unmarshal(data, &reg); err != nil {
		return ProjectRegistry{}
	}
	return reg
}

// LoadLocalHint walks upward from cwd up to 4 directory levels looking for
// .backscroll/project.toml.
func LoadLocalHint(cwd string) *ProjectHint {
	current := cwd
	for range 4 {
		hintPath := filepath.Join(current, ".backscroll", "project.toml")
		if data, err := os.ReadFile(hintPath); err == nil {
			var hint ProjectHint
			if err := toml.Unmarshal(data, &hint); err == nil && hint.ProjectID != "" {
				return &hint
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}

// Identify resolves the canonical project for cwd.
// Resolution order: local hint → exact root → worktree pattern → truncated suffix → unknown.
func Identify(cwd string, registry ProjectRegistry) Identification {
	if hint := LoadLocalHint(cwd); hint != nil {
		return Identification{ProjectID: hint.ProjectID, Confidence: ConfidenceHint}
	}

	// 1. Exact root match (cwd == root itself).
	for _, p := range registry.Projects {
		for _, root := range p.Roots {
			if cwd == root {
				return Identification{ProjectID: p.ID, Confidence: ConfidenceExact}
			}
		}
	}

	// 2. Worktree pattern match — checked before subpath so worktrees get "pattern" confidence.
	for _, p := range registry.Projects {
		for _, pattern := range p.WorktreePatterns {
			if matched, _ := filepath.Match(pattern, cwd); matched {
				return Identification{ProjectID: p.ID, Confidence: ConfidencePattern}
			}
		}
	}

	// 3. Subpath under a known root.
	for _, p := range registry.Projects {
		for _, root := range p.Roots {
			if strings.HasPrefix(cwd, root+string(filepath.Separator)) {
				return Identification{ProjectID: p.ID, Confidence: ConfidenceExact}
			}
		}
	}

	// Truncated path: cwd suffix matches a known root (leading path stripped).
	cwdClean := strings.TrimPrefix(cwd, string(filepath.Separator))
	for _, p := range registry.Projects {
		for _, root := range p.Roots {
			rootClean := strings.TrimPrefix(root, string(filepath.Separator))
			if strings.HasSuffix(rootClean, cwdClean) || strings.HasSuffix(cwdClean, rootClean) {
				return Identification{ProjectID: p.ID, Confidence: ConfidenceTruncated}
			}
		}
	}

	return Identification{ProjectID: "unknown", Confidence: ConfidenceUnknown}
}

func ListProjects(registry ProjectRegistry) []ProjectConfig {
	return registry.Projects
}
