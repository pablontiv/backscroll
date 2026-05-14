package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/pablontiv/backscroll/internal/projects"
)

func newProjectsCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Project identity registry commands",
		Long:  `Commands for managing and querying the project identity registry.`,
	}
	cmd.AddCommand(
		newProjectsIdentifyCmd(stdout),
		newProjectsListCmd(stdout),
		newProjectsAliasesCmd(stdout),
	)
	return cmd
}

func newProjectsIdentifyCmd(stdout io.Writer) *cobra.Command {
	var (
		cwd     string
		jsonOut bool
	)
	cmd := &cobra.Command{
		Use:   "identify",
		Short: "Identify the canonical project for a directory path",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cwd == "" {
				var err error
				cwd, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("get working directory: %w", err)
				}
			}
			registry := projects.LoadGlobalRegistry()
			result := projects.Identify(cwd, registry)
			if jsonOut {
				return json.NewEncoder(stdout).Encode(map[string]string{
					"project_id": result.ProjectID,
					"confidence": string(result.Confidence),
					"cwd":        cwd,
				})
			}
			fmt.Fprintf(stdout, "project: %s (confidence: %s)\n", result.ProjectID, result.Confidence)
			return nil
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", "Directory path to identify (default: current directory)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON")
	return cmd
}

func newProjectsListCmd(stdout io.Writer) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all projects in the registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := projects.LoadGlobalRegistry()
			if jsonOut {
				return json.NewEncoder(stdout).Encode(map[string]interface{}{
					"count":    len(registry.Projects),
					"projects": registry.Projects,
				})
			}
			for _, p := range registry.Projects {
				fmt.Fprintf(stdout, "%s — roots: %v  patterns: %v  aliases: %v\n",
					p.ID, p.Roots, p.WorktreePatterns, p.Aliases)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON")
	return cmd
}

func newProjectsAliasesCmd(stdout io.Writer) *cobra.Command {
	var (
		projectID string
		jsonOut   bool
	)
	cmd := &cobra.Command{
		Use:   "aliases",
		Short: "Show aliases for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := projects.LoadGlobalRegistry()
			var aliases []string
			for _, p := range registry.Projects {
				if p.ID == projectID {
					aliases = p.Aliases
					break
				}
			}
			if aliases == nil {
				aliases = []string{}
			}
			if jsonOut {
				return json.NewEncoder(stdout).Encode(map[string]interface{}{
					"project_id": projectID,
					"aliases":    aliases,
				})
			}
			for _, alias := range aliases {
				_, _ = fmt.Fprintln(stdout, alias)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project ID to look up")
	if err := cmd.MarkFlagRequired("project-id"); err != nil {
		panic(err)
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON")
	return cmd
}
