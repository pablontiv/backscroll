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
	ProjectID    string
	Confidence   Confidence
	FromRegistry bool // true iff the match came from registry.Projects (not fallback)
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

// NormalizeRootEquivalence maps equivalent roots (from cross-host syncs like
// /home/shared vs /Users/Shared) to a canonical form using the project registry.
// It finds the longest suffix of a canonical root that appears as a contiguous
// sequence in cwd's components. If found, cwd is remapped to use that canonical root.
// If no equivalent is found, cwd is returned unchanged.
func NormalizeRootEquivalence(cwd string, registry ProjectRegistry) string {
	if cwd == "" {
		return cwd
	}

	for _, p := range registry.Projects {
		for _, canonicalRoot := range p.Roots {
			if isCrossHostEquivalent(cwd, canonicalRoot) {
				return remapPath(cwd, canonicalRoot)
			}
		}
	}

	return cwd
}

// isCrossHostEquivalent returns true if cwd and canonicalRoot refer to the same
// logical path with different host/mount prefixes.
// It checks if a suffix of canonicalRoot's path components appears as a contiguous
// sequence in cwd's components, using case-insensitive comparison.
func isCrossHostEquivalent(cwd, canonicalRoot string) bool {
	cwdParts := strings.Split(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(cwd)), "/"), "/")
	rootParts := strings.Split(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(canonicalRoot)), "/"), "/")

	// Require at least 3 components in cwd and 2 in root
	if len(cwdParts) < 3 || len(rootParts) < 2 {
		return false
	}

	// Try tail lengths from longest to shortest (down to 2 components minimum)
	for tailLen := len(rootParts); tailLen >= 2; tailLen-- {
		tail := rootParts[len(rootParts)-tailLen:]

		// Check if cwd_parts contains tail as a contiguous subsequence
		for i := 0; i <= len(cwdParts)-len(tail); i++ {
			if slicesEqualFold(cwdParts[i:i+len(tail)], tail) {
				return true
			}
		}
	}

	return false
}

// remapPath rewrites cwd to use canonicalRoot, preserving any trailing subpath.
// Assumes isCrossHostEquivalent(cwd, canonicalRoot) returned true.
func remapPath(cwd, canonicalRoot string) string {
	cwdParts := strings.Split(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(cwd)), "/"), "/")
	rootParts := strings.Split(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(canonicalRoot)), "/"), "/")

	// Find the matching tail and its position in cwdParts
	for tailLen := len(rootParts); tailLen >= 2; tailLen-- {
		tail := rootParts[len(rootParts)-tailLen:]

		for i := 0; i <= len(cwdParts)-len(tail); i++ {
			if slicesEqualFold(cwdParts[i:i+len(tail)], tail) {
				// Found tail at position i; extract rest (subpath after tail)
				rest := cwdParts[i+len(tail):]
				// Reconstruct: canonical root + rest, preserving leading /
				parts := append(rootParts, rest...)
				return "/" + filepath.Join(parts...)
			}
		}
	}

	return cwd
}

// slicesEqualFold reports whether a and b are equal under Unicode case-folding.
func slicesEqualFold(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

// Identify resolves the canonical project for cwd.
// Resolution order: local hint → exact root → worktree pattern → subpath → truncated suffix → unknown.
// Paths are normalized for cross-host equivalence (e.g., /home/shared vs /Users/Shared roots).
// FromRegistry=true only if the match came from registry.Projects (not fallback).
func Identify(cwd string, registry ProjectRegistry) Identification {
	if hint := LoadLocalHint(cwd); hint != nil {
		return Identification{ProjectID: hint.ProjectID, Confidence: ConfidenceHint, FromRegistry: false}
	}

	// Normalize cwd for cross-host equivalence
	normalizedCwd := NormalizeRootEquivalence(cwd, registry)

	// 1. Exact root match (normalizedCwd == root itself).
	for _, p := range registry.Projects {
		for _, root := range p.Roots {
			if normalizedCwd == root {
				return Identification{ProjectID: p.ID, Confidence: ConfidenceExact, FromRegistry: true}
			}
		}
	}

	// 2. Worktree pattern match — checked before subpath so worktrees get "pattern" confidence.
	for _, p := range registry.Projects {
		for _, pattern := range p.WorktreePatterns {
			if matched, _ := filepath.Match(pattern, normalizedCwd); matched {
				return Identification{ProjectID: p.ID, Confidence: ConfidencePattern, FromRegistry: true}
			}
		}
	}

	// 3. Subpath under a known root.
	for _, p := range registry.Projects {
		for _, root := range p.Roots {
			if strings.HasPrefix(normalizedCwd, root+string(filepath.Separator)) {
				return Identification{ProjectID: p.ID, Confidence: ConfidenceExact, FromRegistry: true}
			}
		}
	}

	// Truncated path: normalizedCwd suffix matches a known root (leading path stripped).
	cwdClean := strings.TrimPrefix(normalizedCwd, string(filepath.Separator))
	for _, p := range registry.Projects {
		for _, root := range p.Roots {
			rootClean := strings.TrimPrefix(root, string(filepath.Separator))
			if strings.HasSuffix(rootClean, cwdClean) || strings.HasSuffix(cwdClean, rootClean) {
				return Identification{ProjectID: p.ID, Confidence: ConfidenceTruncated, FromRegistry: true}
			}
		}
	}

	// No registry match. Try fallback identity from cwd.
	fallbackID := DeriveFallbackID(normalizedCwd)
	if fallbackID != "unknown" {
		return Identification{ProjectID: fallbackID, Confidence: ConfidenceUnknown, FromRegistry: false}
	}

	return Identification{ProjectID: "unknown", Confidence: ConfidenceUnknown, FromRegistry: false}
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
