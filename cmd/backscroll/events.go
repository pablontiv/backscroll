package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newEventsCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Query session events",
	}
	cmd.AddCommand(newEventsQueryCmd(stdout, stderr))
	return cmd
}

func newEventsQueryCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		jsonOut     bool
		jsonlOut    bool
		robot       bool
		project     string
		allProjects bool
		source      string
		sourcePath  string
		eventType   string
		role        string
		after       string
		before      string
		limit       int
		indexedOnly bool
	)

	cmd := &cobra.Command{
		Use:   "query [session-path-or-id]",
		Short: "Emit events from sessions in deterministic order",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionArg := ""
			if len(args) > 0 {
				sessionArg = args[0]
			}
			return runEventsQuery(stdout, stderr, sessionArg,
				jsonOut || jsonlOut, robot, project, allProjects, source, sourcePath, eventType, role, after, before, limit, indexedOnly)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSONL")
	cmd.Flags().BoolVar(&jsonlOut, "jsonl", false, "Output as JSONL (alias for --json)")
	cmd.Flags().BoolVar(&robot, "robot", false, "Output optimized for LLM (plain text)")
	cmd.Flags().StringVar(&project, "project", "", "Filter by project (default: derived from cwd)")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query all projects")
	cmd.Flags().StringVar(&source, "source", "session", "Filter by source type (session, plan, ke, etc.; 'all' = no filter)")
	cmd.Flags().StringVar(&sourcePath, "source-path", "", "Filter by source path (exact or * glob pattern)")
	cmd.Flags().StringVar(&eventType, "event-type", "", "Filter by event type (message, tool_call, tool_result, command, etc.)")
	cmd.Flags().StringVar(&role, "role", "", "Filter by role (user|assistant)")
	cmd.Flags().StringVar(&after, "after", "", "Filter events after date (ISO 8601)")
	cmd.Flags().StringVar(&before, "before", "", "Filter events before date (ISO 8601)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum events to return (0 = no limit)")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")

	return cmd
}

func runEventsQuery(stdout, stderr io.Writer, sessionArg string,
	jsonOut, robot bool, project string, allProjects bool,
	source, sourcePath, eventType, role, after, before string, limit int, indexedOnly bool) error {

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

	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Build the query
	q := storage.SessionEventQuery{
		Role:   role,
		After:  after,
		Before: before,
		Limit:  limit,
	}

	// Source filter: "all" → nil
	if source != "" && source != "all" {
		q.Source = &source
	}

	// EventType filter
	if eventType != "" {
		q.EventType = &eventType
	}

	// Project filter using canonical effectiveProject derivation
	if !allProjects {
		if project != "" {
			q.Project = &project
		} else if sessionArg == "" && sourcePath == "" {
			// Derive project from cwd only when no explicit session/path provided
			derived := effectiveProject("", allProjects)
			if derived != "" {
				q.Project = &derived
			}
		}
	}

	// SourcePath: positional arg takes precedence if provided
	if sessionArg != "" && sourcePath == "" {
		resolved, resolveErr := db.ResolveSessionPath(sessionArg)
		if resolveErr != nil || resolved == "" {
			_, _ = fmt.Fprintf(stderr, "warning: session not found: %s\n", sessionArg)
			q.SourcePath = sessionArg
		} else {
			q.SourcePath = resolved
		}
	} else if sourcePath != "" {
		q.SourcePath = sourcePath
	}

	events, err := db.QuerySessionEvents(q)
	if err != nil {
		return fmt.Errorf("query events: %w", err)
	}

	for _, e := range events {
		if jsonOut {
			data := buildEventJSON(e)
			if err := json.NewEncoder(stdout).Encode(data); err != nil {
				return fmt.Errorf("encode event: %w", err)
			}
		} else if robot {
			roleStr := ""
			if e.Role != nil {
				roleStr = *e.Role
			}
			_, _ = fmt.Fprintf(stdout, "[%s] %s\n", roleStr, truncate(e.Snippet, 500))
		} else {
			roleStr := ""
			if e.Role != nil {
				roleStr = *e.Role
			}
			ts := ""
			if e.Timestamp != nil {
				ts = *e.Timestamp
			}
			_, _ = fmt.Fprintf(stdout, "--- %s %s ---\n%s\n", roleStr, ts, e.Snippet)
		}
	}

	return nil
}

func buildEventJSON(e storage.SessionEvent) map[string]interface{} {
	m := map[string]interface{}{
		"id":         e.ID,
		"source":     e.Source,
		"sourcePath": e.SourcePath,
		"ordinal":    e.Ordinal,
		"eventType":  e.EventType,
		"snippet":    e.Snippet,
	}
	if e.Project != nil {
		m["project"] = *e.Project
	}
	if e.Timestamp != nil {
		m["timestamp"] = *e.Timestamp
	}
	if e.Role != nil {
		m["role"] = *e.Role
	}
	if e.Actor != nil {
		m["actor"] = *e.Actor
	}
	if e.ToolName != nil {
		m["toolName"] = *e.ToolName
	}
	if e.Command != nil {
		m["command"] = *e.Command
	}
	if e.ExitCode != nil {
		m["exitCode"] = *e.ExitCode
	}
	if e.IsError != nil {
		m["isError"] = *e.IsError
	}
	return m
}
