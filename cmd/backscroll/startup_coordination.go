package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/startuplock"
	"github.com/pablontiv/backscroll/internal/storage"
)

const (
	defaultStartupMutationWait = 5 * time.Second
	startupLockRetry           = 50 * time.Millisecond
)

var startupMutationWait = defaultStartupMutationWait

var (
	startupTryAcquire = func(path string) (startupLease, bool, error) {
		return startuplock.TryAcquire(path)
	}
	startupAcquire = func(ctx context.Context, path string, delay time.Duration) (startupLease, error) {
		return startuplock.Acquire(ctx, path, delay)
	}
	startupPrepareIndex = func(ctx context.Context, cfg *config.Config, class indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		return prepareIndex(ctx, cfg, class)
	}
)

func coordinateStartup(ctx context.Context, cfg *config.Config, progress io.Writer, class startupCommandClass) startupResult {
	lease, acquired, err := startupTryAcquire(cfg.DatabasePath)
	if err != nil {
		return startupLockFailure(cfg, err)
	}
	if acquired {
		return runOwnedStartup(ctx, cfg, progress, class, lease)
	}

	switch class {
	case startupSnapshotRead:
		return concurrentSnapshotResult(ctx, cfg)
	case startupMetadataRead:
		return startupResult{
			Config: cfg,
			Warning: &startupWarning{
				Code:    compat.CodeSyncInProgress,
				Summary: "startup sync active; continuing without a new startup sync",
			},
		}
	default:
		waitCtx, cancel := context.WithTimeout(ctx, startupMutationWait)
		defer cancel()
		lease, err := startupAcquire(waitCtx, cfg.DatabasePath, startupLockRetry)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				return startupResult{Config: cfg, Failure: syncInProgressFailure(
					"startup sync remains active after 5s; retry the command", err,
				)}
			}
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return startupResult{Config: cfg, Failure: syncInProgressFailure(
					"startup lock wait canceled; retry the command", err,
				)}
			}
			return startupLockFailure(cfg, err)
		}
		return runOwnedStartup(ctx, cfg, progress, startupMutation, lease)
	}
}

func runOwnedStartup(ctx context.Context, cfg *config.Config, progress io.Writer, class startupCommandClass, lease startupLease) startupResult {
	db, diag, err := startupPrepareIndex(ctx, cfg, indexMutation)
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
		return ownedStartupFailureResult(cfg, class, lease, &startupFailure{Stage: startupStageIndexPrepare, Cause: err, Diagnostic: d, Recoverable: true})
	}
	if err := startupSync(cfg, progress); err != nil {
		activePath, _ := resolveActiveIndexPath(cfg.DatabasePath)
		d := continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: fmt.Sprintf("index sync failed: %v", err)}, activePath)
		return ownedStartupFailureResult(cfg, class, lease, &startupFailure{Stage: startupStageStartupSync, Cause: err, Diagnostic: d, Recoverable: true})
	}
	result := startupResult{Config: cfg}
	if class == startupMutation {
		result.Lease = lease
		return result
	}
	return releaseOwnedStartupResult(result, lease, startupStageSyncLock)
}

func ownedStartupFailureResult(cfg *config.Config, class startupCommandClass, lease startupLease, failure *startupFailure) startupResult {
	result := startupResult{Config: cfg, Failure: failure}
	if class == startupMutation {
		result.Lease = lease
		return result
	}
	return releaseOwnedStartupResult(result, lease, failure.Stage)
}

func releaseOwnedStartupResult(result startupResult, lease startupLease, stage startupStage) startupResult {
	if lease == nil {
		return result
	}
	if err := lease.Release(); err != nil {
		releaseErr := fmt.Errorf("release startup lock: %w", err)
		if result.Failure != nil {
			result.Failure.Cause = errors.Join(result.Failure.Cause, releaseErr)
			if result.Failure.Diagnostic.Summary == "" {
				result.Failure.Diagnostic.Summary = releaseErr.Error()
			}
			return result
		}
		result.Failure = &startupFailure{
			Stage: stage,
			Cause: releaseErr,
			Diagnostic: compat.Diagnostic{
				Code:    compat.CodeMigrationFailed,
				Summary: releaseErr.Error(),
			},
		}
	}
	return result
}

func concurrentSnapshotResult(ctx context.Context, cfg *config.Config) startupResult {
	db, diag, err := startupPrepareIndex(ctx, cfg, indexDataRead)
	if db != nil {
		err = closeIndexDB(db, err)
	}
	if diag != nil || err != nil {
		summary := "startup sync active; index snapshot is not ready; retry the command"
		if diag != nil && diag.Summary != "" {
			summary = "startup sync active; " + diag.Summary
		}
		return startupResult{Config: cfg, Failure: syncInProgressFailure(summary, snapshotContentionCause(diag, err))}
	}
	return startupResult{
		Config: cfg,
		Warning: &startupWarning{
			Code:    compat.CodeSyncInProgress,
			Summary: "startup sync active; using last committed index snapshot",
		},
	}
}

func snapshotContentionCause(diag *compat.Diagnostic, err error) error {
	if err != nil {
		return err
	}
	if diag != nil {
		return fmt.Errorf("%s: %s", diag.Code, diag.Summary)
	}
	return nil
}

func startupLockFailure(cfg *config.Config, err error) startupResult {
	return startupResult{Config: cfg, Failure: &startupFailure{
		Stage: startupStageSyncLock,
		Cause: err,
		Diagnostic: compat.Diagnostic{
			Code:    compat.CodeMigrationFailed,
			Summary: fmt.Sprintf("startup sync lock failed: %v", err),
		},
	}}
}

func syncInProgressFailure(summary string, cause error) *startupFailure {
	return &startupFailure{
		Stage: startupStageSyncLock,
		Cause: cause,
		Diagnostic: compat.Diagnostic{
			Code:    compat.CodeSyncInProgress,
			Summary: summary,
		},
	}
}
