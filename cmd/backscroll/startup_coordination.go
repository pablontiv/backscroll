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

type startupCoordinator struct {
	mutationWait time.Duration
	tryAcquire   func(string) (startupLease, bool, error)
	acquire      func(context.Context, string, time.Duration) (startupLease, error)
	prepareIndex func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error)
	syncService  *startupSyncService
}

func newStartupCoordinator() *startupCoordinator {
	return newStartupCoordinatorWithSyncService(newStartupSyncService())
}

func newStartupCoordinatorWithSyncService(syncService *startupSyncService) *startupCoordinator {
	if syncService == nil {
		syncService = newStartupSyncService()
	}
	return &startupCoordinator{
		mutationWait: defaultStartupMutationWait,
		tryAcquire: func(path string) (startupLease, bool, error) {
			return startuplock.TryAcquire(path)
		},
		acquire: func(ctx context.Context, path string, delay time.Duration) (startupLease, error) {
			return startuplock.Acquire(ctx, path, delay)
		},
		prepareIndex: func(ctx context.Context, cfg *config.Config, class indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
			return prepareIndex(ctx, cfg, class)
		},
		syncService: syncService,
	}
}

func (c *startupCoordinator) coordinate(ctx context.Context, cfg *config.Config, progress io.Writer, class startupCommandClass) (result startupResult) {
	if err := ctx.Err(); err != nil {
		return canceledStartupResult(cfg, startupStageSyncLock, err)
	}

	timing := startupPhaseTiming{measured: c.syncService.diagnostics}
	var lockStart time.Time
	if timing.measured {
		timing.report, _ = ctx.Value(startupDiagnosticsContextKey{}).(*startupDiagnostics)
		if timing.report == nil {
			timing.report = c.syncService.newDiagnostics()
		}
		lockStart = timing.report.now()
		if !timing.report.policyOwned {
			defer func() {
				if class != startupRemediation || result.Failure != nil {
					timing.report.render(startupDiagnosticsWriter(ctx, progress))
				}
			}()
		}
	}

	lease, acquired, err := c.tryAcquire(cfg.DatabasePath)
	if timing.measured {
		timing.LockAcquisitionTime = timing.report.now().Sub(lockStart)
		timing.report.lock, timing.report.hasLock = timing.LockAcquisitionTime, true
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		if timing.measured {
			timing.report.failedPhase = "Lock Acquisition"
		}
		result := canceledStartupResult(cfg, startupStageSyncLock, ctxErr)
		if acquired {
			return ownedStartupFailureResult(cfg, class, lease, result.Failure)
		}
		return result
	}

	if err != nil {
		if timing.measured {
			timing.report.failedPhase = "Lock Acquisition"
		}
		return startupLockFailure(cfg, err)
	}
	if acquired {
		return c.runOwned(ctx, cfg, progress, class, lease, timing)
	}

	switch class {
	case startupSnapshotRead:
		if timing.measured {
			start := timing.report.now()
			result = c.concurrentSnapshotResult(ctx, cfg)
			timing.report.prepare, timing.report.hasPrepare = timing.report.now().Sub(start), true
			if result.Failure != nil {
				timing.report.failedPhase = "Index Prepare"
			}
			return result
		}
		return c.concurrentSnapshotResult(ctx, cfg)
	case startupMetadataRead:
		return startupResult{
			Config: cfg,
			Warning: &startupWarning{
				Code:    compat.CodeSyncInProgress,
				Summary: "startup sync active; continuing without a new startup sync",
			},
		}
	default:
		waitCtx, cancel := context.WithTimeout(ctx, c.mutationWait)
		defer cancel()

		lease, err := c.acquire(waitCtx, cfg.DatabasePath, startupLockRetry)

		if timing.measured {
			timing.LockAcquisitionTime = timing.report.now().Sub(lockStart)
			timing.report.lock, timing.report.hasLock = timing.LockAcquisitionTime, true
		}

		if err != nil {
			if timing.measured {
				timing.report.failedPhase = "Lock Acquisition"
			}
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
		return c.runOwned(ctx, cfg, progress, class, lease, timing)
	}
}

func (c *startupCoordinator) runOwned(ctx context.Context, cfg *config.Config, progress io.Writer, class startupCommandClass, lease startupLease, timing startupPhaseTiming) startupResult {
	if class == startupRemediation {
		return startupResult{Config: cfg, Lease: lease, timing: timing}
	}

	var indexPrepareStart time.Time
	if timing.measured {
		indexPrepareStart = timing.report.now()
	}

	db, diag, err := c.prepareIndex(ctx, cfg, indexMutation)
	if db != nil {
		err = closeIndexDB(db, err)
	}

	if timing.measured {
		timing.IndexPrepareTime = timing.report.now().Sub(indexPrepareStart)
		timing.report.prepare, timing.report.hasPrepare = timing.IndexPrepareTime, true
	}

	if ctxErr := ctx.Err(); ctxErr != nil && diag == nil && err == nil {
		err = ctxErr
	}
	if diag != nil || err != nil {
		if timing.measured {
			timing.report.failedPhase = "Index Prepare"
		}
		d := compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: "prepare index failed"}
		if diag != nil {
			d = *diag
		} else if err != nil {
			d.Summary = fmt.Sprintf("prepare index failed: %v", err)
		}
		return ownedStartupFailureResult(cfg, class, lease, &startupFailure{Stage: startupStageIndexPrepare, Cause: err, Diagnostic: d, Recoverable: true})
	}
	if err := c.syncService.sync(ctx, cfg, progress, timing); err != nil {
		activePath, _ := resolveActiveIndexPath(cfg.DatabasePath)
		d := continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: fmt.Sprintf("index sync failed: %v", err)}, activePath)
		return ownedStartupFailureResult(cfg, class, lease, &startupFailure{Stage: startupStageStartupSync, Cause: err, Diagnostic: d, Recoverable: true})
	}
	if err := ctx.Err(); err != nil {
		activePath, _ := resolveActiveIndexPath(cfg.DatabasePath)
		d := continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: fmt.Sprintf("index sync failed: %v", err)}, activePath)
		return ownedStartupFailureResult(cfg, class, lease, &startupFailure{Stage: startupStageStartupSync, Cause: err, Diagnostic: d, Recoverable: true})
	}
	result := startupResult{Config: cfg}
	if startupClassRetainsLease(class) {
		result.Lease = lease
		return result
	}
	return releaseOwnedStartupResult(result, lease, startupStageSyncLock)
}

func ownedStartupFailureResult(cfg *config.Config, class startupCommandClass, lease startupLease, failure *startupFailure) startupResult {
	result := startupResult{Config: cfg, Failure: failure}
	if startupClassRetainsLease(class) {
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

func (c *startupCoordinator) concurrentSnapshotResult(ctx context.Context, cfg *config.Config) startupResult {
	db, diag, err := c.prepareIndex(ctx, cfg, indexDataRead)
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

func canceledStartupResult(cfg *config.Config, stage startupStage, err error) startupResult {
	return startupResult{Config: cfg, Failure: &startupFailure{
		Stage: stage,
		Cause: err,
		Diagnostic: compat.Diagnostic{
			Code:    compat.CodeMigrationFailed,
			Summary: fmt.Sprintf("startup canceled: %v", err),
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
