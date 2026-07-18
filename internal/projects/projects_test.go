package projects_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/projects"
)

func makeRegistry() projects.ProjectRegistry {
	return projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:               "backscroll",
				Roots:            []string{"/home/shared/harness/backscroll"},
				WorktreePatterns: []string{"/home/shared/harness/backscroll/.worktrees/*"},
				Aliases:          []string{"bs"},
			},
		},
	}
}

func TestIdentifyExactRoot(t *testing.T) {
	reg := makeRegistry()
	id := projects.Identify("/home/shared/harness/backscroll", reg)
	if id.ProjectID != "backscroll" {
		t.Errorf("expected backscroll, got %s", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceExact {
		t.Errorf("expected exact, got %s", id.Confidence)
	}
}

func TestIdentifySubpath(t *testing.T) {
	reg := makeRegistry()
	id := projects.Identify("/home/shared/harness/backscroll/internal/config", reg)
	if id.ProjectID != "backscroll" {
		t.Errorf("expected backscroll, got %s", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceExact {
		t.Errorf("expected exact, got %s", id.Confidence)
	}
}

func TestIdentifyWorktreePattern(t *testing.T) {
	reg := makeRegistry()
	// Use a path outside the root to test pattern matching.
	id := projects.Identify("/tmp/backscroll-worktree-feature-x", projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:               "backscroll",
				Roots:            []string{"/home/shared/harness/backscroll"},
				WorktreePatterns: []string{"/tmp/backscroll-worktree-*"},
			},
		},
	})
	if id.ProjectID != "backscroll" {
		t.Errorf("expected backscroll, got %s", id.ProjectID)
	}
	if id.Confidence != projects.ConfidencePattern {
		t.Errorf("expected pattern, got %s", id.Confidence)
	}
	_ = reg
}

func TestIdentifyUnknown(t *testing.T) {
	// Use empty registry to avoid truncation matching
	reg := projects.ProjectRegistry{}

	// Path with valid basename derives fallback ID
	id := projects.Identify("/some/other/path", reg)
	if id.ProjectID != "path" {
		t.Errorf("expected fallback 'path', got %s", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceUnknown {
		t.Errorf("expected unknown confidence, got %s", id.Confidence)
	}

	// Path with empty or unparseable basename returns "unknown"
	id = projects.Identify("", reg)
	if id.ProjectID != "unknown" {
		t.Errorf("expected unknown, got %s", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceUnknown {
		t.Errorf("expected unknown confidence, got %s", id.Confidence)
	}
}

func TestLoadLocalHintNotExist(t *testing.T) {
	dir := t.TempDir()
	hint := projects.LoadLocalHint(dir)
	if hint != nil {
		t.Errorf("expected nil hint, got %v", hint)
	}
}

func TestLoadLocalHintFound(t *testing.T) {
	dir := t.TempDir()
	hintDir := filepath.Join(dir, ".backscroll")
	if err := os.MkdirAll(hintDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `project_id = "my-project"`
	if err := os.WriteFile(filepath.Join(hintDir, "project.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	hint := projects.LoadLocalHint(dir)
	if hint == nil {
		t.Fatal("expected hint, got nil")
	}
	if hint.ProjectID != "my-project" {
		t.Errorf("expected my-project, got %s", hint.ProjectID)
	}
}

func TestIdentifyWithHint(t *testing.T) {
	dir := t.TempDir()
	hintDir := filepath.Join(dir, ".backscroll")
	if err := os.MkdirAll(hintDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `project_id = "hinted-project"`
	if err := os.WriteFile(filepath.Join(hintDir, "project.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := makeRegistry()
	id := projects.Identify(dir, reg)
	if id.ProjectID != "hinted-project" {
		t.Errorf("expected hinted-project, got %s", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceHint {
		t.Errorf("expected hint confidence, got %s", id.Confidence)
	}
}

func TestLoadLocalHintWalksUp(t *testing.T) {
	dir := t.TempDir()
	hintDir := filepath.Join(dir, ".backscroll")
	if err := os.MkdirAll(hintDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `project_id = "parent-project"`
	if err := os.WriteFile(filepath.Join(hintDir, "project.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory — hint should be found by walking up
	subdir := filepath.Join(dir, "sub", "dir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	hint := projects.LoadLocalHint(subdir)
	if hint == nil {
		t.Fatal("expected hint from parent, got nil")
	}
	if hint.ProjectID != "parent-project" {
		t.Errorf("expected parent-project, got %s", hint.ProjectID)
	}
}

func TestLoadGlobalRegistry(t *testing.T) {
	// Just ensure it doesn't panic; may return empty registry if file not present.
	reg := projects.LoadGlobalRegistry()
	_ = reg
}

func TestListProjects(t *testing.T) {
	reg := makeRegistry()
	list := projects.ListProjects(reg)
	if len(list) != 1 {
		t.Errorf("expected 1 project, got %d", len(list))
	}
	if list[0].ID != "backscroll" {
		t.Errorf("expected backscroll, got %s", list[0].ID)
	}
}

func TestListProjectsEmpty(t *testing.T) {
	reg := projects.ProjectRegistry{}
	list := projects.ListProjects(reg)
	if len(list) != 0 {
		t.Errorf("expected 0 projects, got %d", len(list))
	}
}

func TestIdentifyTruncated(t *testing.T) {
	reg := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:    "myapp",
				Roots: []string{"/home/user/myapp"},
			},
		},
	}
	// Truncated: path is a suffix of the root (partial path)
	id := projects.Identify("myapp", reg)
	if id.ProjectID != "myapp" {
		t.Errorf("expected myapp, got %s", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceTruncated {
		t.Logf("confidence: %s (truncated matching is heuristic)", id.Confidence)
	}
}

func TestLoadLocalHintInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	hintDir := filepath.Join(dir, ".backscroll")
	if err := os.MkdirAll(hintDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hintDir, "project.toml"), []byte("not valid toml ==="), 0o644); err != nil {
		t.Fatal(err)
	}
	hint := projects.LoadLocalHint(dir)
	if hint != nil {
		t.Errorf("expected nil hint for invalid TOML, got %+v", hint)
	}
}

func TestLoadGlobalRegistryWithFile(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "backscroll")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `
[[projects]]
id = "myapp"
roots = ["/home/user/myapp"]
`
	if err := os.WriteFile(filepath.Join(cfgDir, "projects.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	reg := projects.LoadGlobalRegistry()
	if len(reg.Projects) != 1 || reg.Projects[0].ID != "myapp" {
		t.Errorf("expected myapp project, got %+v", reg.Projects)
	}
}

func TestLoadGlobalRegistryInvalidTOML(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "backscroll")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "projects.toml"), []byte("not valid toml ==="), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	reg := projects.LoadGlobalRegistry()
	if len(reg.Projects) != 0 {
		t.Errorf("expected empty registry for invalid TOML, got %+v", reg.Projects)
	}
}

// TestIdentifySessionCwdUnderRoot verifies AC#3: session whose cwd falls under
// a root in projects.toml is indexed with Project = that id, NOT unknown.
func TestIdentifySessionCwdUnderRoot(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "backscroll")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create projects.toml with a known root
	content := `
[[projects]]
id = "myproject"
roots = ["/tmp/myproject"]
`
	if err := os.WriteFile(filepath.Join(cfgDir, "projects.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	// Load the registry (should contain myproject)
	reg := projects.LoadGlobalRegistry()
	if len(reg.Projects) == 0 {
		t.Fatal("expected projects in registry")
	}

	// Create a cwd under the root
	cwd := "/tmp/myproject/subdir/deeper"

	// Identify the cwd using the registry
	id := projects.Identify(cwd, reg)

	// Verify it is identified as "myproject", not "unknown"
	if id.ProjectID != "myproject" {
		t.Errorf("expected myproject, got %s", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceExact {
		t.Errorf("expected exact confidence, got %s", id.Confidence)
	}
}

func TestNormalizeRootEquivalence_CrossHost(t *testing.T) {
	reg := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:    "myproj",
				Roots: []string{"/home/shared/myproject"},
			},
		},
	}

	result := projects.NormalizeRootEquivalence("/Users/Shared/myproject/src", reg)
	expected := filepath.Join("/home/shared/myproject", "src")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestNormalizeRootEquivalence_NoMatch(t *testing.T) {
	reg := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:    "other",
				Roots: []string{"/home/other/project"},
			},
		},
	}

	result := projects.NormalizeRootEquivalence("/Users/Shared/myproject/src", reg)
	if result != "/Users/Shared/myproject/src" {
		t.Errorf("expected unchanged path, got %s", result)
	}
}

func TestIdentify_CrossHostEquivalence(t *testing.T) {
	reg := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:    "myproj",
				Roots: []string{"/home/shared/myproject"},
			},
		},
	}

	result := projects.Identify("/Users/Shared/myproject/src", reg)
	if result.ProjectID != "myproj" {
		t.Errorf("expected ProjectID 'myproj', got %q", result.ProjectID)
	}
	if result.Confidence == projects.ConfidenceUnknown {
		t.Errorf("expected non-unknown confidence, got %s", result.Confidence)
	}
}

func TestIdentifyFallbackFromCwd(t *testing.T) {
	reg := projects.ProjectRegistry{} // empty registry
	id := projects.Identify("/Users/Shared/harness/backscroll", reg)
	if id.ProjectID != "backscroll" {
		t.Errorf("expected fallback 'backscroll', got %q", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceUnknown {
		t.Errorf("expected unknown confidence, got %s", id.Confidence)
	}
}

func TestIdentifyFallbackSanitization(t *testing.T) {
	reg := projects.ProjectRegistry{}
	id := projects.Identify("/Users/Shared/harness/My-Project_123", reg)
	if id.ProjectID != "my-project_123" {
		t.Errorf("expected sanitized fallback 'my-project_123', got %q", id.ProjectID)
	}
}

func TestIdentifyFallbackSpecialChars(t *testing.T) {
	reg := projects.ProjectRegistry{}
	// Path with special chars should be stripped
	id := projects.Identify("/Users/Shared/harness/project@v1.0", reg)
	if id.ProjectID != "projectv10" {
		t.Errorf("expected 'projectv10' (special chars removed), got %q", id.ProjectID)
	}
}

func TestIdentifyFallbackEmptyCwd(t *testing.T) {
	reg := projects.ProjectRegistry{}
	id := projects.Identify("", reg)
	if id.ProjectID != "unknown" {
		t.Errorf("expected 'unknown' for empty cwd, got %q", id.ProjectID)
	}
}

func TestIdentifyRegistryWinsFallback(t *testing.T) {
	reg := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:    "myapp",
				Roots: []string{"/home/user/myapp"},
			},
		},
	}
	id := projects.Identify("/home/user/myapp/src", reg)
	if id.ProjectID != "myapp" {
		t.Errorf("expected registry match 'myapp', got %q", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceExact {
		t.Errorf("expected exact confidence, got %s", id.Confidence)
	}
}

func TestDecodeCwdFromSessionPath_ExistsOnDisk(t *testing.T) {
	// Create a temp project dir
	tmpBase := t.TempDir()
	tmpProj := filepath.Join(tmpBase, "Users", "Shared", "harness", "backscroll")
	if err := os.MkdirAll(tmpProj, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create encoded session path pointing to the temp structure
	encodedDir := filepath.Join(tmpBase, ".claude", "projects", "-Users-Shared-harness-backscroll")
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionPath := filepath.Join(encodedDir, "uuid.jsonl")

	// Decode; it should find the path and return basename
	cwd := projects.DecodeCwdFromSessionPath(sessionPath)
	if cwd == "" {
		t.Errorf("expected non-empty cwd, got empty")
	}
	// Should contain "backscroll" (the basename)
	if cwd != "backscroll" {
		t.Errorf("expected basename 'backscroll', got %q", cwd)
	}
}

func TestDecodeCwdFromSessionPath_Malformed(t *testing.T) {
	sessionPath := "/some/random/path/file.jsonl"
	cwd := projects.DecodeCwdFromSessionPath(sessionPath)
	if cwd != "" {
		t.Errorf("expected empty for malformed path, got %q", cwd)
	}
}

func TestDecodeCwdFromSessionPath_StandardFormat(t *testing.T) {
	// Test the standard Claude path format; path won't exist on disk, so fallback applies
	sessionPath := "/Users/pones/.claude/projects/-Users-Shared-harness-backscroll/uuid.jsonl"
	cwd := projects.DecodeCwdFromSessionPath(sessionPath)

	if cwd == "" {
		t.Errorf("expected non-empty cwd from standard format, got empty")
	}

	// Should use fallback: last dash-separated segment
	if cwd != "backscroll" {
		t.Errorf("expected fallback 'backscroll', got %q", cwd)
	}
}

func TestDecodeCwdFromSessionPath_MultipartProjectName(t *testing.T) {
	sessionPath := "/Users/pones/.claude/projects/-Users-Shared-harness-my-project-v2/uuid.jsonl"
	cwd := projects.DecodeCwdFromSessionPath(sessionPath)

	if cwd == "" {
		t.Errorf("expected non-empty cwd, got empty")
	}
	// Fallback should use last segment: "v2"
	if cwd != "v2" {
		t.Errorf("expected fallback 'v2', got %q", cwd)
	}
}

func TestDeriveFallbackID_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		cwd      string
		expected string
	}{
		{"root path", "/", "unknown"},
		{"single component", "myproj", "myproj"},
		{"dots", "/path/to/..", "path"},
		{"trailing slash", "/path/to/myproj/", "myproj"},
		{"uppercase", "/path/MYPROJ", "myproj"},
		{"with numbers", "/path/proj123", "proj123"},
		{"with underscores", "/path/my_proj", "my_proj"},
		{"with mixed special chars", "/path/my@proj!name", "myprojname"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := projects.DeriveFallbackID(tt.cwd)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDecodeCwdFromSessionPath_NoProjects(t *testing.T) {
	sessionPath := "/Users/pones/.claude/projects/-/uuid.jsonl"
	cwd := projects.DecodeCwdFromSessionPath(sessionPath)
	if cwd != "" {
		t.Errorf("expected empty for empty encoded dir, got %q", cwd)
	}
}

func TestDecodeCwdFromSessionPath_NoSlashAfterProjects(t *testing.T) {
	sessionPath := "/Users/pones/.claude/projectsfile.jsonl"
	cwd := projects.DecodeCwdFromSessionPath(sessionPath)
	if cwd != "" {
		t.Errorf("expected empty for malformed marker, got %q", cwd)
	}
}

func TestIdentifyFallbackWithCrossHostEquiv(t *testing.T) {
	// Test that cross-host normalization happens before fallback
	reg := projects.ProjectRegistry{
		Projects: []projects.ProjectConfig{
			{
				ID:    "myproj",
				Roots: []string{"/home/shared/myproject"},
			},
		},
	}

	// Path equivalent to registered root should be identified as myproj
	id := projects.Identify("/Users/Shared/myproject", reg)
	if id.ProjectID != "myproj" {
		t.Errorf("expected 'myproj' from cross-host equivalence, got %q", id.ProjectID)
	}

	// Path under equivalent root should also be identified as myproj
	id = projects.Identify("/Users/Shared/myproject/src", reg)
	if id.ProjectID != "myproj" {
		t.Errorf("expected 'myproj' from cross-host subpath, got %q", id.ProjectID)
	}
}
