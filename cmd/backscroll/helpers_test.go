package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEffectiveProjectAllProjects tests that --all-projects returns empty string
func TestEffectiveProjectAllProjects(t *testing.T) {
	// When allProjects=true, result should be empty regardless of project or cwd
	result := effectiveProject("anyproject", true)
	if result != "" {
		t.Errorf("effectiveProject with allProjects=true should return empty string, got %q", result)
	}

	result = effectiveProject("", true)
	if result != "" {
		t.Errorf("effectiveProject with allProjects=true should return empty string, got %q", result)
	}
}

// TestEffectiveProjectExplicitProject tests that explicit --project flag takes precedence
func TestEffectiveProjectExplicitProject(t *testing.T) {
	result := effectiveProject("myproject", false)
	if result != "myproject" {
		t.Errorf("effectiveProject with explicit project should return that project, got %q", result)
	}
}

// TestEffectiveProjectFromCwd tests that cwd-based project derivation works
func TestEffectiveProjectFromCwd(t *testing.T) {
	// Create a temporary home directory with a test registry
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create a minimal projects.toml in the config directory
	configDir := filepath.Join(tmpHome, ".config", "backscroll")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create the project directory
	projDir := filepath.Join(tmpHome, "projects", "testproj")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Create projects.toml with a test project that uses exact root match
	projectsPath := filepath.Join(configDir, "projects.toml")
	projectsContent := `
[[projects]]
id = "testproj"
roots = ["` + projDir + `"]

[[projects]]
id = "other"
roots = ["/tmp/other"]
`
	if err := os.WriteFile(projectsPath, []byte(projectsContent), 0644); err != nil {
		t.Fatalf("failed to write projects.toml: %v", err)
	}

	// Change to the project directory
	t.Chdir(projDir)

	// Test that derivation returns the correct project ID
	result := effectiveProject("", false)
	if result != "testproj" {
		t.Errorf("effectiveProject from cwd should return testproj, got %q", result)
	}
}

// TestEffectiveProjectUnknownCwd tests that unknown cwd returns empty string
func TestEffectiveProjectUnknownCwd(t *testing.T) {
	// Create a temporary home with empty config (no projects.toml)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create a temporary directory that is NOT in the registry
	tmpDir := t.TempDir()

	t.Chdir(tmpDir)

	// When cwd is not in the registry, should return empty string
	result := effectiveProject("", false)
	if result != "" {
		t.Errorf("effectiveProject with unknown cwd should return empty string, got %q", result)
	}
}
