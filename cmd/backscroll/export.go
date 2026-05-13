package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newExportCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		format      string
		project     string
		allProjects bool
	)

	cmd := &cobra.Command{
		Use:   "export <query>",
		Short: "Export search results to markdown or CSV",
		Long: `Export searches for results and exports them in the specified format.

Use --format to specify export format: markdown (default) or csv.
Use --project to filter to a single project.
Use --all-projects to search across all projects.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(stdout, stderr, args[0], format, project, allProjects)
		},
	}

	cmd.Flags().StringVar(&format, "format", "markdown", "Export format (markdown|csv)")
	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Search all projects")

	return cmd
}

func runExport(stdout, stderr io.Writer,
	query, format, project string, allProjects bool) error {

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

	// Search
	opts := models.SearchOptions{
		Project:     project,
		AllProjects: allProjects,
		Limit:       1000,
		Offset:      0,
	}

	results, err := db.Search(query, opts)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintf(stdout, "No results found for: %s\n", query)
		return nil
	}

	// Export based on format
	switch strings.ToLower(format) {
	case "csv":
		return exportCSV(stdout, results)
	case "markdown":
		return exportMarkdown(stdout, results)
	default:
		return fmt.Errorf("unknown export format: %s (use 'markdown' or 'csv')", format)
	}
}

func exportMarkdown(stdout io.Writer, results []storage.SearchResult) error {
	fmt.Fprintf(stdout, "# Export Results\n\n")
	fmt.Fprintf(stdout, "**Total: %d results**\n\n", len(results))

	for i, r := range results {
		fmt.Fprintf(stdout, "## Result %d\n\n", i+1)
		fmt.Fprintf(stdout, "**Source:** `%s` (%s)\n", r.SourcePath, r.Source)
		fmt.Fprintf(stdout, "**Project:** %s\n", r.Project)
		fmt.Fprintf(stdout, "**Role:** %s\n", r.Role)
		fmt.Fprintf(stdout, "**Score:** %.2f\n", r.Score)
		fmt.Fprintf(stdout, "**Timestamp:** %s\n\n", r.Timestamp.Format("2006-01-02 15:04:05 MST"))
		fmt.Fprintf(stdout, "```\n%s\n```\n\n", r.Text)
	}

	return nil
}

func exportCSV(stdout io.Writer, results []storage.SearchResult) error {
	// Write header
	fmt.Fprintf(stdout, "source_path,source,project,role,timestamp,score,content\n")

	// Write rows
	for _, r := range results {
		// Escape CSV fields
		sourcePath := escapeCSV(r.SourcePath)
		project := escapeCSV(r.Project)
		role := escapeCSV(r.Role)
		content := escapeCSV(r.Text)
		timestamp := r.Timestamp.Format("2006-01-02 15:04:05 MST")

		fmt.Fprintf(stdout, "\"%s\",\"%s\",\"%s\",\"%s\",\"%s\",%.2f,\"%s\"\n",
			sourcePath, r.Source, project, role, timestamp, r.Score, content)
	}

	return nil
}

func escapeCSV(s string) string {
	// Escape double quotes by doubling them
	return strings.ReplaceAll(s, "\"", "\"\"")
}
