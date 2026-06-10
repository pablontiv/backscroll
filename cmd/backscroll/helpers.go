package main

import (
	"os"

	"github.com/pablontiv/backscroll/internal/projects"
)

// effectiveProject returns the canonical project ID for filtering purposes.
// It implements the following logic:
// - If allProjects is true, returns "" (no filter)
// - If project is explicitly set, returns that value
// - Otherwise derives project from current working directory via projects.Identify()
// - If derivation fails or project is unknown, returns ""
func effectiveProject(project string, allProjects bool) string {
	if allProjects {
		return ""
	}
	if project != "" {
		return project
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	registry := projects.LoadGlobalRegistry()
	result := projects.Identify(cwd, registry)
	if result.ProjectID == "unknown" {
		return ""
	}
	return result.ProjectID
}
