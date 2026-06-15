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
		sourcePath          string
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
		text                string
		input               string
		indexedOnly         bool
	)

	cmd := &cobra.Command{
		Use:   "search [<query>]",
		Short: "Full-text search indexed content",
		Long: `Search performs a hybrid search (BM25 + vector embeddings with RRF fusion)
across all indexed sessions, plans, and external sources.

Use --text <query> (v2 preferred) or positional <query> (legacy) for search text.
Use --project to filter to a single project (default: auto-detect from cwd).
Use --all-projects to search across all projects.
Use --input to filter by configured input ID (v2 grammar).
Use --source to filter by source type (session, plan, ke, decision, memory, rule, spec, backlog).
Use --after/--before to filter by date (YYYY-MM-DD format).
Use --role to filter by message role (user, assistant).
Use --content-type to filter by content type (text, code, tool).
Use --tag to filter sessions by auto-detected tags.
Use --source-path to filter by indexed source path (exact, SQL LIKE pattern, or * glob).
Use --json to output as JSON.
Use --fields to choose JSON detail: minimal (default) or full.
Use --max-tokens to limit output size (approximate token count).
Use --indexed-only to skip auto-sync (read existing index only).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := text
			if query == "" && len(args) > 0 {
				query = args[0]
			}
			if query == "" {
				return fmt.Errorf("search query required (use --text <query> or positional argument)")
			}
			return runSearch(stdout, stderr, query,
				project, allProjects, jsonFormat, robotFormat,
				source, sourcePath, after, before, role, limit, offset, contentType, tag,
				fields, maxTokens, lexicalOnly, similarityThreshold, input, indexedOnly)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Search all projects")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")
	cmd.Flags().StringVar(&source, "source", "", "Filter by source type")
	cmd.Flags().StringVar(&sourcePath, "source-path", "", "Filter by source path (exact, SQL LIKE pattern, or * glob)")
	cmd.Flags().StringVar(&after, "after", "", "Filter after date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&before, "before", "", "Filter before date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&role, "role", "", "Filter by message role")
	cmd.Flags().IntVar(&limit, "limit", 20, "Result limit")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset")
	cmd.Flags().StringVar(&contentType, "content-type", "", "Filter by content type")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter sessions by tag")
	cmd.Flags().StringVar(&fields, "fields", "minimal", "JSON fields to emit: minimal or full")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", 0, "Max tokens in output (0=unlimited)")
	cmd.Flags().BoolVar(&lexicalOnly, "lexical-only", false, "Use BM25 only, skip vector search")
	cmd.Flags().Float64Var(&similarityThreshold, "similarity-threshold", 0.3, "Minimum cosine similarity for vector results (0=no threshold)")
	cmd.Flags().StringVar(&text, "text", "", "Search text (v2 preferred grammar)")
	cmd.Flags().StringVar(&input, "input", "", "Filter by input ID (v2 grammar)")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")

	return cmd
}

func runSearch(stdout, stderr io.Writer,
	query string,
	project string, allProjects bool, jsonFormat, robotFormat bool,
	source, sourcePath, after, before, role string,
	limit, offset int, contentType, tag string,
	fields string, maxTokens int,
	lexicalOnly bool, similarityThreshold float64, input string, indexedOnly bool) error {

	// Validate flag values before opening the database
	if fields != "minimal" && fields != "full" {
		return fmt.Errorf("invalid --fields value %q: must be minimal or full", fields)
	}

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

	// Map v2 --input flag to --source internally
	if input != "" && source == "" {
		source = input
	}

	// Build search options
	opts := models.SearchOptions{
		Project:             project,
		AllProjects:         allProjects,
		Source:              source,
		SourcePath:          sourcePath,
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
		// For JSON, --fields selects the payload: minimal (v0 default) or full struct
		if fields == "minimal" {
			minimal := make([]minimalSearchResult, len(results))
			for i, r := range results {
				minimal[i] = minimalSearchResult{
					SourcePath: r.SourcePath,
					Snippet:    r.Snippet,
					Score:      r.Score,
					Role:       r.Role,
					Timestamp:  r.Timestamp,
				}
			}
			if err := formatter.WriteJSON(stdout, minimal); err != nil {
				return fmt.Errorf("write results: %w", err)
			}
		} else if err := formatter.WriteJSON(stdout, modelResults); err != nil {
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

// minimalSearchResult is the reduced JSON payload emitted by --fields=minimal,
// matching the v0 minimal field set.
type minimalSearchResult struct {
	SourcePath string    `json:"source_path"`
	Snippet    string    `json:"snippet"`
	Score      float64   `json:"score"`
	Role       string    `json:"role"`
	Timestamp  time.Time `json:"timestamp"`
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
