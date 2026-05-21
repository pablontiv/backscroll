package main

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	picokitoutput "github.com/pablontiv/picokit/output"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newResumeCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		robotFormat bool
		source      string
	)

	cmd := &cobra.Command{
		Use:   "resume <query>",
		Short: "Find the most recent session to resume work",
		Long: `Resume searches for the most relevant session and returns the most recent one,
formatted for resuming work on that topic.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResume(stdout, stderr, args[0], project, allProjects, robotFormat, source)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Search all projects")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")
	cmd.Flags().StringVar(&source, "source", "", "Filter by source type")

	return cmd
}

func runResume(stdout, stderr io.Writer,
	query, project string, allProjects, robotFormat bool, source string) error {

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Open read-only database
	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Search with higher limit to find the most recent
	opts := models.SearchOptions{
		Project:     project,
		AllProjects: allProjects,
		Source:      source,
		Limit:       100,
		Offset:      0,
	}

	// Execute search
	results, err := db.Search(query, opts)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		_, _ = fmt.Fprintf(stdout, "No relevant sessions found for: %s\n", query)
		return nil
	}

	// Group by source_path and get the most recent
	sessionMap := make(map[string]storage.SearchResult)
	for _, r := range results {
		if existing, ok := sessionMap[r.SourcePath]; !ok || r.Timestamp.After(existing.Timestamp) {
			sessionMap[r.SourcePath] = r
		}
	}

	// Find the most recent one
	var mostRecent storage.SearchResult
	var mostRecentTime time.Time
	for _, r := range sessionMap {
		if r.Timestamp.After(mostRecentTime) {
			mostRecentTime = r.Timestamp
			mostRecent = r
		}
	}

	// Convert to model result
	modelResult := models.SearchResult{
		Source:      mostRecent.Source,
		Role:        mostRecent.Role,
		Content:     mostRecent.Text,
		FilePath:    mostRecent.SourcePath,
		Timestamp:   mostRecent.Timestamp,
		ProjectPath: mostRecent.Project,
		Score:       mostRecent.Score,
		ContentType: mostRecent.ContentType,
		Rank:        1,
	}

	// Use robot format by default for resume
	format := picokitoutput.FormatRobot
	if !robotFormat {
		format = picokitoutput.FormatText
	}

	// Format and output
	formatter := picokitoutput.NewFormatter(format, 0)
	modelResults := []models.SearchResult{modelResult}
	lines := resultsToLines(modelResults, format)
	if err := formatter.WriteLines(stdout, lines); err != nil {
		return fmt.Errorf("write results: %w", err)
	}

	return nil
}
