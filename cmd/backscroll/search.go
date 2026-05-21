package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	picokitoutput "github.com/pablontiv/picokit/output"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newSearchCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project             string
		allProjects         bool
		jsonFormat          bool
		robotFormat         bool
		source              string
		after               string
		before              string
		role                string
		limit               int
		offset              int
		contentType         string
		tag                 string
		fields              string
		maxTokens           int
		lexicalOnly         bool
		similarityThreshold float64
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
				source, after, before, role, limit, offset, contentType, tag, maxTokens,
				lexicalOnly, similarityThreshold)
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
	cmd.Flags().BoolVar(&lexicalOnly, "lexical-only", false, "Use BM25 only, skip vector search")
	cmd.Flags().Float64Var(&similarityThreshold, "similarity-threshold", 0, "Minimum cosine similarity for vector results (0=no threshold)")

	return cmd
}

func runSearch(stdout, stderr io.Writer,
	query string,
	project string, allProjects bool, jsonFormat, robotFormat bool,
	source, after, before, role string,
	limit, offset int, contentType, tag string, maxTokens int,
	lexicalOnly bool, similarityThreshold float64) error {

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
		Project:             project,
		AllProjects:         allProjects,
		Source:              source,
		After:               afterTime,
		Before:              beforeTime,
		Role:                role,
		Limit:               limit,
		Offset:              offset,
		ContentType:         contentType,
		Tag:                 tag,
		LexicalOnly:         lexicalOnly,
		SimilarityThreshold: similarityThreshold,
	}

	// Execute search — HybridSearch falls back to BM25 when no provider/vectors
	results, err := db.HybridSearch(query, opts)
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
	format := picokitoutput.FormatText
	if jsonFormat {
		format = picokitoutput.FormatJSON
	} else if robotFormat {
		format = picokitoutput.FormatRobot
	}

	// Format and output
	formatter := picokitoutput.NewFormatter(format, maxTokens)
	if format == picokitoutput.FormatJSON {
		// For JSON, use WriteJSON directly
		if err := formatter.WriteJSON(stdout, modelResults); err != nil {
			return fmt.Errorf("write results: %w", err)
		}
	} else {
		// For text and robot formats, convert results to lines
		lines := resultsToLines(modelResults, format)
		if err := formatter.WriteLines(stdout, lines); err != nil {
			return fmt.Errorf("write results: %w", err)
		}
	}

	return nil
}

// resultsToLines converts SearchResults to string lines for the specified format.
// For text format, produces the separator + fields output.
// For robot format, produces result_N_field=value lines.
func resultsToLines(results []models.SearchResult, format picokitoutput.Format) []string {
	var lines []string

	for i, result := range results {
		if format == picokitoutput.FormatRobot {
			// Robot format: result_N_field=value
			lines = append(lines,
				fmt.Sprintf("result_%d_source=%s", i, result.Source),
				fmt.Sprintf("result_%d_role=%s", i, result.Role),
				fmt.Sprintf("result_%d_filepath=%s", i, result.FilePath),
				fmt.Sprintf("result_%d_content=%s", i, result.Content),
			)
			if result.SessionID != "" {
				lines = append(lines, fmt.Sprintf("result_%d_session_id=%s", i, result.SessionID))
			}
			if result.ProjectPath != "" {
				lines = append(lines, fmt.Sprintf("result_%d_project=%s", i, result.ProjectPath))
			}
			lines = append(lines, fmt.Sprintf("result_%d_score=%.2f", i, result.Score))
			if len(result.Tags) > 0 {
				lines = append(lines, fmt.Sprintf("result_%d_tags=%s", i, strings.Join(result.Tags, ",")))
			}
			lines = append(lines, fmt.Sprintf("result_%d_rank=%d", i, result.Rank))
		} else {
			// Text format: separator + fields
			lines = append(lines, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			lines = append(lines, fmt.Sprintf("Rank: %d | Source: %s | Role: %s | Score: %.2f", result.Rank, result.Source, result.Role, result.Score))
			lines = append(lines, fmt.Sprintf("Path: %s", result.FilePath))
			if !result.Timestamp.IsZero() {
				lines = append(lines, fmt.Sprintf("Time: %s", result.Timestamp.Format("2006-01-02 15:04:05")))
			}
			if result.SessionID != "" {
				lines = append(lines, fmt.Sprintf("Session: %s", result.SessionID))
			}
			if result.ProjectPath != "" {
				lines = append(lines, fmt.Sprintf("Project: %s", result.ProjectPath))
			}
			if len(result.Tags) > 0 {
				lines = append(lines, fmt.Sprintf("Tags: %s", strings.Join(result.Tags, ", ")))
			}
			lines = append(lines, result.Content)
		}
	}

	return lines
}
