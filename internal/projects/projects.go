package projects

import (
	"errors"
	"fmt"
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
	ProjectID    string
	Confidence   Confidence
	FromRegistry bool // true iff the match came from registry.Projects (not fallback)
}

// LoadGlobalRegistry reads the optional registry. Only a missing file is empty;
// explicit unreadable or malformed state must not become fallback attribution.
func LoadGlobalRegistry() (ProjectRegistry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ProjectRegistry{}, fmt.Errorf("resolve project registry home: %w", err)
	}
	return loadRegistryFrom(filepath.Join(home, ".config", "backscroll", "projects.toml"))
}

func loadRegistryFrom(path string) (ProjectRegistry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ProjectRegistry{}, nil
	}
	if err != nil {
		return ProjectRegistry{}, fmt.Errorf("read project registry %q: %w; fix its path or read permissions", path, err)
	}
	var reg ProjectRegistry
	if err := toml.Unmarshal(data, &reg); err != nil {
		return ProjectRegistry{}, fmt.Errorf("parse project registry %q: %w; fix the TOML configuration", path, err)
	}
	return reg, nil
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

// NormalizeRootEquivalence maps only explicitly configured aliases to a primary
// root. Ambiguous matches are left unchanged; suffixes are never identity proof.
func NormalizeRootEquivalence(cwd string, registry ProjectRegistry) string {
	identity := Identify(cwd, registry)
	if !identity.FromRegistry {
		return cwd
	}
	for _, p := range registry.Projects {
		if p.ID != identity.ProjectID || len(p.Roots) == 0 {
			continue
		}
		for _, alias := range p.Aliases {
			if underRoot(cwd, alias) {
				suffix := strings.TrimPrefix(filepath.Clean(cwd), filepath.Clean(alias))
				return filepath.Clean(p.Roots[0]) + suffix
			}
		}
	}
	return cwd
}

func underRoot(cwd, root string) bool {
	if cwd == "" || root == "" || !filepath.IsAbs(cwd) || !filepath.IsAbs(root) {
		return false
	}
	cwd, root = filepath.Clean(cwd), filepath.Clean(root)
	return cwd == root || strings.HasPrefix(cwd, strings.TrimRight(root, string(filepath.Separator))+string(filepath.Separator))
}

// Identify resolves hints and explicit registry roots/aliases. Matches belonging
// to different project IDs are non-matches, independent of registry file order.
func Identify(cwd string, registry ProjectRegistry) Identification {
	if cwd != "" {
		if hint := LoadLocalHint(cwd); hint != nil {
			return Identification{ProjectID: hint.ProjectID, Confidence: ConfidenceHint}
		}
	}
	var found Identification
	foundPriority := 0
	ambiguous := false
	for _, p := range registry.Projects {
		confidence := ConfidenceUnknown
		priority := 0
		for _, root := range append(append([]string{}, p.Roots...), p.Aliases...) {
			if underRoot(cwd, root) {
				confidence = ConfidenceExact
				if priority < 1 {
					priority = 1
				}
				if filepath.Clean(cwd) == filepath.Clean(root) {
					priority = 3
				}
			}
		}
		for _, pattern := range p.WorktreePatterns {
			if cwd != "" {
				if matched, _ := filepath.Match(pattern, filepath.Clean(cwd)); matched && priority < 3 {
					confidence = ConfidencePattern
					priority = 2
				}
			}
		}
		if confidence == ConfidenceUnknown {
			continue
		}
		if found.FromRegistry && found.ProjectID != p.ID {
			ambiguous = true
		}
		if !found.FromRegistry || (found.ProjectID == p.ID && priority > foundPriority) {
			foundPriority = priority
			found = Identification{ProjectID: p.ID, Confidence: confidence, FromRegistry: true}
		}
	}
	if found.FromRegistry && !ambiguous {
		return found
	}
	return Identification{ProjectID: DeriveFallbackID(cwd), Confidence: ConfidenceUnknown}
}

func ListProjects(registry ProjectRegistry) []ProjectConfig {
	return registry.Projects
}

// DeriveFallbackID extracts and sanitizes the last path component to form a project ID.
// Lowercases, keeps alphanumerics, dashes, and underscores; strips other chars.
// Returns "unknown" if cwd is empty or has no valid basename.
func DeriveFallbackID(cwd string) string {
	if cwd == "" {
		return "unknown"
	}

	// Extract last component
	lastComponent := filepath.Base(filepath.Clean(cwd))
	if lastComponent == "" || lastComponent == "." {
		return "unknown"
	}

	// Sanitize: lowercase, keep [a-z0-9-_], drop everything else
	var sanitized strings.Builder
	for _, r := range strings.ToLower(lastComponent) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sanitized.WriteRune(r)
		}
	}

	result := sanitized.String()
	if result == "" {
		return "unknown"
	}
	return result
}
