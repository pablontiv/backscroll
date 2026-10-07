package main

import (
	"bytes"
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
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/recovery"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/spf13/cobra"
)

type cancelingSyncReader struct {
	name        string
	path        string
	phase       string
	parseErr    error
	cancel      context.CancelFunc
	discoveries int
	hashes      int
	parses      int
}

func (r *cancelingSyncReader) Name() string {
	if r.name != "" {
		return r.name
	}
	return "cancel-test"
}

func (r *cancelingSyncReader) Discover(ctx context.Context, _ input_config.InputDefinition) ([]string, error) {
	r.discoveries++
	if r.phase == "discovery" {
		r.cancel()
		return nil, ctx.Err()
	}
	path := r.path
	if path == "" {
		path = "cancel-test-input"
	}
	return []string{path}, nil
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
	if r.parseErr != nil {
		return models.ParsedFile{}, r.parseErr
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
	t.Parallel()
	for _, phase := range []string{"discovery", "hash", "parse", "storage"} {
		phase := phase
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := &cancelingSyncReader{phase: phase, cancel: cancel}
			syncService := newStartupSyncService()
			syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
				return []input_config.InputDefinition{{
					ID:     "cancel-test",
					Source: "session",
					Active: true,
					Decode: input_config.DecodeConfig{Format: "cancel-test"},
				}}, input_config.ModeDeclarative, nil
			}
			syncService.newRegistry = func() *readers.Registry {
				registry := readers.NewRegistry()
				registry.Register(reader)
				return registry
			}
			storageCalls := 0
			if phase == "storage" {
				syncService.syncFiles = func(ctx context.Context, db *storage.Database, files []storage.IndexedFile) error {
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
			err = syncService.sync(ctx, &config.Config{DatabasePath: dbPath}, &progress, startupPhaseTiming{})
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

func TestStartupSyncServicesIsolateReaderAndStorageDependencies(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"first-reader", "second-reader"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := "/" + name + ".jsonl"
			reader := &cancelingSyncReader{name: name, path: path}
			syncService := newStartupSyncService()
			syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
				return []input_config.InputDefinition{{ID: name, Source: "session", Active: true, Decode: input_config.DecodeConfig{Format: name}}}, input_config.ModeDeclarative, nil
			}
			syncService.newRegistry = func() *readers.Registry {
				registry := readers.NewRegistry()
				registry.Register(reader)
				return registry
			}
			syncCalls := 0
			syncFiles := syncService.syncFiles
			syncService.syncFiles = func(ctx context.Context, db *storage.Database, files []storage.IndexedFile) error {
				syncCalls++
				if len(files) != 1 || files[0].SourcePath != path {
					t.Fatalf("%s storage received files %+v", name, files)
				}
				return syncFiles(ctx, db, files)
			}

			if err := syncService.sync(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), name+".db")}, io.Discard, startupPhaseTiming{}); err != nil {
				t.Fatalf("sync: %v", err)
			}
			if reader.hashes != 1 || reader.parses != 1 || syncCalls != 1 {
				t.Fatalf("reader/storage calls=%d/%d/%d, want 1/1/1", reader.hashes, reader.parses, syncCalls)
			}
		})
	}
}

func TestMaybeAutoSyncContextPublishesSuccessfulProgressInOrder(t *testing.T) {
	t.Setenv("BACKSCROLL_STARTUP_DIAGNOSTICS", "")
	syncService := newStartupSyncService()

	dbPath := filepath.Join(t.TempDir(), "index.db")
	const sourcePath = "/success/empty-pi.jsonl"
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO indexed_files (path, hash) VALUES (?, ?)`, sourcePath, "cancel-test-hash"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reader := &cancelingSyncReader{name: "pi", path: sourcePath}
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{ID: "success-empty-pi", Source: "session", Active: true, Decode: input_config.DecodeConfig{Format: "pi"}}}, input_config.ModeDeclarative, nil
	}
	syncService.newRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(reader)
		return registry
	}

	var progress bytes.Buffer
	if err := syncService.sync(context.Background(), &config.Config{DatabasePath: dbPath}, &progress, startupPhaseTiming{}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	want := "Re-parsing empty Pi file 1: " + sourcePath + "\n"
	if got := progress.String(); got != want {
		t.Fatalf("progress=%q want %q", got, want)
	}
}

func TestStartupSyncServicesIsolateDiagnosticsTimings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		lock      time.Duration
		prepare   time.Duration
		otherLock time.Duration
	}{
		{name: "first", lock: 11 * time.Millisecond, prepare: 13 * time.Millisecond, otherLock: 21 * time.Millisecond},
		{name: "second", lock: 21 * time.Millisecond, prepare: 23 * time.Millisecond, otherLock: 11 * time.Millisecond},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			syncService := newStartupSyncService()
			syncService.diagnostics = true
			syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
				return nil, input_config.ModeLegacy, nil
			}
			var progress bytes.Buffer
			timing := startupPhaseTiming{LockAcquisitionTime: tc.lock, IndexPrepareTime: tc.prepare, measured: true}
			err := syncService.sync(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), tc.name+".db")}, &progress, timing)
			if err != nil {
				t.Fatalf("sync: %v", err)
			}
			for _, want := range []string{"  Lock Acquisition:" + tc.lock.String() + "\n", "  Index Prepare:   " + tc.prepare.String() + "\n"} {
				if !strings.Contains(progress.String(), want) {
					t.Fatalf("diagnostics=%q missing %q", progress.String(), want)
				}
			}
			if strings.Contains(progress.String(), "  Lock Acquisition:"+tc.otherLock.String()+"\n") {
				t.Fatalf("diagnostics leaked another execution's timing: %q", progress.String())
			}
		})
	}
}

func TestMaybeAutoSyncContextErrorDiscardsBufferedProgress(t *testing.T) {
	syncService := newStartupSyncService()

	dbPath := filepath.Join(t.TempDir(), "index.db")
	const sourcePath = "/error/empty-pi.jsonl"
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO indexed_files (path, hash) VALUES (?, ?)`, sourcePath, "cancel-test-hash"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	parseErr := errors.New("parse failed after progress")
	reader := &cancelingSyncReader{name: "pi", path: sourcePath, parseErr: parseErr}
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{
			ID:     "error-empty-pi",
			Source: "session",
			Active: true,
			Decode: input_config.DecodeConfig{Format: "pi"},
		}}, input_config.ModeDeclarative, nil
	}
	syncService.newRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(reader)
		return registry
	}

	var progress bytes.Buffer
	err = syncService.sync(context.Background(), &config.Config{DatabasePath: dbPath}, &progress, startupPhaseTiming{})
	if !errors.Is(err, parseErr) {
		t.Fatalf("error=%v want %v", err, parseErr)
	}
	if reader.parses != 1 {
		t.Fatalf("parse calls=%d want 1", reader.parses)
	}
	if progress.Len() != 0 {
		t.Fatalf("failed sync emitted partial progress: %q", progress.String())
	}
}

func TestRecoverCancellationDiscardsBufferedPostInstallProgress(t *testing.T) {
	t.Setenv("BACKSCROLL_STARTUP_DIAGNOSTICS", "")
	syncService := newStartupSyncService()

	dbPath := filepath.Join(t.TempDir(), "active.db")
	const sourcePath = "/recover/empty-pi.jsonl"
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO indexed_files (path, hash)
		VALUES (?, ?)
	`, sourcePath, "cancel-test-hash"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &cancelingSyncReader{name: "pi", path: sourcePath, phase: "parse", cancel: cancel}
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{
			ID:     "recover-empty-pi",
			Source: "session",
			Active: true,
			Decode: input_config.DecodeConfig{Format: "pi"},
		}}, input_config.ModeDeclarative, nil
	}
	syncService.newRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(reader)
		return registry
	}
	execute := func(context.Context, recovery.Options) (recovery.Report, error) {
		return recovery.Report{ActivePath: dbPath}, nil
	}

	cfg := &config.Config{DatabasePath: dbPath}
	lease := &fakeStartupLease{}
	var stdout, stderr bytes.Buffer
	root := buildRootCmdWithDependencies(&stdout, &stderr, func(context.Context, io.Writer, startupCommandClass) startupResult {
		return startupResult{Config: cfg, Lease: lease}
	}, syncService, execute)
	root.SetContext(ctx)
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	err = root.Execute()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context.Canceled", err)
	}
	if reader.parses != 1 {
		t.Fatalf("parse calls=%d want 1 after replay progress", reader.parses)
	}
	if strings.Contains(stderr.String(), "Re-parsing empty Pi file") {
		t.Fatalf("canceled recover emitted partial sync progress: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("canceled recover emitted report: %q", stdout.String())
	}
	if lease.releases != 1 {
		t.Fatalf("lease releases=%d want 1", lease.releases)
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

func TestOwnedStartupPreservesSuccessfulProgressBytes(t *testing.T) {
	type startupSyncContextKey struct{}
	ctx := context.WithValue(context.Background(), startupSyncContextKey{}, "startup-sync")
	dbPath := filepath.Join(t.TempDir(), "index.db")
	const sourcePath = "/startup/empty-pi.jsonl"
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO indexed_files (path, hash) VALUES (?, ?)`, sourcePath, "cancel-test-hash"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reader := &cancelingSyncReader{name: "pi", path: sourcePath}
	syncService := newStartupSyncService()
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{ID: "startup-empty-pi", Source: "session", Active: true, Decode: input_config.DecodeConfig{Format: "pi"}}}, input_config.ModeDeclarative, nil
	}
	syncService.newRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(reader)
		return registry
	}
	open := syncService.open
	syncService.open = func(gotCtx context.Context, path string) (*storage.Database, error) {
		if gotCtx.Value(startupSyncContextKey{}) != "startup-sync" {
			t.Fatal("startup sync did not receive coordinator context")
		}
		return open(gotCtx, path)
	}

	coordinator := newStartupCoordinatorWithSyncService(syncService)
	lease := &fakeStartupLease{}
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return lease, true, nil }
	coordinator.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		return nil, nil, nil
	}

	var progress bytes.Buffer
	result := coordinator.coordinate(ctx, &config.Config{DatabasePath: dbPath}, &progress, startupSnapshotRead)
	if result.Failure != nil {
		t.Fatalf("failure=%v", result.Failure)
	}
	want := "Re-parsing empty Pi file 1: " + sourcePath + "\n"
	if got := progress.String(); got != want {
		t.Fatalf("progress=%q want %q", got, want)
	}
	if lease.releases != 1 {
		t.Fatalf("lease releases=%d want 1", lease.releases)
	}
}
