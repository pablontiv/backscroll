package input_config

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverFiles returns the absolute paths of files matching the given DiscoverConfig.
// Patterns in Include are glob patterns relative to each root in Roots.
// Patterns in Exclude are matched against the full path; any match skips the file.
// Tilde (~) in roots is expanded to the user's home directory.
func DiscoverFiles(cfg DiscoverConfig) ([]string, error) {
	home, _ := os.UserHomeDir()

	var results []string
	seen := map[string]struct{}{}

	for _, root := range cfg.Roots {
		root = expandTilde(root, home)
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}

		for _, pattern := range cfg.Include {
			matches, err := walkGlob(absRoot, pattern, cfg.Exclude, cfg.FollowSymlinks, seen)
			if err != nil {
				return nil, err
			}
			results = append(results, matches...)
		}
	}
	return results, nil
}

func walkGlob(root, pattern string, excludes []string, followSymlinks bool, seen map[string]struct{}) ([]string, error) {
	var results []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		if d.Type()&fs.ModeSymlink != 0 {
			if !followSymlinks {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			// Resolve symlink and check for loops
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil
			}
			if _, already := seen[resolved]; already {
				return filepath.SkipDir
			}
			seen[resolved] = struct{}{}
		}

		if d.IsDir() {
			return nil
		}

		// Check against exclude patterns
		for _, excl := range excludes {
			matched, err := matchPattern(path, excl)
			if err != nil {
				return err
			}
			if matched {
				return nil
			}
		}

		// Check against the include pattern
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		matched, err := filepath.Match(flattenDoublestar(pattern, rel), rel)
		if err != nil {
			return err
		}
		if !matched {
			// Try doublestar match
			if !matchDoublestar(pattern, rel) {
				return nil
			}
		}

		abs, _ := filepath.Abs(path)
		if _, dup := seen[abs]; !dup {
			seen[abs] = struct{}{}
			results = append(results, abs)
		}
		return nil
	})
	return results, err
}

// matchPattern matches a path against a glob pattern, supporting ** for any depth.
func matchPattern(path, pattern string) (bool, error) {
	// Normalize separators
	path = filepath.ToSlash(path)
	pattern = filepath.ToSlash(pattern)

	if strings.Contains(pattern, "**") {
		return matchDoublestarFull(path, pattern), nil
	}
	return filepath.Match(pattern, filepath.Base(path))
}

// matchDoublestar matches path relative to root against a **-containing pattern.
func matchDoublestar(pattern, rel string) bool {
	rel = filepath.ToSlash(rel)
	pattern = filepath.ToSlash(pattern)
	return matchDoublestarFull(rel, pattern)
}

// matchDoublestarFull implements simple ** glob matching.
// ** matches zero or more path segments.
func matchDoublestarFull(path, pattern string) bool {
	if !strings.Contains(pattern, "**") {
		matched, _ := filepath.Match(pattern, path)
		return matched
	}

	parts := strings.SplitN(pattern, "**", 2)
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")

	if prefix != "" && !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.TrimPrefix(rest, "/")

	if suffix == "" {
		return true
	}

	// suffix may itself contain **, recurse
	if strings.Contains(suffix, "**") {
		// simple: check that any trailing segment matches suffix
		segments := strings.Split(rest, "/")
		for i := range segments {
			candidate := strings.Join(segments[i:], "/")
			if matchDoublestarFull(candidate, suffix) {
				return true
			}
		}
		return false
	}

	// suffix is a simple glob — match against each trailing segment combination
	segments := strings.Split(rest, "/")
	for i := range segments {
		candidate := strings.Join(segments[i:], "/")
		matched, _ := filepath.Match(suffix, candidate)
		if matched {
			return true
		}
	}
	return false
}

// flattenDoublestar converts a **-pattern to a simple glob for filepath.Match
// when the relative path has no directory components that ** would skip.
func flattenDoublestar(pattern, rel string) string {
	if !strings.Contains(pattern, "**") {
		return pattern
	}
	// Return the base name pattern so filepath.Match can at least filter by extension
	parts := strings.Split(filepath.ToSlash(pattern), "/")
	return parts[len(parts)-1]
}

func expandTilde(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
