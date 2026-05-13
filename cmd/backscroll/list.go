package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newListCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		recent      bool
		jsonFormat  bool
		robotFormat bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all indexed sessions",
		Long: `List displays all indexed sessions, optionally filtered by project.

Use --project to filter to a single project.
Use --all-projects to list across all projects.
Use --recent to sort by most recent first.
Use --json to output as JSON.
Use --robot to output in robot format.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(stdout, stderr, project, allProjects, recent, jsonFormat, robotFormat)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "List all projects")
	cmd.Flags().BoolVar(&recent, "recent", false, "Sort by most recent first")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")

	return cmd
}

func runList(stdout, stderr io.Writer,
	project string, allProjects, recent, jsonFormat, robotFormat bool) error {

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Open read-only database
	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// List sessions
	sessions, err := db.ListSessions(project, recent)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Fprintf(stdout, "No sessions found\n")
		return nil
	}

	// Format output
	if jsonFormat {
		// JSON output
		data := map[string]interface{}{
			"count":    len(sessions),
			"sessions": sessions,
		}
		if err := json.NewEncoder(stdout).Encode(data); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else if robotFormat {
		// Robot format
		fmt.Fprintf(stdout, "*** Sessions ***\n")
		for i, s := range sessions {
			fmt.Fprintf(stdout, "Session %d: %s\n", i+1, s.Path)
			fmt.Fprintf(stdout, "  Project: %s\n", s.Project)
			fmt.Fprintf(stdout, "  Timestamp: %s\n", s.Timestamp.Format("2006-01-02 15:04:05 MST"))
			if len(s.Tags) > 0 {
				fmt.Fprintf(stdout, "  Tags: %v\n", s.Tags)
			}
		}
		fmt.Fprintf(stdout, "*** Total: %d sessions ***\n", len(sessions))
	} else {
		// Text output
		for i, s := range sessions {
			fmt.Fprintf(stdout, "%d. %s\n", i+1, s.Path)
			fmt.Fprintf(stdout, "   Project: %s, Timestamp: %s\n", s.Project, s.Timestamp.Format("2006-01-02 15:04:05 MST"))
			if len(s.Tags) > 0 {
				fmt.Fprintf(stdout, "   Tags: %v\n", s.Tags)
			}
		}
		fmt.Fprintf(stdout, "\nTotal: %d sessions\n", len(sessions))
	}

	return nil
}
