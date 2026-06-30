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
		order       string
		limit       int
		offset      int
		eventType   string
		toolName    string
		command     string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all indexed sessions",
		Long: `List displays all indexed sessions, optionally filtered by project.

Use --project to filter to a single project.
Use --all-projects to list across all projects.
Use --order to sort results (e.g., timestamp:desc).
Use --limit to restrict result count.
Use --offset to skip results.
Use --type to filter by event type (e.g., tool_call) - queries structured events.
Use --tool to filter by tool name (e.g., bash, subagent) - requires --type.
Use --command to filter by command field - queries structured events.
Use --recent N to show N most recent sessions (legacy flag; prefer --order timestamp:desc --limit N).
Use --indexed-only to skip auto-sync (read existing index only).
Use --json to output as JSON.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected positional argument %q; use --text for text search", args[0])
			}
			return runList(stdout, stderr, project, allProjects, recent, jsonFormat, robotFormat, indexedOnly,
				order, limit, offset, eventType, toolName, command)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "List all projects")
	cmd.Flags().IntVar(&recent, "recent", 20, "Show N most recent sessions (0 = all)")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")
	cmd.Flags().StringVar(&order, "order", "", "Sort results (e.g., timestamp:desc)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Result limit (0 = no limit)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset")
	cmd.Flags().StringVar(&eventType, "type", "", "Filter by event type (e.g., tool_call)")
	cmd.Flags().StringVar(&toolName, "tool", "", "Filter by tool name (e.g., bash, subagent)")
	cmd.Flags().StringVar(&command, "command", "", "Filter by command")

	return cmd
}

func runList(stdout, stderr io.Writer,
	project string, allProjects bool, recent int, jsonFormat, robotFormat, indexedOnly bool,
	order string, limit, offset int, eventType, toolName, command string) error {

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

	// If structured query filters are provided (--type, --tool, --command), use ListSessionEventsV2
	if eventType != "" || toolName != "" || command != "" {
		opts := storage.ListOptions{
			Project:     project,
			AllProjects: allProjects,
			Order:       order,
			Limit:       limit,
			Offset:      offset,
			EventType:   eventType,
			ToolName:    toolName,
			Command:     command,
		}
		events, err := db.ListSessionEventsV2(opts)
		if err != nil {
			return fmt.Errorf("list session events v2: %w", err)
		}
		return formatStructuredEvents(stdout, events, jsonFormat)
	}

	// If v2 grammar flags are provided (input, order, limit, offset), use ListItemsV2
	// Otherwise fall back to legacy ListSessions for backward compat
	var sessions []storage.SessionEntry
	if order != "" || limit > 0 || offset > 0 {
		opts := storage.ListOptions{
			Project:     project,
			AllProjects: allProjects,
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

// formatStructuredEvents formats structured event rows for output.
func formatStructuredEvents(stdout io.Writer, events []storage.StructuredEventRow, jsonFormat bool) error {
	if len(events) == 0 {
		if jsonFormat {
			_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"events\":[]}\n")
		} else {
			_, _ = fmt.Fprintf(stdout, "No events found\n")
		}
		return nil
	}

	if jsonFormat {
		// JSON output
		data := map[string]interface{}{
			"count":  len(events),
			"events": events,
		}
		if err := json.NewEncoder(stdout).Encode(data); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else {
		// Text output
		for i, e := range events {
			_, _ = fmt.Fprintf(stdout, "%d. Event: %s\n", i+1, e.EventType)
			if e.ToolName != "" {
				_, _ = fmt.Fprintf(stdout, "   Tool: %s\n", e.ToolName)
			}
			if e.Actor != "" {
				_, _ = fmt.Fprintf(stdout, "   Actor: %s\n", e.Actor)
			}
			_, _ = fmt.Fprintf(stdout, "   Path: %s (ordinal: %d)\n", e.SourcePath, e.Ordinal)
			if e.Timestamp != "" {
				_, _ = fmt.Fprintf(stdout, "   Timestamp: %s\n", e.Timestamp)
			}
			_, _ = fmt.Fprintf(stdout, "   Snippet: %.80s\n", e.Snippet)
		}
		_, _ = fmt.Fprintf(stdout, "\nTotal: %d events\n", len(events))
	}

	return nil
}
