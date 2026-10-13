package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/startuplock"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestOMPStartupCanceledBeforeLockHasNoSideEffects(t *testing.T) {
	path, _, _ := installOMPFixture(t, false)
	path = filepath.Join(filepath.Dir(path), "untouched.db")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var progress bytes.Buffer
	result := newStartupCoordinator().coordinate(ctx, &config.Config{DatabasePath: path}, &progress, startupSnapshotRead)
	if result.Failure == nil || !errors.Is(result.Failure.Cause, context.Canceled) || result.Warning != nil || progress.Len() != 0 {
		t.Fatalf("canceled startup: %+v progress=%q", result, progress.String())
	}
	for _, name := range []string{path, path + ".startup-sync.lock"} {
		if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("canceled startup created %s: %v", name, err)
		}
	}
}

// A busy OMP owner must not make an unusable snapshot look like a successful
// empty search. The follower cannot create or migrate an index behind it.
func TestOMPBusyStartupRejectsUnusableSnapshot(t *testing.T) {
	for _, state := range []string{"missing", "invalid"} {
		t.Run(state, func(t *testing.T) {
			path, _, _ := installOMPFixture(t, false)
			path = filepath.Join(filepath.Dir(path), "untouched.db")
			if state == "invalid" {
				writeFile(t, path, "not a SQLite database")
			}
			lease, ok, err := startuplock.TryAcquire(path)
			if err != nil || !ok {
				t.Fatalf("owner lock: %t %v", ok, err)
			}
			defer lease.Release()
			var progress bytes.Buffer
			result := newStartupCoordinator().coordinate(context.Background(), &config.Config{DatabasePath: path}, &progress, startupSnapshotRead)
			if result.Failure == nil || result.Failure.Diagnostic.Code != compat.CodeSyncInProgress || result.Failure.Cause == nil || result.Warning != nil || progress.Len() != 0 {
				t.Fatalf("unsafe follower: %+v progress=%q", result, progress.String())
			}
			if state == "missing" {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("follower created index: %v", err)
				}
			} else {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "not a SQLite database" {
					t.Fatalf("follower modified index: %q %v", data, err)
				}
			}
		})
	}
}

// Use the real SQLite adapter while faulting its on-disk schema at the open
// boundary. Startup must name the failed maintenance phase, hide progress,
// and leave the OMP source unindexed rather than treating a failed queue as empty.
func TestOMPStartupMaintenanceFailuresDoNotPublishProgress(t *testing.T) {
	for _, tc := range []struct{ ddl, want string }{
		{"DROP TABLE indexed_files", "get file metadata"},
		{"DROP TABLE search_items", "discover stale paths"},
		{"ALTER TABLE search_items DROP COLUMN origin_version", "discover pending message origins"},
		{"DROP TABLE message_templates", "discover stale templates"},
		{"DROP TABLE correction_signals", "re-derive superseded correction signals"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			path, source, _ := installOMPFixture(t, false)
			db, err := storage.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			// Retain a metadata-only path so origin discovery receives a positive cap.
			if _, err := db.DB().Exec(`INSERT INTO indexed_files(path,hash) VALUES('/expired','hash')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.DB().Exec(tc.ddl); err != nil {
				t.Fatal(err)
			}
			svc := newStartupSyncService()
			svc.open = func(context.Context, string) (*storage.Database, error) { return db, nil }
			// Queue failures happen before source discovery; later maintenance faults
			// are isolated from new writes, just as a no-change OMP startup would be.
			if strings.Contains(tc.want, "templates") || strings.Contains(tc.want, "correction") {
				svc.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
					return nil, input_config.ModeDeclarative, nil
				}
			}
			var progress bytes.Buffer
			err = svc.sync(context.Background(), &config.Config{DatabasePath: path}, &progress, startupPhaseTiming{})
			if err == nil || !strings.Contains(err.Error(), tc.want) || progress.Len() != 0 {
				t.Fatalf("startup: %v progress=%q", err, progress.String())
			}
			check, err := storage.OpenReadOnly(path)
			if err != nil {
				t.Fatal(err)
			}
			defer check.Close()
			if tc.want != "discover stale paths" {
				var n int
				if err := check.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path=?`, source).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n != 0 {
					t.Fatal("failed startup published OMP records")
				}
			}
		})
	}
}

func TestOMPStartupCancellationAfterAcquiringRealLeaseReleasesIt(t *testing.T) {
	path, _, _ := installOMPFixture(t, false)
	path = filepath.Join(filepath.Dir(path), "untouched.db")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	coordinator := newStartupCoordinator()
	acquire := coordinator.tryAcquire
	coordinator.tryAcquire = func(path string) (startupLease, bool, error) {
		lease, ok, err := acquire(path)
		cancel()
		return lease, ok, err
	}
	var progress bytes.Buffer
	result := coordinator.coordinate(ctx, &config.Config{DatabasePath: path}, &progress, startupSnapshotRead)
	if result.Failure == nil || !errors.Is(result.Failure.Cause, context.Canceled) || progress.Len() != 0 {
		t.Fatalf("cancel after acquisition: %+v progress=%q", result, progress.String())
	}
	lease, ok, err := startuplock.TryAcquire(path)
	if err != nil || !ok {
		t.Fatalf("canceled owner leaked lease: %t %v", ok, err)
	}
	defer lease.Release()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled owner created index: %v", err)
	}
}
