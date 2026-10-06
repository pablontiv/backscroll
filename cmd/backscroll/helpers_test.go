package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEffectiveProjectAllProjects tests that --all-projects returns empty string.
func TestEffectiveProjectAllProjects(t *testing.T) {
	for _, project := range []string{"anyproject", ""} {
		result, err := effectiveProject(project, true)
		if err != nil {
			t.Fatalf("effectiveProject(%q, true): %v", project, err)
		}
		if result != "" {
			t.Errorf("effectiveProject(%q, true) = %q, want empty", project, result)
		}
	}
}

// TestEffectiveProjectExplicitProject tests that explicit --project is retained.
func TestEffectiveProjectExplicitProject(t *testing.T) {
	result, err := effectiveProject("myproject", false)
	if err != nil {
		t.Fatalf("effectiveProject: %v", err)
	}
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
	result, err := effectiveProject("", false)
	if err != nil {
		t.Fatalf("effectiveProject: %v", err)
	}
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

	// When cwd is not in the registry, Identify() now returns a fallback ID from the basename
	// So effectiveProject will return that fallback ID instead of empty string
	result, err := effectiveProject("", false)
	if err != nil {
		t.Fatalf("effectiveProject: %v", err)
	}
	if result == "" {
		t.Error("effectiveProject should return fallback ID from cwd basename, got empty string")
	}
	// The result should be derived from the temp directory basename (which is not "unknown")
	if result == "unknown" {
		t.Errorf("effectiveProject should return fallback ID, not 'unknown', got %q", result)
	}
}

func TestEffectiveProjectDeletedCwdFailsClosed(t *testing.T) {
	originalGetwd := currentWorkingDirectory
	currentWorkingDirectory = func() (string, error) { return "", errors.New("current directory was removed") }
	t.Cleanup(func() { currentWorkingDirectory = originalGetwd })

	assertProjectResolutionError(t)
}

func TestEffectiveProjectUnknownIdentityFailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	originalGetwd := currentWorkingDirectory
	currentWorkingDirectory = func() (string, error) { return filepath.Join(string(filepath.Separator), "💥"), nil }
	t.Cleanup(func() { currentWorkingDirectory = originalGetwd })

	assertProjectResolutionError(t)
}

func assertProjectResolutionError(t *testing.T) {
	t.Helper()
	project, err := effectiveProject("", false)
	if err == nil {
		t.Fatalf("effectiveProject = %q, nil; want resolution error", project)
	}
	for _, want := range []string{"--project NAME", "--all-projects"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing guidance %q", err, want)
		}
	}
}
