package main

import (
	"context"
	"fmt"
	"io"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/spf13/cobra"
)

type startupPolicyFunc func(context.Context, io.Writer) startupResult

type startupResult struct {
	Config     *config.Config
	Diagnostic *compat.Diagnostic
	Err        error
}

type startupContextKey struct{}

var startupSync = maybeAutoSync

func startupResultFrom(cmd *cobra.Command) startupResult {
	result, _ := cmd.Context().Value(startupContextKey{}).(startupResult)
	return result
}

func defaultStartupPolicy(ctx context.Context, progress io.Writer) startupResult {
	inputsDir, err := input_config.InputsDir()
	if err != nil {
		return startupResult{Err: fmt.Errorf("resolve inputs directory: %w", err)}
	}
	if err := config.ValidateNoLegacySources(inputsDir); err != nil {
		return startupResult{Err: err}
	}
	cfg, err := config.Load()
	if err != nil {
		return startupResult{Err: fmt.Errorf("load config: %w", err)}
	}
	if _, _, err := input_config.ActiveInputs(cfg.SessionDirs); err != nil {
		return startupResult{Config: cfg, Err: fmt.Errorf("validate active inputs: %w", err)}
	}
	db, diag, err := prepareIndex(ctx, cfg, indexMutation)
	if db != nil {
		err = closeIndexDB(db, err)
	}
	if diag != nil || err != nil {
		return startupResult{Config: cfg, Diagnostic: diag, Err: err}
	}
	if err := startupSync(cfg, progress); err != nil {
		activePath, _ := resolveActiveIndexPath(cfg.DatabasePath)
		d := continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: fmt.Sprintf("index sync failed: %v", err)}, activePath)
		return startupResult{Config: cfg, Diagnostic: &d, Err: err}
	}
	return startupResult{Config: cfg}
}

func buildRootCmd(stdout, stderr io.Writer) *cobra.Command {
	return buildRootCmdWithStartup(stdout, stderr, defaultStartupPolicy)
}

func buildRootCmdWithStartup(stdout, stderr io.Writer, policy startupPolicyFunc) *cobra.Command {
	root := &cobra.Command{
		Use:           "backscroll",
		Short:         "A permanent, searchable record of your coding-agent sessions",
		SilenceErrors: true,
		Long: `Backscroll turns your coding-agent sessions into a permanent, searchable
record of what happened. It indexes Claude Code, Pi and OpenCode sessions into
SQLite and keeps them after the session files expire.

Prose and tool activity are indexed separately — a Porter-stemmed FTS5 index for
conversation, a trigram index for commands, paths and errors — and an unfiltered
query merges both by rank position (RRF).`,
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			result := policy(cmd.Context(), startupProgressWriter(cmd, stderr))
			cmd.SetContext(context.WithValue(cmd.Context(), startupContextKey{}, result))
			if result.Diagnostic == nil && result.Err == nil {
				return nil
			}
			if cmd.Name() == "recover" {
				return nil
			}
			if result.Diagnostic != nil {
				return refuseIndex(stdout, stderr, *result.Diagnostic, commandBoolFlag(cmd, "json"), commandBoolFlag(cmd, "robot"))
			}
			return result.Err
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.AddCommand(
		newSearchCmd(stdout, stderr),
		newListCmd(stdout, stderr),
		newPatternsCmd(stdout, stderr),
		newRebuildCmd(stdout, stderr),
		newPurgeCmd(stdout, stderr),
		newValidateCmd(stdout, stderr),
		newStatusCmd(stdout, stderr),
		newConfigCmd(stdout, stderr),
		newAnnotateCmd(stdout, stderr),
		newRecoverCmd(stdout, stderr),
	)

	return root
}

func startupProgressWriter(cmd *cobra.Command, stderr io.Writer) io.Writer {
	if commandBoolFlag(cmd, "json") || commandBoolFlag(cmd, "robot") {
		return io.Discard
	}
	return stderr
}

func commandBoolFlag(cmd *cobra.Command, name string) bool {
	flag := cmd.Flag(name)
	if flag == nil {
		return false
	}
	return flag.Value.String() == "true"
}
