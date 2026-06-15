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
		input       string
		order       string
		limit       int
		offset      int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all indexed sessions",
		Long: `List displays all indexed sessions, optionally filtered by project.

Use --project to filter to a single project.
Use --all-projects to list across all projects.
Use --input to filter by configured input ID (v2 grammar).
Use --order to sort results (e.g., timestamp:desc).
Use --limit to restrict result count.
Use --offset to skip results.
Use --recent N to show N most recent sessions (legacy flag; prefer --order timestamp:desc --limit N).
Use --indexed-only to skip auto-sync (read existing index only).
Use --json to output as JSON.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(stdout, stderr, project, allProjects, recent, jsonFormat, robotFormat, indexedOnly,
				input, order, limit, offset)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "List all projects")
	cmd.Flags().IntVar(&recent, "recent", 20, "Show N most recent sessions (0 = all)")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")
	cmd.Flags().StringVar(&input, "input", "", "Filter by input ID (v2 grammar)")
	cmd.Flags().StringVar(&order, "order", "", "Sort results (e.g., timestamp:desc)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Result limit (0 = no limit)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset")

	return cmd
}

func runList(stdout, stderr io.Writer,
	project string, allProjects bool, recent int, jsonFormat, robotFormat, indexedOnly bool,
	input, order string, limit, offset int) error {

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Auto-sync before query unless --indexed-only is set
	if !indexedOnly {
		if err := maybeAutoSync(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: auto-sync failed: %v; using cached index\n", err)
		}
	}

	// Derive effective project from cwd if not explicitly set
	project = effectiveProject(project, allProjects)

	// Open read-only database
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

	// If v2 grammar flags are provided (input, order, limit, offset), use ListItemsV2
	// Otherwise fall back to legacy ListSessions for backward compat
	var sessions []storage.SessionEntry
	if input != "" || order != "" || limit > 0 || offset > 0 {
		opts := storage.ListOptions{
			Project:     project,
			AllProjects: allProjects,
			Input:       input,
			Order:       order,
			Limit:       limit,
			Offset:      offset,
		}
		var err error
		sessions, err = db.ListItemsV2(opts)
		if err != nil {
			return fmt.Errorf("list items v2: %w", err)
		}
	} else {
		// Legacy path: use old ListSessions
		var err error
		sessions, err = db.ListSessions(project, recent)
		if err != nil {
			return fmt.Errorf("list sessions: %w", err)
		}
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
