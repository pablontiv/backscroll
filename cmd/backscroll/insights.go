package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newInsightsCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		jsonFormat  bool
		robotFormat bool
	)

	cmd := &cobra.Command{
		Use:   "insights",
		Short: "Show indexing insights and statistics",
		Long: `Insights displays overall statistics about indexed sessions and content.

Shows counts of indexed files, messages, and timestamp of last indexing.

Use --project to filter to a single project.
Use --all-projects to analyze all projects.
Use --json to output as JSON.
Use --robot to output in robot format.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInsights(stdout, stderr, project, allProjects, jsonFormat, robotFormat)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Analyze all projects")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")

	return cmd
}

func runInsights(stdout, stderr io.Writer,
	project string, allProjects bool, jsonFormat, robotFormat bool) error {

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Auto-sync before query
	if err := maybeAutoSync(cfg); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: auto-sync failed: %v; using cached index\n", err)
	}

	// Derive effective project from cwd if not explicitly set
	project = effectiveProject(project, allProjects)

	// Open read-only database
	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Get stats
	stats, err := db.GetStats()
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}

	// Get sessions for count
	sessions, err := db.ListSessions(project, 0)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	// Format output
	if jsonFormat {
		// JSON output
		data := map[string]interface{}{
			"total_files":    stats.TotalFiles,
			"total_messages": stats.TotalMessages,
			"total_sessions": len(sessions),
			"indexed_at":     stats.IndexedAt,
		}
		if err := json.NewEncoder(stdout).Encode(data); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else if robotFormat {
		// Robot format
		_, _ = fmt.Fprintf(stdout, "*** Insights ***\n")
		_, _ = fmt.Fprintf(stdout, "Total Files: %d\n", stats.TotalFiles)
		_, _ = fmt.Fprintf(stdout, "Total Messages: %d\n", stats.TotalMessages)
		_, _ = fmt.Fprintf(stdout, "Total Sessions: %d\n", len(sessions))
		if !stats.IndexedAt.IsZero() {
			_, _ = fmt.Fprintf(stdout, "Last Indexed: %s\n", stats.IndexedAt.Format("2006-01-02 15:04:05 MST"))
		}
		_, _ = fmt.Fprintf(stdout, "*** End Insights ***\n")
	} else {
		// Text output
		_, _ = fmt.Fprintf(stdout, "Indexing Insights:\n")
		_, _ = fmt.Fprintf(stdout, "  Total files:    %d\n", stats.TotalFiles)
		_, _ = fmt.Fprintf(stdout, "  Total messages: %d\n", stats.TotalMessages)
		_, _ = fmt.Fprintf(stdout, "  Total sessions: %d\n", len(sessions))
		if !stats.IndexedAt.IsZero() {
			_, _ = fmt.Fprintf(stdout, "  Last indexed:   %s\n", stats.IndexedAt.Format("2006-01-02 15:04:05 MST"))
		}
	}

	return nil
}
