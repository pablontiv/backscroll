package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func newPatternsCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		kind        string
		project     string
		allProjects bool
		tag         string
		limit       int
		offset      int
		jsonFormat  bool
		robotFormat bool
		indexedOnly bool
	)

	cmd := &cobra.Command{
		Use:   "patterns",
		Short: "Discover deterministic patterns in tool events",
		Long: `Patterns computes census aggregations over tool events (commands, failures)
to expose actionable pattern candidates for agent loop classification.

Use --kind to select aggregation type (required; supported: commands, failures).
Use --project to filter to a single project.
Use --tag to filter by session tags (e.g., debugging, testing).
Use --limit, --offset for pagination.
Use --json, --robot for output formats.
Use --indexed-only to skip auto-sync (read existing index only).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected positional argument %q", args[0])
			}
			return runPatterns(stdout, stderr, kind, project, allProjects, tag, limit, offset, jsonFormat, robotFormat, indexedOnly)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Aggregation kind (required: commands|failures)")
	cmd.Flags().StringVar(&project, "project", "", "Filter to single project")
	cmd.Flags().BoolVar(&allProjects, "all-projects", false, "Query across all projects (default if --project not set)")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by session tag")
	cmd.Flags().IntVar(&limit, "limit", 20, "Result limit")
	cmd.Flags().IntVar(&offset, "offset", 0, "Result offset")
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&robotFormat, "robot", false, "Output in robot format")
	cmd.Flags().BoolVar(&indexedOnly, "indexed-only", false, "Read existing index without auto-sync")

	cmd.MarkFlagRequired("kind")

	return cmd
}

func runPatterns(stdout, stderr io.Writer,
	kind string, project string, allProjects bool, tag string,
	limit, offset int, jsonFormat, robotFormat, indexedOnly bool) error {

	// Early flag validation before DB open
	validKinds := map[string]bool{
		"commands": true,
		"failures": true,
	}
	if !validKinds[kind] {
		return fmt.Errorf("unsupported --kind %q (supported: commands, failures)", kind)
	}

	if project != "" && allProjects {
		return fmt.Errorf("--project and --all-projects are mutually exclusive")
	}

	if limit < 0 || offset < 0 {
		return fmt.Errorf("--limit and --offset must be >= 0")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Auto-sync unless --indexed-only
	if !indexedOnly {
		if err := maybeAutoSync(cfg); err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: auto-sync failed: %v; using cached index\n", err)
		}
	}

	// Derive effective project
	if project == "" && !allProjects {
		project = effectiveProject(project, allProjects)
	}

	db, err := storage.OpenReadOnly(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	opts := storage.AggregateOptions{
		Project: project,
		Tag:     tag,
		Limit:   limit,
		Offset:  offset,
	}

	// Execute aggregation
	if kind == "commands" {
		results, err := db.AggregateCommands(opts)
		if err != nil {
			return fmt.Errorf("aggregate commands: %w", err)
		}

		if len(results) == 0 {
			if !jsonFormat && !robotFormat {
				_, _ = fmt.Fprintf(stdout, "No patterns found.\n")
			} else if jsonFormat {
				_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"patterns\":[]}\n")
			}
			writeSearchHints(stderr, allProjects, true)
			return nil
		}

		if jsonFormat {
			data := map[string]interface{}{
				"count":    len(results),
				"kind":     "commands",
				"patterns": results,
			}
			if err := json.NewEncoder(stdout).Encode(data); err != nil {
				return fmt.Errorf("encode JSON: %w", err)
			}
		} else if robotFormat {
			_, _ = fmt.Fprintf(stdout, "*** Commands ***\n")
			for i, p := range results {
				_, _ = fmt.Fprintf(stdout, "result_%d_tool_name=%s\n", i, p.ToolName)
				_, _ = fmt.Fprintf(stdout, "result_%d_command_head=%s\n", i, p.CommandHead)
				_, _ = fmt.Fprintf(stdout, "result_%d_count=%d\n", i, p.Count)
			}
			_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(results))
		} else {
			_, _ = fmt.Fprintf(stdout, "Top Commands\n")
			_, _ = fmt.Fprintf(stdout, "============\n\n")
			for i, p := range results {
				_, _ = fmt.Fprintf(stdout, "%d. %s %s (%d times)\n", i+1, p.ToolName, p.CommandHead, p.Count)
			}
		}

	} else if kind == "failures" {
		results, err := db.AggregateFailures(opts)
		if err != nil {
			return fmt.Errorf("aggregate failures: %w", err)
		}

		if len(results) == 0 {
			if !jsonFormat && !robotFormat {
				_, _ = fmt.Fprintf(stdout, "No failure patterns found.\n")
			} else if jsonFormat {
				_, _ = fmt.Fprintf(stdout, "{\"count\":0,\"patterns\":[]}\n")
			}
			writeSearchHints(stderr, allProjects, true)
			return nil
		}

		if jsonFormat {
			data := map[string]interface{}{
				"count":    len(results),
				"kind":     "failures",
				"patterns": results,
			}
			if err := json.NewEncoder(stdout).Encode(data); err != nil {
				return fmt.Errorf("encode JSON: %w", err)
			}
		} else if robotFormat {
			_, _ = fmt.Fprintf(stdout, "*** Failures ***\n")
			for i, p := range results {
				exitCodeStr := "null"
				if p.ExitCode != nil {
					exitCodeStr = fmt.Sprintf("%d", *p.ExitCode)
				}
				_, _ = fmt.Fprintf(stdout, "result_%d_tool_name=%s\n", i, p.ToolName)
				_, _ = fmt.Fprintf(stdout, "result_%d_is_error=%v\n", i, p.IsError)
				_, _ = fmt.Fprintf(stdout, "result_%d_exit_code=%s\n", i, exitCodeStr)
				_, _ = fmt.Fprintf(stdout, "result_%d_count=%d\n", i, p.Count)
				_, _ = fmt.Fprintf(stdout, "result_%d_coverage=%d/%d\n", i, p.Count, p.SignalledEvents)
			}
			_, _ = fmt.Fprintf(stdout, "*** Total: %d patterns ***\n", len(results))
		} else {
			_, _ = fmt.Fprintf(stdout, "Failure Patterns\n")
			_, _ = fmt.Fprintf(stdout, "================\n\n")
			if len(results) > 0 {
				_, _ = fmt.Fprintf(stdout, "Signalled events (with error signal): %d\n\n", results[0].SignalledEvents)
			}
			for i, p := range results {
				_, _ = fmt.Fprintf(stdout, "%d. %s (is_error=%v, exit_code=%v) — %d occurrences\n",
					i+1, p.ToolName, p.IsError, p.ExitCode, p.Count)
			}
		}
	}

	return nil
}
