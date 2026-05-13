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
				Roots:            []string{"/home/shared/backscroll"},
				WorktreePatterns: []string{"/home/shared/backscroll/.worktrees/*"},
				Aliases:          []string{"bs"},
			},
		},
	}
}

func TestIdentifyExactRoot(t *testing.T) {
	reg := makeRegistry()
	id := projects.Identify("/home/shared/backscroll", reg)
	if id.ProjectID != "backscroll" {
		t.Errorf("expected backscroll, got %s", id.ProjectID)
	}
	if id.Confidence != projects.ConfidenceExact {
		t.Errorf("expected exact, got %s", id.Confidence)
	}
}

func TestIdentifySubpath(t *testing.T) {
	reg := makeRegistry()
	id := projects.Identify("/home/shared/backscroll/internal/config", reg)
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
				Roots:            []string{"/home/shared/backscroll"},
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
	reg := makeRegistry()
	id := projects.Identify("/some/other/path", reg)
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
