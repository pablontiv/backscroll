package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pablontiv/backscroll/internal/projects"
)

var currentWorkingDirectory = os.Getwd

// effectiveProject returns the canonical project ID for filtering purposes.
// Only an explicit all-projects scope may return an empty project filter.
func effectiveProject(project string, allProjects bool) (string, error) {
	if allProjects {
		return "", nil
	}
	if project != "" {
		return project, nil
	}
	cwd, err := currentWorkingDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve project from current working directory: %w; use --project NAME or --all-projects", err)
	}
	registry := projects.LoadGlobalRegistry()
	result := projects.Identify(cwd, registry)
	projectID := strings.TrimSpace(result.ProjectID)
	if projectID == "" {
		return "", fmt.Errorf("resolve project from current working directory %q: project ID is empty; use --project NAME or --all-projects", cwd)
	}
	if projectID == "unknown" {
		return "", fmt.Errorf("resolve project from current working directory %q: project is unknown; use --project NAME or --all-projects", cwd)
	}
	return result.ProjectID, nil
}
