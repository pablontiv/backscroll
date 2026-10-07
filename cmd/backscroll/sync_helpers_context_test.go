package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/spf13/cobra"
)

type cancelingSyncReader struct {
	phase       string
	cancel      context.CancelFunc
	discoveries int
	hashes      int
	parses      int
}

func (*cancelingSyncReader) Name() string { return "cancel-test" }

func (r *cancelingSyncReader) Discover(ctx context.Context, _ input_config.InputDefinition) ([]string, error) {
	r.discoveries++
	if r.phase == "discovery" {
		r.cancel()
		return nil, ctx.Err()
	}
	return []string{"cancel-test-input"}, nil
}

func (r *cancelingSyncReader) Hash(ctx context.Context, _ string) (string, error) {
	r.hashes++
	if r.phase == "hash" {
		r.cancel()
		return "", ctx.Err()
	}
	return "cancel-test-hash", nil
}

func (r *cancelingSyncReader) Parse(ctx context.Context, path string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	r.parses++
	if r.phase == "parse" {
		r.cancel()
		return models.ParsedFile{}, ctx.Err()
	}
	return models.ParsedFile{
		Path: path,
		Hash: "cancel-test-hash",
		Records: []models.Message{{
			Role:        "user",
			Origin:      models.OriginHuman,
			Content:     "cancellable startup sync",
			UUID:        "cancel-test-message",
			ContentType: "text",
		}},
	}, nil
}

func TestMaybeAutoSyncContextCancellationStopsAtEachPhase(t *testing.T) {
	originalActiveInputs := maybeAutoSyncActiveInputs
	originalNewRegistry := maybeAutoSyncNewRegistry
	originalSyncFiles := maybeAutoSyncSyncFiles
	t.Cleanup(func() {
		maybeAutoSyncActiveInputs = originalActiveInputs
		maybeAutoSyncNewRegistry = originalNewRegistry
		maybeAutoSyncSyncFiles = originalSyncFiles
	})

	maybeAutoSyncActiveInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{
			ID:     "cancel-test",
			Source: "session",
			Active: true,
			Decode: input_config.DecodeConfig{Format: "cancel-test"},
		}}, input_config.ModeDeclarative, nil
	}

	for _, phase := range []string{"discovery", "hash", "parse", "storage"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := &cancelingSyncReader{phase: phase, cancel: cancel}
			maybeAutoSyncNewRegistry = func() *readers.Registry {
				registry := readers.NewRegistry()
				registry.Register(reader)
				return registry
			}
			maybeAutoSyncSyncFiles = originalSyncFiles
			storageCalls := 0
			if phase == "storage" {
				maybeAutoSyncSyncFiles = func(ctx context.Context, db *storage.Database, files []storage.IndexedFile) error {
					storageCalls++
					cancel()
					return db.SyncFilesContext(ctx, files)
				}
			}

			dbPath := filepath.Join(t.TempDir(), "index.db")
			db, err := storage.Open(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			var progress bytes.Buffer
			err = maybeAutoSyncContext(ctx, &config.Config{DatabasePath: dbPath}, &progress)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v, want context.Canceled", err)
			}
			if progress.Len() != 0 {
				t.Fatalf("canceled sync emitted progress %q", progress.String())
			}
			if reader.discoveries != 1 {
				t.Fatalf("discoveries=%d want 1", reader.discoveries)
			}
			wantHashes, wantParses, wantStorage := 0, 0, 0
			switch phase {
			case "hash":
				wantHashes = 1
			case "parse":
				wantHashes, wantParses = 1, 1
			case "storage":
				wantHashes, wantParses, wantStorage = 1, 1, 1
			}
			if reader.hashes != wantHashes || reader.parses != wantParses || storageCalls != wantStorage {
				t.Fatalf("calls hash/parse/storage=%d/%d/%d want %d/%d/%d", reader.hashes, reader.parses, storageCalls, wantHashes, wantParses, wantStorage)
			}
		})
	}
}

func TestCanceledMutationReleasesRetainedLeaseExactlyOnce(t *testing.T) {
	for _, phase := range []string{"pre-run", "run"} {
		t.Run(phase, func(t *testing.T) {
			lease := &fakeStartupLease{}
			var stdout, stderr bytes.Buffer
			root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer, startupCommandClass) startupResult {
				result := startupResult{Config: &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, Lease: lease}
				if phase == "pre-run" {
					result.Failure = &startupFailure{
						Stage:      startupStageStartupSync,
						Cause:      context.Canceled,
						Diagnostic: compat.Diagnostic{Code: compat.CodeIndexStale, Summary: "startup canceled"},
					}
				}
				return result
			})
			if phase == "run" {
				replaceRootCommandRunEWrapped(t, root, "rebuild", func(*cobra.Command, []string) error {
					return context.Canceled
				})
			}
			root.SetArgs([]string{"rebuild"})
			if err := root.Execute(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v want context.Canceled", err)
			}
			if lease.releases != 1 {
				t.Fatalf("lease releases=%d want 1", lease.releases)
			}
		})
	}
}

func TestOwnedStartupPublishesBufferedProgressOnlyOnSuccess(t *testing.T) {
	type startupSyncContextKey struct{}
	ctx := context.WithValue(context.Background(), startupSyncContextKey{}, "startup-sync")
	for _, tc := range []struct {
		name         string
		syncErr      error
		wantProgress string
	}{
		{name: "success", wantProgress: "first\nsecond\n"},
		{name: "canceled", syncErr: context.Canceled},
		{name: "error", syncErr: errors.New("sync failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restoreStartupCoordinatorGlobals(t)
			lease := &fakeStartupLease{}
			startupTryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
			startupPrepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
				return nil, nil, nil
			}
			startupSync = func(gotCtx context.Context, _ *config.Config, progress io.Writer) error {
				if gotCtx.Value(startupSyncContextKey{}) != "startup-sync" {
					t.Fatal("startup sync did not receive coordinator context")
				}
				_, _ = io.WriteString(progress, "first\nsecond\n")
				return tc.syncErr
			}

			var progress bytes.Buffer
			result := coordinateStartup(ctx, &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, &progress, startupSnapshotRead)
			if tc.syncErr == nil {
				if result.Failure != nil {
					t.Fatalf("failure=%v", result.Failure)
				}
			} else if result.Failure == nil || !errors.Is(result.Failure, tc.syncErr) {
				t.Fatalf("failure=%v want cause %v", result.Failure, tc.syncErr)
			}
			if got := progress.String(); got != tc.wantProgress {
				t.Fatalf("progress=%q want %q", got, tc.wantProgress)
			}
			if lease.releases != 1 {
				t.Fatalf("lease releases=%d want 1", lease.releases)
			}
		})
	}
}
