package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"io"

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
		jsonOut bool
		robot   bool
		role    string
		after   string
		before  string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "query <session-path-or-id>",
		Short: "Emit events from a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEventsQuery(stdout, stderr, args[0], jsonOut, robot, role, after, before, limit)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSONL")
	cmd.Flags().BoolVar(&robot, "robot", false, "Output optimized for LLM (plain text)")
	cmd.Flags().StringVar(&role, "role", "", "Filter by role (user|assistant)")
	cmd.Flags().StringVar(&after, "after", "", "Filter events after date (ISO 8601)")
	cmd.Flags().StringVar(&before, "before", "", "Filter events before date (ISO 8601)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum events to return")

	return cmd
}

func runEventsQuery(stdout, stderr io.Writer, sessionArg string, jsonOut, robot bool, role, after, before string, limit int) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Resolve session identifier to source_path
	sourcePath, err := db.ResolveSessionPath(sessionArg)
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}
	if sourcePath == "" {
		_, _ = fmt.Fprintf(stderr, "warning: session not found: %s\n", sessionArg)
		sourcePath = sessionArg
	}

	events, err := db.QuerySessionEvents(storage.SessionEventQuery{
		SourcePath: sourcePath,
		Role:       role,
		After:      after,
		Before:     before,
		Limit:      limit,
	})
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
