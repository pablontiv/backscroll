package input_config

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablontiv/picokit/pathsec"
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
		matched, err := matchDoublestar(pattern, rel)
		if err != nil {
			return err
		}
		if !matched {
			return nil
		}

		// Validate path stays within root and resolves symlinks securely
		abs, _, err := pathsec.ResolveInside(root, rel)
		if err != nil {
			return nil // Skip paths that escape root or are invalid
		}
		if _, dup := seen[abs]; !dup {
			seen[abs] = struct{}{}
			results = append(results, abs)
		}
		return nil
	})
	return results, err
}

// matchPattern matches an exclusion against a path, supporting ** for any depth.
func matchPattern(path, pattern string) (bool, error) {
	if strings.Contains(pattern, "**") {
		return matchDoublestar(pattern, path)
	}
	return filepath.Match(pattern, filepath.Base(path))
}

// matchDoublestar matches a slash-separated path against a glob. A ** path
// segment matches zero or more complete path segments; all other segments use
// filepath.Match semantics.
func matchDoublestar(pattern, path string) (bool, error) {
	pathSegments := strings.Split(filepath.ToSlash(path), "/")
	patternSegments := strings.Split(filepath.ToSlash(pattern), "/")
	return matchGlobSegments(patternSegments, pathSegments)
}

func matchGlobSegments(pattern, path []string) (bool, error) {
	if len(pattern) == 0 {
		return len(path) == 0, nil
	}

	if pattern[0] == "**" {
		// Adjacent doublestars are equivalent to one and avoiding them here keeps
		// recursive matching bounded to the number of path segments.
		for len(pattern) > 1 && pattern[1] == "**" {
			pattern = pattern[1:]
		}
		if len(pattern) == 1 {
			return true, nil
		}
		for i := 0; i <= len(path); i++ {
			matched, err := matchGlobSegments(pattern[1:], path[i:])
			if err != nil || matched {
				return matched, err
			}
		}
		return false, nil
	}

	if len(path) == 0 {
		return false, nil
	}
	matched, err := filepath.Match(pattern[0], path[0])
	if err != nil || !matched {
		return false, err
	}
	return matchGlobSegments(pattern[1:], path[1:])
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
