package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newSessionsCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Inspect and query indexed sessions",
	}
	cmd.AddCommand(
		newSessionsListCmd(stdout, stderr),
		newSessionsValidateCmd(stdout, stderr),
		newSessionsQueryCmd(stdout, stderr),
	)
	return cmd
}

// newSessionsListCmd delegates to the same logic as the top-level list command.
func newSessionsListCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		recent      int
		jsonOut     bool
		robot       bool
		indexedOnly bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List indexed sessions (alias for backscroll list)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// v2 filters not used in legacy sessions list subcommand; pass empty/zero values
			return runList(stdout, stderr, project, allProjects, recent, jsonOut, robot, indexedOnly,
				"", "", 0, 0, "", "", "")
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Filter by project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Show sessions from all projects")
	cmd.Flags().IntVar(&recent, "recent", 20, "Show N most recent sessions (0 = all)")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robot, "robot", false, "Output optimized for LLM")
	return cmd
}

// newSessionsValidateCmd delegates to the same logic as the top-level validate command.
func newSessionsValidateCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate indexed sessions (alias for backscroll validate)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(stdout, stderr, false)
		},
	}
	return cmd
}

// newSessionsQueryCmd queries sessions by metadata filters (after/before/source/project).
func newSessionsQueryCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		after       string
		before      string
		source      string
		sourcePath  string
		maxChars    int
		jsonOut     bool
		jsonlOut    bool
		limit       int
		indexedOnly bool
	)

	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query indexed sessions by metadata filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionsQuery(stdout, stderr, project, allProjects, after, before, source, sourcePath, jsonOut || jsonlOut, limit, maxChars, indexedOnly)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Filter by project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query sessions from all projects")
	cmd.Flags().StringVar(&after, "after", "", "Filter sessions after date (ISO 8601)")
	cmd.Flags().StringVar(&before, "before", "", "Filter sessions before date (ISO 8601)")
	cmd.Flags().StringVar(&source, "source", "session", "Source type to query")
	cmd.Flags().StringVar(&sourcePath, "source-path", "", "Filter by source path (exact or * glob pattern)")
	cmd.Flags().IntVar(&maxChars, "max-chars", 2000, "Maximum text characters per record (0 = no limit)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&jsonlOut, "jsonl", false, "Output as JSONL (alias for --json)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum sessions to return")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")
	return cmd
}

func runSessionsQuery(stdout, stderr io.Writer, project string, allProjects bool, after, before, source, sourcePath string, jsonOut bool, limit, maxChars int, indexedOnly bool) error {
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

	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		return nil
	}
	defer func() { _ = db.Close() }()

	var proj *string
	if project != "" && !allProjects {
		proj = &project
	}
	var afterPtr, beforePtr, sourcePtr *string
	if after != "" {
		afterPtr = &after
	}
	if before != "" {
		beforePtr = &before
	}
	normalizedSource := source
	if normalizedSource == "sessions" {
		normalizedSource = "session"
	}
	if normalizedSource != "" {
		sourcePtr = &normalizedSource
	}

	var spPtr *string
	if sourcePath != "" {
		spPtr = &sourcePath
	}

	records, err := db.QueryIndexedRecords(storage.IndexedRecordQuery{
		Project:    proj,
		Source:     sourcePtr,
		SourcePath: spPtr,
		After:      afterPtr,
		Before:     beforePtr,
		Limit:      limit,
		MaxChars:   maxChars,
	})
	if err != nil {
		return fmt.Errorf("query sessions: %w", err)
	}

	seen := map[string]bool{}
	for _, r := range records {
		if seen[r.SourcePath] {
			continue
		}
		seen[r.SourcePath] = true

		if jsonOut {
			data := map[string]interface{}{
				"source_path": r.SourcePath,
				"source":      r.Source,
			}
			if r.Project != nil {
				data["project"] = *r.Project
			}
			if r.Timestamp != nil {
				data["timestamp"] = *r.Timestamp
			}
			if encErr := encodeJSON(stdout, data); encErr != nil {
				return encErr
			}
		} else {
			ts := ""
			if r.Timestamp != nil {
				ts = *r.Timestamp
			}
			proj := ""
			if r.Project != nil {
				proj = *r.Project
			}
			_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", r.SourcePath, proj, ts)
		}
	}

	return nil
}

func encodeJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
