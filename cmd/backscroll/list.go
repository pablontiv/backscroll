package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newListCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		project     string
		allProjects bool
		recent      int
		jsonFormat  bool
		robotFormat bool
		order       string
		limit       int
		offset      int
	)

	cmd := &cobra.Command{
		Use:          "list",
		Short:        "List indexed sessions for the current project",
		SilenceUsage: true,
		Long: `List displays indexed sessions for the project inferred from the current working directory.

Use --project to select a single project explicitly.
Use --all-projects to list across all projects.
Use --order to sort results (e.g., timestamp:desc).
Use --limit to restrict result count (0 = unlimited); otherwise --recent defaults to 20.
Use --offset to skip results.
Use --recent N for the N newest sessions; it cannot be combined with --limit or ascending order.
Use --json to output as JSON.`,
		Args: func(cmd *cobra.Command, args []string) error {
			return validateCommandBeforeStartup(cmd, args, func(cmd *cobra.Command, args []string) error {
				if len(args) > 0 {
					return fmt.Errorf("unexpected positional argument %q; use --text for text search", args[0])
				}
				if recent < 0 || limit < 0 || offset < 0 {
					return fmt.Errorf("--recent, --limit and --offset must be >= 0")
				}
				if order != "timestamp:desc" && order != "timestamp:asc" {
					return fmt.Errorf("--order must be timestamp:desc or timestamp:asc")
				}
				if cmd.Flags().Changed("recent") && cmd.Flags().Changed("limit") {
					return fmt.Errorf("--recent and --limit cannot be set together")
				}
				if cmd.Flags().Changed("recent") && order == "timestamp:asc" {
					return fmt.Errorf("--recent requires timestamp:desc")
				}
				if !cmd.Flags().Changed("limit") {
					limit = recent
				}
				return nil
			}, func() error {
				if err := validateListNumbers(recent, limit, offset); err != nil {
					return err
				}
				resolved, err := effectiveProject(project, allProjects)
				if err == nil {
					project = resolved
				}
				return err
			})
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			startup := startupResultFrom(cmd)
			if startup.Config == nil {
				return fmt.Errorf("startup configuration unavailable")
			}
			return runList(cmd.Context(), stdout, stderr, startup.Config, project, allProjects, recent, invocationOutputMode(cmd),
				order, limit, offset)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Filter to project (mutually exclusive with --all-projects)")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "List sessions across all projects")
	cmd.Flags().IntVar(&recent, "recent", 20, "Show N most recent sessions (0 = all)")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")
	cmd.Flags().StringVar(&order, "order", "timestamp:desc", "Sort sessions: timestamp:desc or timestamp:asc")
	cmd.Flags().IntVar(&limit, "limit", 0, "Result limit (0 = no limit)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset")

	cmd.MarkFlagsMutuallyExclusive("project", "all-projects")
	describeOutputModes(cmd)
	return cmd
}

func runList(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config,
	project string, allProjects bool, recent int, mode outputMode,
	order string, limit, offset int) (retErr error) {

	project, err := effectiveProject(project, allProjects)
	if err != nil {
		return err
	}

	db, diag, err := prepareIndex(ctx, cfg, indexDataRead)
	if diag != nil {
		return refuseIndex(stdout, stderr, *diag, mode)
	}
	if err != nil {
		return fmt.Errorf("prepare index: %w", err)
	}
	defer func() { retErr = closeIndexDB(db, retErr) }()

	sessions, err := db.ListItemsV2(storage.ListOptions{
		Project: project, AllProjects: allProjects, Order: order, Limit: limit, Offset: offset,
	})
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		if mode == outputJSON {
			if err := writeOutput(stdout, "{\"count\":0,\"sessions\":[]}\n"); err != nil {
				return err
			}
		} else if mode == outputText {
			if err := writeOutput(stdout, "No sessions found\n"); err != nil {
				return err
			}
		}
		writeListEmptyHint(stderr, project, allProjects)
		return nil
	}

	// Format output
	if mode == outputJSON {
		// JSON output
		data := map[string]interface{}{
			"count":    len(sessions),
			"sessions": sessions,
		}
		if err := json.NewEncoder(stdout).Encode(data); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else if mode == outputRobot {
		// Robot format
		if err := writeOutput(stdout, "*** Sessions ***\n"); err != nil {
			return err
		}
		for i, s := range sessions {
			if err := writeOutput(stdout, "Session %d: %s\n", i+1, s.Path); err != nil {
				return err
			}
			if s.SessionID != "" {
				if err := writeOutput(stdout, "  Session ID: %s\n", s.SessionID); err != nil {
					return err
				}
			}
			if err := writeOutput(stdout, "  Project: %s\n", s.Project); err != nil {
				return err
			}
			if err := writeOutput(stdout, "  Timestamp: %s\n", s.Timestamp.Format("2006-01-02 15:04:05 MST")); err != nil {
				return err
			}
			if len(s.Tags) > 0 {
				if err := writeOutput(stdout, "  Tags: %v\n", s.Tags); err != nil {
					return err
				}
			}
		}
		if err := writeOutput(stdout, "*** Total: %d sessions ***\n", len(sessions)); err != nil {
			return err
		}
	} else {
		// Text output
		for i, s := range sessions {
			if err := writeOutput(stdout, "%d. %s\n", i+1, s.Path); err != nil {
				return err
			}
			if s.SessionID != "" {
				if err := writeOutput(stdout, "   Session ID: %s\n", s.SessionID); err != nil {
					return err
				}
			}
			if err := writeOutput(stdout, "   Project: %s, Timestamp: %s\n", s.Project, s.Timestamp.Format("2006-01-02 15:04:05 MST")); err != nil {
				return err
			}
			if len(s.Tags) > 0 {
				if err := writeOutput(stdout, "   Tags: %v\n", s.Tags); err != nil {
					return err
				}
			}
		}
		if err := writeOutput(stdout, "\nTotal: %d sessions\n", len(sessions)); err != nil {
			return err
		}
	}

	return nil
}

func writeListEmptyHint(w io.Writer, project string, allProjects bool) {
	if allProjects {
		fmt.Fprintln(w, "no sessions found across all projects")
		return
	}
	fmt.Fprintf(w, "no sessions found for project %q; use --all-projects to list sessions across all projects\n", project)
}
