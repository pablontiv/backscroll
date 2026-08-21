package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func restoreStartupCoordinatorGlobals(t *testing.T) {
	t.Helper()
	originalTryAcquire := startupTryAcquire
	originalAcquire := startupAcquire
	originalMutationWait := startupMutationWait
	originalPrepare := startupPrepareIndex
	originalSync := startupSync
	t.Cleanup(func() {
		startupTryAcquire = originalTryAcquire
		startupAcquire = originalAcquire
		startupMutationWait = originalMutationWait
		startupPrepareIndex = originalPrepare
		startupSync = originalSync
	})
}

func TestCoordinateStartupImmediateOwnerSnapshotSyncsAndReleasesBeforeResult(t *testing.T) {
	restoreStartupCoordinatorGlobals(t)
	lease := &fakeStartupLease{}
	syncCalls := 0
	startupTryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
	startupPrepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		return nil, nil, nil
	}
	startupSync = func(*config.Config, io.Writer) error {
		syncCalls++
		if lease.releases != 0 {
			t.Fatalf("lease released before sync")
		}
		return nil
	}

	result := coordinateStartup(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupSnapshotRead)
	if result.Failure != nil {
		t.Fatalf("startup failed: %+v", result.Failure)
	}
	if result.Lease != nil {
		t.Fatalf("snapshot result retained lease %+v", result.Lease)
	}
	if syncCalls != 1 {
		t.Fatalf("sync calls=%d want 1", syncCalls)
	}
	if lease.releases != 1 {
		t.Fatalf("lease releases=%d want 1", lease.releases)
	}
}

func TestCoordinateStartupImmediateOwnerMutationSyncsAndRetainsLease(t *testing.T) {
	restoreStartupCoordinatorGlobals(t)
	lease := &fakeStartupLease{}
	syncCalls := 0
	startupTryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
	startupPrepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		return nil, nil, nil
	}
	startupSync = func(*config.Config, io.Writer) error { syncCalls++; return nil }

	result := coordinateStartup(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupMutation)
	if result.Failure != nil {
		t.Fatalf("startup failed: %+v", result.Failure)
	}
	if result.Lease != lease {
		t.Fatalf("result lease=%+v want retained owner lease", result.Lease)
	}
	if lease.releases != 0 {
		t.Fatalf("mutation lease releases=%d want 0 before handler", lease.releases)
	}
	if syncCalls != 1 {
		t.Fatalf("sync calls=%d want 1", syncCalls)
	}
}

func TestCoordinateStartupBusySnapshotUsesCompatibleReadOnlySnapshot(t *testing.T) {
	restoreStartupCoordinatorGlobals(t)
	dbPath := seedCompatibleStartupDB(t)
	cfg := &config.Config{DatabasePath: dbPath}
	syncCalls := 0
	startupTryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	startupSync = func(*config.Config, io.Writer) error { syncCalls++; return nil }

	result := coordinateStartup(context.Background(), cfg, io.Discard, startupSnapshotRead)
	if result.Failure != nil {
		t.Fatalf("busy snapshot failed: %+v", result.Failure)
	}
	if result.Warning == nil || result.Warning.Code != compat.CodeSyncInProgress {
		t.Fatalf("warning=%+v want sync_in_progress", result.Warning)
	}
	if !strings.Contains(result.Warning.Summary, "last committed index snapshot") {
		t.Fatalf("warning summary=%q want committed snapshot", result.Warning.Summary)
	}
	if syncCalls != 0 {
		t.Fatalf("follower sync calls=%d want 0", syncCalls)
	}
	if _, diag, err := prepareIndex(context.Background(), cfg, indexMutation); diag != nil || err != nil {
		t.Fatalf("busy snapshot inspection leaked read handle: diag=%+v err=%v", diag, err)
	}
}

func TestCoordinateStartupBusyMetadataSkipsReadPreparationWhenDatabaseMissing(t *testing.T) {
	restoreStartupCoordinatorGlobals(t)
	prepareCalls := 0
	startupTryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	startupPrepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		prepareCalls++
		return nil, nil, nil
	}

	result := coordinateStartup(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "missing.db")}, io.Discard, startupMetadataRead)
	if result.Failure != nil {
		t.Fatalf("busy metadata failed: %+v", result.Failure)
	}
	if result.Warning == nil || result.Warning.Code != compat.CodeSyncInProgress {
		t.Fatalf("warning=%+v want sync_in_progress", result.Warning)
	}
	if prepareCalls != 0 {
		t.Fatalf("prepare calls=%d want 0", prepareCalls)
	}
}

func TestCoordinateStartupBusyMutationAcquiresWithinWaitAndBecomesOwner(t *testing.T) {
	restoreStartupCoordinatorGlobals(t)
	lease := &fakeStartupLease{}
	syncCalls := 0
	startupTryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	startupAcquire = func(ctx context.Context, path string, delay time.Duration) (startupLease, error) {
		if delay != startupLockRetry {
			t.Fatalf("retry delay=%v want %v", delay, startupLockRetry)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatalf("mutation acquire context has no deadline")
		}
		return lease, nil
	}
	startupPrepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		return nil, nil, nil
	}
	startupSync = func(*config.Config, io.Writer) error { syncCalls++; return nil }

	result := coordinateStartup(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupMutation)
	if result.Failure != nil {
		t.Fatalf("startup failed: %+v", result.Failure)
	}
	if result.Lease != lease {
		t.Fatalf("lease=%+v want acquired lease", result.Lease)
	}
	if syncCalls != 1 {
		t.Fatalf("sync calls=%d want 1", syncCalls)
	}
}

func TestCoordinateStartupBusyMutationDeadlineReturnsSyncInProgressWithoutContinuation(t *testing.T) {
	restoreStartupCoordinatorGlobals(t)
	startupMutationWait = time.Millisecond
	startupTryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	startupAcquire = func(ctx context.Context, path string, delay time.Duration) (startupLease, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	startupSync = func(*config.Config, io.Writer) error {
		t.Fatal("sync should not run after mutation lock deadline")
		return nil
	}

	result := coordinateStartup(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupMutation)
	failure := result.startupFailure()
	if failure == nil {
		t.Fatal("busy mutation unexpectedly succeeded")
	}
	if failure.Stage != startupStageSyncLock || failure.Diagnostic.Code != compat.CodeSyncInProgress {
		t.Fatalf("failure=%+v want sync lock sync_in_progress", failure)
	}
	if len(failure.Diagnostic.Continuation) != 0 {
		t.Fatalf("continuation=%v want none", failure.Diagnostic.Continuation)
	}
	if result.Lease != nil {
		t.Fatalf("deadline result retained lease %+v", result.Lease)
	}
}

func TestCoordinateStartupLockIOErrorIsSyncLockFailureNotContention(t *testing.T) {
	restoreStartupCoordinatorGlobals(t)
	lockErr := errors.New("lock sidecar I/O failed")
	startupTryAcquire = func(string) (startupLease, bool, error) { return nil, false, lockErr }

	result := coordinateStartup(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupSnapshotRead)
	failure := result.startupFailure()
	if failure == nil {
		t.Fatal("lock I/O error unexpectedly succeeded")
	}
	if failure.Stage != startupStageSyncLock {
		t.Fatalf("stage=%s want %s", failure.Stage, startupStageSyncLock)
	}
	if failure.Diagnostic.Code == compat.CodeSyncInProgress {
		t.Fatalf("lock I/O error misclassified as contention: %+v", failure)
	}
	if !errors.Is(failure.Cause, lockErr) {
		t.Fatalf("cause=%v want lock err", failure.Cause)
	}
}

func TestCoordinateStartupOwnerFailureLeaseRetentionByCommandClass(t *testing.T) {
	for _, tc := range []struct {
		name         string
		class        startupCommandClass
		failPrepare  bool
		wantRetained bool
	}{
		{name: "snapshot_prepare_failure_releases", class: startupSnapshotRead, failPrepare: true, wantRetained: false},
		{name: "mutation_prepare_failure_retains", class: startupMutation, failPrepare: true, wantRetained: true},
		{name: "snapshot_sync_failure_releases", class: startupSnapshotRead, wantRetained: false},
		{name: "mutation_sync_failure_retains", class: startupMutation, wantRetained: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restoreStartupCoordinatorGlobals(t)
			lease := &fakeStartupLease{}
			startupTryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
			prepareErr := errors.New("prepare failed")
			syncErr := errors.New("sync failed")
			startupPrepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
				if tc.failPrepare {
					return nil, nil, prepareErr
				}
				return nil, nil, nil
			}
			startupSync = func(*config.Config, io.Writer) error { return syncErr }

			result := coordinateStartup(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, tc.class)
			failure := result.startupFailure()
			if failure == nil || !failure.Recoverable {
				t.Fatalf("failure=%+v want recoverable startup failure", failure)
			}
			if tc.wantRetained {
				if result.Lease != lease || lease.releases != 0 {
					t.Fatalf("lease=%+v releases=%d want retained", result.Lease, lease.releases)
				}
			} else {
				if result.Lease != nil || lease.releases != 1 {
					t.Fatalf("lease=%+v releases=%d want released before refusal", result.Lease, lease.releases)
				}
			}
		})
	}
}

func seedCompatibleStartupDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	if err := db.SyncFiles([]storage.IndexedFile{{
		SourcePath: "/fixture/session.jsonl",
		Source:     "session",
		Hash:       "hash",
		Project:    "project",
		Messages: []storage.IndexedMessage{{
			Ordinal:           0,
			UUID:              "u",
			Role:              "user",
			Text:              "cached snapshot",
			Timestamp:         "2026-01-01T00:00:00Z",
			ContentType:       "text",
			ExtractionVersion: storage.CurrentExtractionVersion,
		}},
	}}); err != nil {
		_ = db.Close()
		t.Fatalf("seed sync: %v", err)
	}
	if _, err := db.DB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = db.Close()
		t.Fatalf("checkpoint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}
	return dbPath
}
