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

	switch strings.ToLower(format) {
	case "csv", "markdown":
	default:
		return fmt.Errorf("unknown export format: %s (use 'markdown' or 'csv')", format)
	}

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
		_, _ = fmt.Fprintf(stdout, "No results found for: %s\n", query)
		return nil
	}

	// Export based on format
	switch strings.ToLower(format) {
	case "csv":
		return exportCSV(stdout, results)
	default:
		return exportMarkdown(stdout, results)
	}
}

func exportMarkdown(stdout io.Writer, results []storage.SearchResult) error {
	_, _ = fmt.Fprintf(stdout, "# Export Results\n\n")
	_, _ = fmt.Fprintf(stdout, "**Total: %d results**\n\n", len(results))

	for i, r := range results {
		_, _ = fmt.Fprintf(stdout, "## Result %d\n\n", i+1)
		_, _ = fmt.Fprintf(stdout, "**Source:** `%s` (%s)\n", r.SourcePath, r.Source)
		_, _ = fmt.Fprintf(stdout, "**Project:** %s\n", r.Project)
		_, _ = fmt.Fprintf(stdout, "**Role:** %s\n", r.Role)
		_, _ = fmt.Fprintf(stdout, "**Score:** %.2f\n", r.Score)
		_, _ = fmt.Fprintf(stdout, "**Timestamp:** %s\n\n", r.Timestamp.Format("2006-01-02 15:04:05 MST"))
		_, _ = fmt.Fprintf(stdout, "```\n%s\n```\n\n", r.Text)
	}

	return nil
}

func exportCSV(stdout io.Writer, results []storage.SearchResult) error {
	// Write header
	_, _ = fmt.Fprintf(stdout, "source_path,source,project,role,timestamp,score,content\n")

	// Write rows
	for _, r := range results {
		// Escape CSV fields
		sourcePath := escapeCSV(r.SourcePath)
		project := escapeCSV(r.Project)
		role := escapeCSV(r.Role)
		content := escapeCSV(r.Text)
		timestamp := r.Timestamp.Format("2006-01-02 15:04:05 MST")

		_, _ = fmt.Fprintf(stdout, "\"%s\",\"%s\",\"%s\",\"%s\",\"%s\",%.2f,\"%s\"\n",
			sourcePath, r.Source, project, role, timestamp, r.Score, content)
	}

	return nil
}

func escapeCSV(s string) string {
	// Escape double quotes by doubling them
	return strings.ReplaceAll(s, "\"", "\"\"")
}
