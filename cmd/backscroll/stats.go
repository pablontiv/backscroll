package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newStatsCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		input       string
		eventType   string
		toolName    string
		groupBy     string
		jsonFormat  bool
		indexedOnly bool
	)

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Aggregate statistics over indexed events",
		Long: `Stats computes aggregate counts over the configured input corpus.

Use --input to filter by input ID.
Use --type to filter by event type (e.g., tool_call).
Use --tool to filter by tool name (e.g., bash, subagent).
Use --group-by to group results (e.g., agent, tool, type).
Use --project to filter to a single project.
Use --all-projects to include all projects.
Use --indexed-only to skip auto-sync.
Use --json to output as JSON.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStats(stdout, stderr, project, allProjects, input, eventType, toolName, groupBy, jsonFormat, indexedOnly)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Include all projects")
	cmd.Flags().StringVar(&input, "input", "", "Filter by input ID")
	cmd.Flags().StringVar(&eventType, "type", "", "Filter by event type (e.g., tool_call)")
	cmd.Flags().StringVar(&toolName, "tool", "", "Filter by tool name")
	cmd.Flags().StringVar(&groupBy, "group-by", "", "Group results by field (e.g., agent, tool, type)")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")

	return cmd
}

func runStats(stdout, stderr io.Writer,
	project string, allProjects bool, input, eventType, toolName, groupBy string,
	jsonFormat, indexedOnly bool) error {

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
	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		if jsonFormat {
			_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"stats\":[]}\n")
		} else {
			_, _ = fmt.Fprintf(stdout, "No data found\n")
		}
		return nil
	}
	defer func() { _ = db.Close() }()

	// Query session_events with the provided filters
	opts := storage.ListOptions{
		Project:     project,
		AllProjects: allProjects,
		Input:       input,
		EventType:   eventType,
		ToolName:    toolName,
		Limit:       0, // No limit for aggregation
	}

	events, err := db.ListSessionEventsV2(opts)
	if err != nil {
		return fmt.Errorf("query events: %w", err)
	}

	// Group the results by the specified dimension
	stats := groupEvents(events, groupBy)

	// Format output
	if jsonFormat {
		data := map[string]interface{}{
			"count": len(stats),
			"stats": stats,
		}
		if err := json.NewEncoder(stdout).Encode(data); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else {
		// Text output
		if len(stats) == 0 {
			_, _ = fmt.Fprintf(stdout, "No data found\n")
			return nil
		}

		for _, s := range stats {
			_, _ = fmt.Fprintf(stdout, "%s: %d\n", s.Name, s.Count)
		}
		_, _ = fmt.Fprintf(stdout, "\nTotal: %d groups\n", len(stats))
	}

	return nil
}

// StatEntry represents a single aggregation result.
type StatEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// groupEvents groups events by the specified dimension.
func groupEvents(events []storage.StructuredEventRow, groupBy string) []StatEntry {
	counts := make(map[string]int)

	switch groupBy {
	case "agent":
		// Group by actor field; use <unknown> for empty values
		for _, e := range events {
			key := e.Actor
			if key == "" {
				key = "<unknown>"
			}
			counts[key]++
		}
	case "tool":
		// Group by tool_name field
		for _, e := range events {
			key := e.ToolName
			if key == "" {
				key = "<unknown>"
			}
			counts[key]++
		}
	case "type":
		// Group by event_type field
		for _, e := range events {
			key := e.EventType
			if key == "" {
				key = "<unknown>"
			}
			counts[key]++
		}
	case "project":
		// Group by project field
		for _, e := range events {
			key := ""
			if e.Project != nil {
				key = *e.Project
			}
			if key == "" {
				key = "<unknown>"
			}
			counts[key]++
		}
	default:
		// Default to grouping by tool
		for _, e := range events {
			key := e.ToolName
			if key == "" {
				key = "<unknown>"
			}
			counts[key]++
		}
	}

	// Convert map to sorted list
	var result []StatEntry
	for name, count := range counts {
		result = append(result, StatEntry{Name: name, Count: count})
	}

	// Sort by name for consistent output
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result
}
