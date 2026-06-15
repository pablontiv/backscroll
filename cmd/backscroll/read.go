package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/reader"
)

func newReadCmd(stdout, stderr io.Writer) *cobra.Command {
	var path string
	var tail int
	var semantic bool
	var pretty bool
	cmd := &cobra.Command{
		Use:   "read [path]",
		Short: "Read a specific session or plan file",
		Long: `Read displays the contents of a session file or plan.
Default output format is agent-readable structured rows.
Use --pretty for human-readable formatting.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			locator := path
			if len(args) > 0 {
				if locator != "" {
					return fmt.Errorf("use either --path or positional path, not both")
				}
				locator = args[0]
			}
			if locator == "" {
				return fmt.Errorf("read requires --path <path>")
			}
			if semantic {
				return runReadSemantic(stdout, locator, tail, pretty)
			}
			return runRead(stdout, stderr, locator)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "path to the JSONL input file to read")
	cmd.Flags().IntVar(&tail, "tail", 0, "return only the last N semantic rows")
	cmd.Flags().BoolVar(&semantic, "semantic", false, "output concise semantic text/tool rows")
	cmd.Flags().BoolVar(&pretty, "pretty", false, "human-readable formatting")

	return cmd
}

func runRead(stdout, stderr io.Writer, path string) error {
	// Read the session file
	messages, err := reader.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Format and print
	for i, msg := range messages {
		_, _ = fmt.Fprintf(stdout, "=== Message %d ===\n", i+1)
		_, _ = fmt.Fprintf(stdout, "Role: %s\n", msg.Role)
		_, _ = fmt.Fprintf(stdout, "ContentType: %s\n", msg.ContentType)
		_, _ = fmt.Fprintf(stdout, "Timestamp: %s\n", msg.Timestamp.Format("2006-01-02 15:04:05 MST"))
		_, _ = fmt.Fprintf(stdout, "\n%s\n\n", msg.Content)
	}

	_, _ = fmt.Fprintf(stdout, "Total messages: %d\n", len(messages))

	return nil
}

func runReadSemantic(stdout io.Writer, path string, tail int, pretty bool) error {
	rows, err := reader.ReadSemanticTail(path, tail)
	if err != nil {
		return fmt.Errorf("read semantic file: %w", err)
	}

	if pretty {
		return formatSemanticRowsPretty(stdout, rows)
	}
	return formatSemanticRowsAgent(stdout, rows)
}

func formatSemanticRowsAgent(stdout io.Writer, rows []reader.SemanticRow) error {
	// Agent-readable format: tab-separated key=value pairs (default, no --pretty)
	for _, row := range rows {
		_, _ = fmt.Fprintf(
			stdout,
			"path=%q line=%d ordinal=%d timestamp=%q role=%q kind=%q content=%q\n",
			row.Path,
			row.Line,
			row.Ordinal,
			row.Timestamp,
			row.Role,
			row.Kind,
			row.Content,
		)
	}
	_, _ = fmt.Fprintf(stdout, "total=%d\n", len(rows))
	return nil
}

func formatSemanticRowsPretty(stdout io.Writer, rows []reader.SemanticRow) error {
	// Human-readable format with headers and aligned columns
	_, _ = fmt.Fprintf(stdout, "Path                                         Line Timestamp               Role        Kind       Content\n")
	_, _ = fmt.Fprintf(stdout, "----                                         ---- ---------               ----        ----       -------\n")
	for _, row := range rows {
		// Truncate long fields for readability
		path := row.Path
		if len(path) > 40 {
			path = "..." + path[len(path)-37:]
		}
		content := row.Content
		if len(content) > 30 {
			content = content[:27] + "..."
		}
		_, _ = fmt.Fprintf(
			stdout,
			"%-40s %4d %-23s %-11s %-10s %s\n",
			path,
			row.Line,
			row.Timestamp,
			row.Role,
			row.Kind,
			content,
		)
	}
	_, _ = fmt.Fprintf(stdout, "\nTotal rows: %d\n", len(rows))
	return nil
}
