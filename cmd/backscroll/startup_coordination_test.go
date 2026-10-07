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
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestStartupCoordinatorsIsolateDependencies(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"first", "second"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lease := &fakeStartupLease{}
			syncCalls := 0
			syncService := newStartupSyncService()
			syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
				syncCalls++
				return nil, input_config.ModeLegacy, nil
			}
			coordinator := &startupCoordinator{
				mutationWait: defaultStartupMutationWait,
				tryAcquire: func(path string) (startupLease, bool, error) {
					if !strings.Contains(path, name) {
						t.Fatalf("%s coordinator received another coordinator's path %q", name, path)
					}
					return lease, true, nil
				},
				acquire: func(context.Context, string, time.Duration) (startupLease, error) {
					t.Fatal("immediate owner unexpectedly waited for lock")
					return nil, nil
				},
				prepareIndex: func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
					return nil, nil, nil
				},
				syncService: syncService,
			}

			result := coordinator.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), name+".db")}, io.Discard, startupSnapshotRead)
			if result.Failure != nil {
				t.Fatalf("startup failed: %v", result.Failure)
			}
			if syncCalls != 1 || lease.releases != 1 {
				t.Fatalf("sync calls=%d releases=%d want 1 each", syncCalls, lease.releases)
			}
		})
	}
}

func TestCoordinateStartupImmediateOwnerSnapshotSyncsAndReleasesBeforeResult(t *testing.T) {
	coordinator := newStartupCoordinator()
	lease := &fakeStartupLease{}
	syncCalls := 0
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
	coordinator.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		return nil, nil, nil
	}
	coordinator.syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		syncCalls++
		if lease.releases != 0 {
			t.Fatalf("lease released before sync")
		}
		return nil, input_config.ModeLegacy, nil
	}

	result := coordinator.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupSnapshotRead)
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
	coordinator := newStartupCoordinator()
	lease := &fakeStartupLease{}
	syncCalls := 0
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
	coordinator.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		return nil, nil, nil
	}
	coordinator.syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		syncCalls++
		return nil, input_config.ModeLegacy, nil
	}

	result := coordinator.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupMutation)
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

func TestCoordinateStartupImmediateRemediationRetainsLeaseWithoutPrepareOrSync(t *testing.T) {
	coordinator := newStartupCoordinator()
	lease := &fakeStartupLease{}
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
	coordinator.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		t.Fatal("remediation must not prepare the index")
		return nil, nil, nil
	}
	coordinator.syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		t.Fatal("remediation must not run pre-handler sync")
		return nil, input_config.ModeLegacy, nil
	}

	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}
	result := coordinator.coordinate(context.Background(), cfg, io.Discard, startupRemediation)
	if result.Failure != nil {
		t.Fatalf("failure=%+v", result.Failure)
	}
	if result.Config != cfg || result.Lease != lease {
		t.Fatalf("result=%+v want cfg and retained lease", result)
	}
	if lease.releases != 0 {
		t.Fatalf("lease releases=%d want 0", lease.releases)
	}
}

func TestCoordinateStartupBusySnapshotUsesCompatibleReadOnlySnapshot(t *testing.T) {
	coordinator := newStartupCoordinator()
	dbPath := seedCompatibleStartupDB(t)
	cfg := &config.Config{DatabasePath: dbPath}
	syncCalls := 0
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	coordinator.syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		syncCalls++
		return nil, input_config.ModeLegacy, nil
	}

	result := coordinator.coordinate(context.Background(), cfg, io.Discard, startupSnapshotRead)
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
	coordinator := newStartupCoordinator()
	prepareCalls := 0
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	coordinator.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		prepareCalls++
		return nil, nil, nil
	}

	result := coordinator.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "missing.db")}, io.Discard, startupMetadataRead)
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
	coordinator := newStartupCoordinator()
	lease := &fakeStartupLease{}
	syncCalls := 0
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	coordinator.acquire = func(ctx context.Context, path string, delay time.Duration) (startupLease, error) {
		if delay != startupLockRetry {
			t.Fatalf("retry delay=%v want %v", delay, startupLockRetry)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatalf("mutation acquire context has no deadline")
		}
		return lease, nil
	}
	coordinator.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		return nil, nil, nil
	}
	coordinator.syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		syncCalls++
		return nil, input_config.ModeLegacy, nil
	}

	result := coordinator.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupMutation)
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

func TestCoordinateStartupBusyRemediationAcquiresAndBypassesPrepareSync(t *testing.T) {
	coordinator := newStartupCoordinator()
	lease := &fakeStartupLease{}
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	coordinator.acquire = func(ctx context.Context, _ string, delay time.Duration) (startupLease, error) {
		if delay != startupLockRetry {
			t.Fatalf("delay=%v", delay)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing deadline")
		}
		return lease, nil
	}
	coordinator.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		t.Fatal("remediation must not prepare")
		return nil, nil, nil
	}
	coordinator.syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		t.Fatal("remediation must not sync")
		return nil, input_config.ModeLegacy, nil
	}

	result := coordinator.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupRemediation)
	if result.Failure != nil || result.Lease != lease {
		t.Fatalf("result=%+v", result)
	}
	if lease.releases != 0 {
		t.Fatalf("lease releases=%d want 0", lease.releases)
	}
}

func TestCoordinateStartupBusyMutationDeadlineReturnsSyncInProgressWithoutContinuation(t *testing.T) {
	for _, class := range []startupCommandClass{startupMutation, startupRemediation} {
		t.Run(string(class), func(t *testing.T) {
			coordinator := newStartupCoordinator()
			coordinator.mutationWait = 0
			coordinator.tryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
			coordinator.acquire = func(ctx context.Context, _ string, _ time.Duration) (startupLease, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			coordinator.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
				t.Fatal("prepare after timeout")
				return nil, nil, nil
			}
			coordinator.syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
				t.Fatal("sync after timeout")
				return nil, input_config.ModeLegacy, nil
			}

			result := coordinator.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, class)
			failure := result.startupFailure()
			if failure == nil || failure.Stage != startupStageSyncLock || failure.Diagnostic.Code != compat.CodeSyncInProgress || !strings.Contains(failure.Diagnostic.Summary, "retry the command") {
				t.Fatalf("failure=%+v", failure)
			}
			if len(failure.Diagnostic.Continuation) != 0 || result.Lease != nil {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

func TestCoordinateStartupLockIOErrorIsSyncLockFailureNotContention(t *testing.T) {
	coordinator := newStartupCoordinator()
	lockErr := errors.New("lock sidecar I/O failed")
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return nil, false, lockErr }

	result := coordinator.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, startupSnapshotRead)
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
			coordinator := newStartupCoordinator()
			lease := &fakeStartupLease{}
			coordinator.tryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
			prepareErr := errors.New("prepare failed")
			syncErr := errors.New("sync failed")
			coordinator.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
				if tc.failPrepare {
					return nil, nil, prepareErr
				}
				return nil, nil, nil
			}
			coordinator.syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
				return nil, input_config.ModeLegacy, syncErr
			}

			result := coordinator.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, io.Discard, tc.class)
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
