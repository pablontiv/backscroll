package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
)

func newInputsCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inputs",
		Short: "Inspect and test input manifests",
	}
	cmd.AddCommand(
		newInputsListCmd(stdout, stderr),
		newInputsValidateCmd(stdout, stderr),
		newInputsAliasesCmd(stdout, stderr),
		newInputsIdentifyCmd(stdout, stderr),
		newInputsTestCmd(stdout, stderr),
	)
	return cmd
}

func newInputsListCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonFormat bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active input definitions",
		RunE: func(cmd *cobra.Command, args []string) error {
			defs, mode, err := resolveInputs()
			if err != nil {
				return err
			}
			if jsonFormat {
				return json.NewEncoder(stdout).Encode(map[string]any{
					"mode":   mode.String(),
					"inputs": inputSummaries(defs),
				})
			}
			_, _ = fmt.Fprintf(stdout, "mode: %s\n", mode)
			_, _ = fmt.Fprintf(stdout, "active inputs: %d\n\n", len(defs))
			for _, def := range defs {
				_, _ = fmt.Fprintf(stdout, "  id:      %s\n", def.ID)
				_, _ = fmt.Fprintf(stdout, "  format:  %s\n", def.Decode.Format)
				_, _ = fmt.Fprintf(stdout, "  include: %v\n", def.Discover.Include)
				_, _ = fmt.Fprintf(stdout, "  exclude: %v\n", def.Discover.Exclude)
				_, _ = fmt.Fprintln(stdout)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	return cmd
}

func newInputsValidateCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonFormat bool

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate input manifests without syncing",
		RunE: func(cmd *cobra.Command, args []string) error {
			defs, mode, err := resolveInputs()
			if err != nil {
				if jsonFormat {
					_ = json.NewEncoder(stdout).Encode(map[string]any{
						"valid": false,
						"error": err.Error(),
					})
					return nil
				}
				return err
			}
			if jsonFormat {
				return json.NewEncoder(stdout).Encode(map[string]any{
					"valid":  true,
					"mode":   mode.String(),
					"inputs": len(defs),
				})
			}
			fmt.Fprintf(stdout, "Inputs valid: %d active inputs (%s)\n", len(defs), mode)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	return cmd
}

func newInputsAliasesCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aliases",
		Short: "Show discovered file paths for each input",
		RunE: func(cmd *cobra.Command, args []string) error {
			defs, _, err := resolveInputs()
			if err != nil {
				return err
			}
			for _, def := range defs {
				files, err := input_config.DiscoverFiles(def.Discover)
				if err != nil {
					fmt.Fprintf(stderr, "warning: discover %s: %v\n", def.ID, err)
					continue
				}
				fmt.Fprintf(stdout, "%s (%d files):\n", def.ID, len(files))
				for _, f := range files {
					fmt.Fprintf(stdout, "  %s\n", f)
				}
			}
			return nil
		},
	}
	return cmd
}

func newInputsIdentifyCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "identify <path>",
		Short: "Identify which input manifest matches a file path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}

			defs, _, err := resolveInputs()
			if err != nil {
				return err
			}

			for _, def := range defs {
				files, err := input_config.DiscoverFiles(def.Discover)
				if err != nil {
					continue
				}
				for _, f := range files {
					if f == target {
						fmt.Fprintf(stdout, "matched: %s (format: %s)\n", def.ID, def.Decode.Format)
						return nil
					}
				}
			}
			fmt.Fprintf(stdout, "no match for %s\n", target)
			return nil
		},
	}
	return cmd
}

func newInputsTestCmd(stdout, stderr io.Writer) *cobra.Command {
	var jsonFormat bool

	cmd := &cobra.Command{
		Use:   "test <path>",
		Short: "Dry-run the input pipeline on a file (no DB writes)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}

			defs, _, err := resolveInputs()
			if err != nil {
				return err
			}

			// Find the matching def
			var matched *input_config.InputDefinition
			for i, def := range defs {
				files, err := input_config.DiscoverFiles(def.Discover)
				if err != nil {
					continue
				}
				for _, f := range files {
					if f == target {
						matched = &defs[i]
						break
					}
				}
				if matched != nil {
					break
				}
			}

			if matched == nil {
				return fmt.Errorf("no input manifest matches %s — run 'backscroll inputs identify' to debug", target)
			}

			result, err := input_config.TestFile(target, *matched)
			if err != nil && !errors.Is(err, input_config.ErrDropped) {
				return err
			}

			if jsonFormat {
				return json.NewEncoder(stdout).Encode(result)
			}
			fmt.Fprintf(stdout, "input: %s\n", matched.ID)
			fmt.Fprintf(stdout, "records: %d\n", len(result))
			for i, r := range result {
				fmt.Fprintf(stdout, "\n[%d] role=%s uuid=%s\n%s\n", i+1, r.Role, r.UUID, truncate(r.Content, 200))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFormat, "json", false, "Output as JSON")
	return cmd
}

// resolveInputs loads inputs using declarative manifests or legacy session_dirs.
func resolveInputs() ([]input_config.InputDefinition, input_config.InputMode, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, input_config.ModeUnknown, fmt.Errorf("load config: %w", err)
	}
	return input_config.ActiveInputs(cfg.SessionDirs)
}

type inputSummary struct {
	ID      string   `json:"id"`
	Format  string   `json:"format"`
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func inputSummaries(defs []input_config.InputDefinition) []inputSummary {
	out := make([]inputSummary, len(defs))
	for i, d := range defs {
		out[i] = inputSummary{
			ID:      d.ID,
			Format:  d.Decode.Format,
			Include: d.Discover.Include,
			Exclude: d.Discover.Exclude,
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
