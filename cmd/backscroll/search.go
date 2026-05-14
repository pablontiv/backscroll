package main

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/output"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newSearchCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		jsonFormat  bool
		robotFormat bool
		source      string
		after       string
		before      string
		role        string
		limit       int
		offset      int
		contentType string
		tag         string
		fields      string
		maxTokens   int
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search indexed content",
		Long: `Search performs a hybrid search (BM25 + vector embeddings with RRF fusion)
across all indexed sessions, plans, and external sources.

Use --project to filter to a single project (default: auto-detect from cwd).
Use --all-projects to search across all projects.
Use --source to filter by source type (session, plan, ke, decision, memory, rule, spec, backlog).
Use --after/--before to filter by date (YYYY-MM-DD format).
Use --role to filter by message role (user, assistant).
Use --content-type to filter by content type (text, code, tool).
Use --tag to filter sessions by auto-detected tags.
Use --json to output as JSON.
Use --robot to output as robot format.
Use --max-tokens to limit output size (approximate token count).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(stdout, stderr, args[0],
				project, allProjects, jsonFormat, robotFormat,
				source, after, before, role, limit, offset, contentType, tag, maxTokens)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Search all projects")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")
	cmd.Flags().StringVar(&source, "source", "", "Filter by source type")
	cmd.Flags().StringVar(&after, "after", "", "Filter after date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&before, "before", "", "Filter before date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&role, "role", "", "Filter by message role")
	cmd.Flags().IntVar(&limit, "limit", 100, "Result limit")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Filter by content type")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter sessions by tag")
	cmd.Flags().StringVar(&fields, "fields", "", "Fields to display (comma-separated)")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", 0, "Max tokens in output (0=unlimited)")

	return cmd
}

func runSearch(stdout, stderr io.Writer,
	query string,
	project string, allProjects bool, jsonFormat, robotFormat bool,
	source, after, before, role string,
	limit, offset int, contentType, tag string, maxTokens int) error {

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

	// Parse dates
	var afterTime, beforeTime *time.Time
	if after != "" {
		t, err := time.Parse("2006-01-02", after)
		if err != nil {
			return fmt.Errorf("parse --after date: %w", err)
		}
		afterTime = &t
	}
	if before != "" {
		t, err := time.Parse("2006-01-02", before)
		if err != nil {
			return fmt.Errorf("parse --before date: %w", err)
		}
		beforeTime = &t
	}

	// Build search options
	opts := models.SearchOptions{
		Project:     project,
		AllProjects: allProjects,
		Source:      source,
		After:       afterTime,
		Before:      beforeTime,
		Role:        role,
		Limit:       limit,
		Offset:      offset,
		ContentType: contentType,
		Tag:         tag,
	}

	// Execute search
	results, err := db.Search(query, opts)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	// Convert storage.SearchResult to models.SearchResult
	var modelResults []models.SearchResult
	for i, r := range results {
		modelResults = append(modelResults, models.SearchResult{
			Source:      r.Source,
			Role:        r.Role,
			Content:     r.Text,
			FilePath:    r.SourcePath,
			Timestamp:   r.Timestamp,
			ProjectPath: r.Project,
			Score:       r.Score,
			ContentType: r.ContentType,
			Rank:        i + 1,
		})
	}

	// Determine output format
	format := output.FormatText
	if jsonFormat {
		format = output.FormatJSON
	} else if robotFormat {
		format = output.FormatRobot
	}

	// Format and output
	formatter := output.NewFormatter(format, maxTokens)
	if err := formatter.WriteResults(stdout, modelResults); err != nil {
		return fmt.Errorf("write results: %w", err)
	}

	return nil
}
