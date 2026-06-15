package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
)

func newConfigCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonFormat bool

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show effective configuration and input manifests",
		Long: `Config displays the effective configuration, including:
- Database path
- Session directories
- Active inputs (from declarative manifest or legacy session_dirs)
- Input format and discovery rules

Use --json to output as JSON.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfig(stdout, stderr, jsonFormat)
		},
	}

	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")

	return cmd
}

func runConfig(stdout, stderr io.Writer, jsonFormat bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Resolve active inputs
	defs, mode, err := input_config.ActiveInputs(cfg.SessionDirs)
	if err != nil {
		return fmt.Errorf("resolve inputs: %w", err)
	}

	// Format output
	if jsonFormat {
		// JSON output with both config and inputs
		inputSummaries := make([]map[string]interface{}, len(defs))
		for i, def := range defs {
			inputSummaries[i] = map[string]interface{}{
				"id":      def.ID,
				"format":  def.Decode.Format,
				"include": def.Discover.Include,
				"exclude": def.Discover.Exclude,
			}
		}

		data := map[string]interface{}{
			"database": map[string]interface{}{
				"path": cfg.DatabasePath,
			},
			"session_dirs": cfg.SessionDirs,
			"inputs": map[string]interface{}{
				"mode":     mode.String(),
				"count":    len(defs),
				"manifest": inputSummaries,
			},
		}
		if err := json.NewEncoder(stdout).Encode(data); err != nil {
			return fmt.Errorf("encode JSON: %w", err)
		}
	} else {
		// Text output
		_, _ = fmt.Fprintf(stdout, "Backscroll Configuration\n")
		_, _ = fmt.Fprintf(stdout, "========================\n\n")

		_, _ = fmt.Fprintf(stdout, "Database:\n")
		_, _ = fmt.Fprintf(stdout, "  Path: %s\n\n", cfg.DatabasePath)

		_, _ = fmt.Fprintf(stdout, "Session Directories:\n")
		for _, dir := range cfg.SessionDirs {
			_, _ = fmt.Fprintf(stdout, "  - %s\n", dir)
		}

		_, _ = fmt.Fprintf(stdout, "\nInputs (%s):\n", mode)
		for _, def := range defs {
			_, _ = fmt.Fprintf(stdout, "  id:      %s\n", def.ID)
			_, _ = fmt.Fprintf(stdout, "  format:  %s\n", def.Decode.Format)
			_, _ = fmt.Fprintf(stdout, "  include: %v\n", def.Discover.Include)
			_, _ = fmt.Fprintf(stdout, "  exclude: %v\n", def.Discover.Exclude)
			_, _ = fmt.Fprintln(stdout)
		}
	}

	return nil
}
