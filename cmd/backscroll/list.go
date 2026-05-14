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
		recent      int
		jsonFormat  bool
		robotFormat bool
		indexedOnly bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all indexed sessions",
		Long: `List displays all indexed sessions, optionally filtered by project.

Use --project to filter to a single project.
Use --all-projects to list across all projects.
Use --recent N to show N most recent sessions.
Use --indexed-only to skip auto-sync (read existing index only).
Use --json to output as JSON.
Use --robot to output in robot format.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(stdout, stderr, project, allProjects, recent, jsonFormat, robotFormat, indexedOnly)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "List all projects")
	cmd.Flags().IntVar(&recent, "recent", 0, "Show N most recent sessions (0 = all)")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")

	return cmd
}

func runList(stdout, stderr io.Writer,
	project string, allProjects bool, recent int, jsonFormat, robotFormat, indexedOnly bool) error {

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Open read-only database (--indexed-only is default behavior for list).
	// If DB doesn't exist yet, return an empty list.
	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		if jsonFormat {
			_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"sessions\":[]}\n")
		} else {
			_, _ = fmt.Fprintf(stdout, "No sessions found\n")
		}
		return nil
	}
	defer func() { _ = db.Close() }()

	// List sessions
	sessions, err := db.ListSessions(project, recent)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		if jsonFormat {
			_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"sessions\":[]}\n")
		} else {
			_, _ = fmt.Fprintf(stdout, "No sessions found\n")
		}
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
		_, _ = fmt.Fprintf(stdout, "*** Sessions ***\n")
		for i, s := range sessions {
			_, _ = fmt.Fprintf(stdout, "Session %d: %s\n", i+1, s.Path)
			_, _ = fmt.Fprintf(stdout, "  Project: %s\n", s.Project)
			_, _ = fmt.Fprintf(stdout, "  Timestamp: %s\n", s.Timestamp.Format("2006-01-02 15:04:05 MST"))
			if len(s.Tags) > 0 {
				_, _ = fmt.Fprintf(stdout, "  Tags: %v\n", s.Tags)
			}
		}
		_, _ = fmt.Fprintf(stdout, "*** Total: %d sessions ***\n", len(sessions))
	} else {
		// Text output
		for i, s := range sessions {
			_, _ = fmt.Fprintf(stdout, "%d. %s\n", i+1, s.Path)
			_, _ = fmt.Fprintf(stdout, "   Project: %s, Timestamp: %s\n", s.Project, s.Timestamp.Format("2006-01-02 15:04:05 MST"))
			if len(s.Tags) > 0 {
				_, _ = fmt.Fprintf(stdout, "   Tags: %v\n", s.Tags)
			}
		}
		_, _ = fmt.Fprintf(stdout, "\nTotal: %d sessions\n", len(sessions))
	}

	return nil
}
