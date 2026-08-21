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

type startupPolicyFunc func(context.Context, io.Writer, startupCommandClass) startupResult

type startupStage string

const (
	startupStageUnknown        startupStage = "unknown"
	startupStageInputDir       startupStage = "input_dir"
	startupStageLegacySource   startupStage = "legacy_source"
	startupStageConfigLoad     startupStage = "config_load"
	startupStageActiveManifest startupStage = "active_manifest"
	startupStageSyncLock       startupStage = "sync_lock"
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

type startupLease interface {
	Release() error
}

type startupWarning struct {
	Code    compat.Code
	Summary string
}

type startupResult struct {
	Config  *config.Config
	Failure *startupFailure
	Warning *startupWarning
	Lease   startupLease
}

func (r startupResult) startupFailure() *startupFailure {
	return r.Failure
}

func (r startupResult) release(retErr error) error {
	if r.Lease == nil {
		return retErr
	}
	if releaseErr := r.Lease.Release(); releaseErr != nil {
		return errors.Join(retErr, fmt.Errorf("release startup lock: %w", releaseErr))
	}
	return retErr
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

func defaultStartupPolicy(ctx context.Context, progress io.Writer, class startupCommandClass) startupResult {
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
	return coordinateStartup(ctx, cfg, progress, class)
}

func validateCommandBeforeStartup(cmd *cobra.Command, args []string, positional cobra.PositionalArgs, semantic func() error) error {
	if positional != nil {
		if err := positional(cmd, args); err != nil {
			return err
		}
	}
	if err := validateRequiredFlagsAndGroups(cmd); err != nil {
		return err
	}
	if semantic != nil {
		return semantic()
	}
	return nil
}

func validateRequiredFlagsAndGroups(cmd *cobra.Command) error {
	if err := cmd.ValidateRequiredFlags(); err != nil {
		return err
	}
	return cmd.ValidateFlagGroups()
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
			if err := validateRequiredFlagsAndGroups(cmd); err != nil {
				return err
			}
			class, _ := startupCommandClassFor(cmd)
			result := policy(cmd.Context(), startupProgressWriter(cmd, stderr), class)
			cmd.SetContext(context.WithValue(cmd.Context(), startupContextKey{}, result))
			if result.Warning != nil {
				if _, err := fmt.Fprintf(stderr, "warning: %s: %s\n", result.Warning.Code, result.Warning.Summary); err != nil {
					return result.release(err)
				}
			}
			failure := result.startupFailure()
			if failure == nil {
				return nil
			}
			if cmd.Name() == "recover" && failure.Recoverable && class == startupMutation {
				return nil
			}
			failureErr := result.release(failure)
			if failure.Diagnostic.Code != "" || strings.TrimSpace(failure.Diagnostic.Summary) != "" {
				return refuseIndexWithCause(stdout, stderr, failure.renderedDiagnostic(), failureErr, commandBoolFlag(cmd, "json"), commandBoolFlag(cmd, "robot"))
			}
			return failureErr
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	registerStartupCommand(root, startupSnapshotRead, newSearchCmd(stdout, stderr))
	registerStartupCommand(root, startupSnapshotRead, newListCmd(stdout, stderr))
	registerStartupCommand(root, startupSnapshotRead, newPatternsCmd(stdout, stderr))
	registerStartupCommand(root, startupMutation, newRebuildCmd(stdout, stderr))
	registerStartupCommand(root, startupMutation, newPurgeCmd(stdout, stderr))
	registerStartupCommand(root, startupSnapshotRead, newValidateCmd(stdout, stderr))
	registerStartupCommand(root, startupSnapshotRead, newStatusCmd(stdout, stderr))
	registerStartupCommand(root, startupMetadataRead, newConfigCmd(stdout, stderr))
	registerStartupCommand(root, startupMutation, newAnnotateCmd(stdout, stderr))
	registerStartupCommand(root, startupMutation, newRecoverCmd(stdout, stderr))

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
