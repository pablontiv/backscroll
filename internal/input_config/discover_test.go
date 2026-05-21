package input_config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func mkfile(t *testing.T, dir, rel string) string {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverFiles_includeExclude(t *testing.T) {
	dir := t.TempDir()
	a := mkfile(t, dir, "proj/session.jsonl")
	_ = mkfile(t, dir, "proj/subagents/sub.jsonl")
	_ = mkfile(t, dir, "proj/other.txt")

	cfg := DiscoverConfig{
		Roots:   []string{dir},
		Include: []string{"**/*.jsonl"},
		Exclude: []string{"**/subagents/**"},
	}
	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	if len(files) != 1 || files[0] != a {
		t.Errorf("got %v, want [%s]", files, a)
	}
}

func TestDiscoverFiles_multipleRoots(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	a := mkfile(t, dir1, "a.jsonl")
	b := mkfile(t, dir2, "b.jsonl")

	cfg := DiscoverConfig{
		Roots:   []string{dir1, dir2},
		Include: []string{"**/*.jsonl"},
	}
	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	sort.Strings(files)
	want := []string{a, b}
	sort.Strings(want)
	if len(files) != len(want) {
		t.Errorf("got %v, want %v", files, want)
	}
}

func TestDiscoverFiles_noSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := mkfile(t, dir, "real/session.jsonl")
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Dir(target), link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	cfg := DiscoverConfig{
		Roots:          []string{dir},
		Include:        []string{"**/*.jsonl"},
		FollowSymlinks: false,
	}
	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	// Should find the real file but not traverse the symlinked dir
	for _, f := range files {
		if filepath.Dir(f) == link {
			t.Errorf("traversed symlinked dir %s", link)
		}
	}
}

func TestDiscoverFiles_missingRoot(t *testing.T) {
	cfg := DiscoverConfig{
		Roots:   []string{"/nonexistent/path/xyz"},
		Include: []string{"**/*.jsonl"},
	}
	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("expected no error for missing root: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty result, got %v", files)
	}
}

func TestDiscoverFiles_noDuplicates(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, dir, "a.jsonl")

	cfg := DiscoverConfig{
		Roots:   []string{dir, dir}, // same root twice
		Include: []string{"**/*.jsonl"},
	}
	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file (no duplicates), got %d: %v", len(files), files)
	}
}

func TestDiscover_RejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	outsideFile := mkfile(t, outside, "escape.jsonl")

	link := filepath.Join(dir, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	cfg := DiscoverConfig{
		Roots:          []string{dir},
		Include:        []string{"**/*.jsonl"},
		FollowSymlinks: true,
	}
	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	// Should not include files from outside the root, even via symlinks
	for _, f := range files {
		if f == outsideFile {
			t.Errorf("symlink escape was not rejected: %s", f)
		}
	}
}

func TestDiscover_RejectsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	_ = mkfile(t, outside, "abs.jsonl")

	cfg := DiscoverConfig{
		Roots:   []string{dir},
		Include: []string{outside}, // absolute path
	}
	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	// Absolute paths should be rejected
	if len(files) > 0 {
		t.Errorf("absolute path was not rejected: got %v", files)
	}
}

func TestDiscover_AcceptsValidNested(t *testing.T) {
	dir := t.TempDir()
	validFile := mkfile(t, dir, "subdir/valid.jsonl")

	cfg := DiscoverConfig{
		Roots:   []string{dir},
		Include: []string{"**/*.jsonl"},
	}
	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	if len(files) != 1 || files[0] != validFile {
		t.Errorf("got %v, want [%s]", files, validFile)
	}
}

func TestDiscover_AcceptsTildeExpansion(t *testing.T) {
	// Create a temporary home directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create a test file in the temp home
	testFile := mkfile(t, tmpHome, "subdir/test.jsonl")

	cfg := DiscoverConfig{
		Roots:   []string{"~/subdir"},
		Include: []string{"*.jsonl"},
	}
	files, err := DiscoverFiles(cfg)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	if len(files) != 1 || files[0] != testFile {
		t.Errorf("got %v, want [%s]", files, testFile)
	}
}
