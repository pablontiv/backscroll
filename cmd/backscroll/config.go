package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
)

func newConfigCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonFormat bool

	cmd := &cobra.Command{
		Use:          "config",
		Args:         cobra.NoArgs,
		Short:        "Show effective configuration and input manifests",
		SilenceUsage: true,
		Long: `This command accepts no positional operands.

Config displays the effective configuration, including:
- Database path
- Session directories
- Active inputs (from declarative manifest or legacy session_dirs)
- Input format and discovery rules

Use --json to output as JSON.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			startup := startupResultFrom(cmd)
			cfg, err := startup.requireConfig()
			if err != nil {
				return err
			}
			return runConfig(stdout, stderr, cfg, startup.inputs, invocationOutputMode(cmd))
		},
	}

	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")

	return cmd
}

func runConfig(stdout, stderr io.Writer, cfg *config.Config, snapshot *inputSnapshot, output outputMode) error {
	if snapshot == nil {
		return fmt.Errorf("startup input snapshot unavailable")
	}
	defs, mode := cloneInputDefinitions(snapshot.definitions), snapshot.mode

	// Format output
	if output == outputJSON {
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
		if err := writeOutput(stdout, "Backscroll Configuration\n"); err != nil {
			return err
		}
		if err := writeOutput(stdout, "========================\n\n"); err != nil {
			return err
		}

		if err := writeOutput(stdout, "Database:\n"); err != nil {
			return err
		}
		if err := writeOutput(stdout, "  Path: %s\n\n", cfg.DatabasePath); err != nil {
			return err
		}

		if err := writeOutput(stdout, "Session Directories:\n"); err != nil {
			return err
		}
		for _, dir := range cfg.SessionDirs {
			if err := writeOutput(stdout, "  - %s\n", dir); err != nil {
				return err
			}
		}

		if err := writeOutput(stdout, "\nInputs (%s):\n", mode); err != nil {
			return err
		}
		for _, def := range defs {
			if err := writeOutput(stdout, "  id:      %s\n", def.ID); err != nil {
				return err
			}
			if err := writeOutput(stdout, "  format:  %s\n", def.Decode.Format); err != nil {
				return err
			}
			if err := writeOutput(stdout, "  include: %v\n", def.Discover.Include); err != nil {
				return err
			}
			if err := writeOutput(stdout, "  exclude: %v\n", def.Discover.Exclude); err != nil {
				return err
			}
			if _, err := fmt.Fprintln(stdout); err != nil {
				return fmt.Errorf("write output: %w", err)
			}
		}
	}

	return nil
}
