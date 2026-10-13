package input_config

import (
	"context"
	"fmt"
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
	return DiscoverFilesContext(context.Background(), cfg)
}

// DiscoverFilesContext is DiscoverFiles with cancellation support while walking roots.
func DiscoverFilesContext(ctx context.Context, cfg DiscoverConfig) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var results []string
	seen := map[string]struct{}{}

	for _, root := range cfg.Roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		root = expandTilde(root, home)
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}

		for _, pattern := range cfg.Include {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			matches, err := walkGlobContext(ctx, absRoot, pattern, cfg.Exclude, cfg.FollowSymlinks, seen)
			if err != nil {
				return nil, err
			}
			results = append(results, matches...)
		}
	}
	return results, nil
}

const (
	maxDiscoveryDepth   = 256
	maxDiscoveryEntries = 100_000
)

func walkGlobContext(ctx context.Context, root, pattern string, excludes []string, followSymlinks bool, seen map[string]struct{}) ([]string, error) {
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil
	}
	rootInfo, err := os.Stat(physicalRoot)
	if err != nil || !rootInfo.IsDir() {
		return nil, nil
	}

	var results []string
	visitedEntries := 0

	addMatch := func(aliasPath string) error {
		for _, excl := range excludes {
			matched, err := matchPattern(aliasPath, excl)
			if err != nil {
				return err
			}
			if matched {
				return nil
			}
		}

		rel, err := filepath.Rel(root, aliasPath)
		if err != nil {
			return err
		}
		matched, err := matchDoublestar(pattern, rel)
		if err != nil || !matched {
			return err
		}

		abs, _, err := pathsec.ResolveInside(root, rel)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return nil
		}
		if _, dup := seen[abs]; !dup {
			seen[abs] = struct{}{}
			results = append(results, abs)
		}
		return nil
	}

	var walk func(aliasDir, physicalDir string, depth int, ancestors []fs.FileInfo) error
	walk = func(aliasDir, physicalDir string, depth int, ancestors []fs.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > maxDiscoveryDepth {
			return fmt.Errorf("discover %q: directory depth exceeds %d", aliasDir, maxDiscoveryDepth)
		}

		entries, err := os.ReadDir(physicalDir)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			visitedEntries++
			if visitedEntries > maxDiscoveryEntries {
				return fmt.Errorf("discover %q: entry limit exceeds %d", root, maxDiscoveryEntries)
			}

			aliasPath := filepath.Join(aliasDir, entry.Name())
			physicalPath := filepath.Join(physicalDir, entry.Name())
			info, err := os.Lstat(physicalPath)
			if err != nil {
				continue
			}

			if info.Mode()&fs.ModeSymlink != 0 {
				if !followSymlinks {
					continue
				}
				resolved, err := filepath.EvalSymlinks(physicalPath)
				if err != nil || !pathWithinRoot(physicalRoot, resolved) {
					continue
				}
				info, err = os.Stat(resolved)
				if err != nil {
					continue
				}
				physicalPath = resolved
			}

			if info.IsDir() {
				if directorySeen(info, ancestors) {
					continue
				}
				nextAncestors := append(append([]fs.FileInfo(nil), ancestors...), info)
				if err := walk(aliasPath, physicalPath, depth+1, nextAncestors); err != nil {
					return err
				}
				continue
			}
			if info.Mode().IsRegular() {
				if err := addMatch(aliasPath); err != nil {
					return err
				}
			}
		}
		return nil
	}

	err = walk(root, physicalRoot, 0, []fs.FileInfo{rootInfo})
	return results, err
}

func pathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func directorySeen(info fs.FileInfo, ancestors []fs.FileInfo) bool {
	for _, ancestor := range ancestors {
		if os.SameFile(info, ancestor) {
			return true
		}
	}
	return false
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
