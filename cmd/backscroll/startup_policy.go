package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/spf13/cobra"
)

type startupPolicyFunc func(context.Context, io.Writer) startupResult

type startupStage string

const (
	startupStageUnknown        startupStage = "unknown"
	startupStageInputDir       startupStage = "input_dir"
	startupStageLegacySource   startupStage = "legacy_source"
	startupStageConfigLoad     startupStage = "config_load"
	startupStageActiveManifest startupStage = "active_manifest"
	startupStageIndexPrepare   startupStage = "index_prepare"
	startupStageStartupSync    startupStage = "startup_sync"
)

type startupFailure struct {
	Stage       startupStage
	Cause       error
	Diagnostic  compat.Diagnostic
	Recoverable bool
}

func (f *startupFailure) Error() string {
	if f == nil {
		return "startup failure"
	}
	stage := f.Stage
	if stage == "" {
		stage = startupStageUnknown
	}
	var rendered []string
	if f.Diagnostic.Code != "" || strings.TrimSpace(f.Diagnostic.Summary) != "" {
		rendered = append(rendered, fmt.Sprintf("diagnostic %s: %s", f.Diagnostic.Code, strings.TrimSpace(f.Diagnostic.Summary)))
	}
	if len(f.Diagnostic.Continuation) > 0 {
		rendered = append(rendered, fmt.Sprintf("continuation: %s", strings.Join(f.Diagnostic.Continuation, " ")))
	}
	if f.Cause != nil {
		rendered = append(rendered, f.Cause.Error())
	}
	if len(rendered) == 0 {
		rendered = append(rendered, "startup failed")
	}
	return fmt.Sprintf("startup %s failed: %s", stage, strings.Join(rendered, ": "))
}

func (f *startupFailure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Cause
}

func (f *startupFailure) renderedDiagnostic() compat.Diagnostic {
	d := f.Diagnostic
	if !f.Recoverable {
		d.Continuation = nil
	}
	return d
}

type startupResult struct {
	Config  *config.Config
	Failure *startupFailure
}

func (r startupResult) startupFailure() *startupFailure {
	return r.Failure
}

func optionalStartupFailureError(f *startupFailure) error {
	if f == nil {
		return nil
	}
	return f
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
		cause := fmt.Errorf("resolve inputs directory: %w", err)
		return startupResult{Failure: &startupFailure{Stage: startupStageInputDir, Cause: cause, Diagnostic: compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: cause.Error()}}}
	}
	if err := config.ValidateNoLegacySources(inputsDir); err != nil {
		stage := startupStageConfigLoad
		var legacyErr *config.LegacySourcesError
		if errors.As(err, &legacyErr) {
			stage = startupStageLegacySource
		}
		return startupResult{Failure: &startupFailure{Stage: stage, Cause: err, Diagnostic: compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: err.Error()}}}
	}
	cfg, err := config.Load()
	if err != nil {
		cause := fmt.Errorf("load config: %w", err)
		return startupResult{Failure: &startupFailure{Stage: startupStageConfigLoad, Cause: cause, Diagnostic: compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: cause.Error()}}}
	}
	if _, _, err := input_config.ActiveInputs(cfg.SessionDirs); err != nil {
		cause := fmt.Errorf("validate active inputs: %w", err)
		return startupResult{Config: cfg, Failure: &startupFailure{Stage: startupStageActiveManifest, Cause: cause, Diagnostic: compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: cause.Error()}}}
	}
	db, diag, err := prepareIndex(ctx, cfg, indexMutation)
	if db != nil {
		err = closeIndexDB(db, err)
	}
	if diag != nil || err != nil {
		d := compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: "prepare index failed"}
		if diag != nil {
			d = *diag
		} else if err != nil {
			d.Summary = fmt.Sprintf("prepare index failed: %v", err)
		}
		return startupResult{Config: cfg, Failure: &startupFailure{Stage: startupStageIndexPrepare, Cause: err, Diagnostic: d, Recoverable: true}}
	}
	if err := startupSync(cfg, progress); err != nil {
		activePath, _ := resolveActiveIndexPath(cfg.DatabasePath)
		d := continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: fmt.Sprintf("index sync failed: %v", err)}, activePath)
		return startupResult{Config: cfg, Failure: &startupFailure{Stage: startupStageStartupSync, Cause: err, Diagnostic: d, Recoverable: true}}
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
			failure := result.startupFailure()
			if failure == nil {
				return nil
			}
			if cmd.Name() == "recover" && failure.Recoverable {
				return nil
			}
			if failure.Diagnostic.Code != "" || strings.TrimSpace(failure.Diagnostic.Summary) != "" {
				return refuseIndexWithCause(stdout, stderr, failure.renderedDiagnostic(), failure, commandBoolFlag(cmd, "json"), commandBoolFlag(cmd, "robot"))
			}
			return failure
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
