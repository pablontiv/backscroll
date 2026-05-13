package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newTopicsCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		limit       int
		jsonFormat  bool
		robotFormat bool
	)

	cmd := &cobra.Command{
		Use:   "topics",
		Short: "Show common topics across indexed content",
		Long: `Topics displays frequently occurring terms and concepts across all indexed sessions.

Use --project to filter to a single project.
Use --all-projects to analyze all projects.
Use --limit to control the number of topics (default: 50).
Use --json to output as JSON.
Use --robot to output in robot format.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTopics(stdout, stderr, project, allProjects, limit, jsonFormat, robotFormat)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Analyze all projects")
	cmd.Flags().IntVar(&limit, "limit", 50, "Number of topics to return")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")

	return cmd
}

func runTopics(stdout, stderr io.Writer,
	project string, allProjects bool, limit int, jsonFormat, robotFormat bool) error {

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

	// Get topics
	topics, err := db.GetTopics(project, limit)
	if err != nil {
		return fmt.Errorf("get topics: %w", err)
	}

	if len(topics) == 0 {
		fmt.Fprintf(stdout, "No topics found\n")
		return nil
	}

	// Format output
	if jsonFormat {
		// JSON output
		data := map[string]interface{}{
			"count":  len(topics),
			"topics": topics,
		}
		if err := json.NewEncoder(stdout).Encode(data); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else if robotFormat {
		// Robot format
		fmt.Fprintf(stdout, "*** Topics ***\n")
		for i, t := range topics {
			fmt.Fprintf(stdout, "%d. %s (%d)\n", i+1, t.Term, t.Count)
		}
		fmt.Fprintf(stdout, "*** Total: %d topics ***\n", len(topics))
	} else {
		// Text output
		fmt.Fprintf(stdout, "Common Topics:\n")
		for i, t := range topics {
			fmt.Fprintf(stdout, "%d. %s (%d occurrences)\n", i+1, t.Term, t.Count)
		}
		fmt.Fprintf(stdout, "\nTotal: %d topics\n", len(topics))
	}

	return nil
}
