package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
)

func diagnosticDuration(t *testing.T, output, name string) time.Duration {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if tail, ok := strings.CutPrefix(strings.TrimSpace(line), name+":"); ok {
			fields := strings.Fields(tail)
			if len(fields) == 0 {
				t.Fatalf("empty %s", name)
			}
			d, err := time.ParseDuration(fields[0])
			if err != nil {
				t.Fatal(err)
			}
			return d
		}
	}
	t.Fatalf("missing %s in %s", name, output)
	return 0
}

func TestStartupAccountingIncludesLockAndPrepare(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BACKSCROLL_CONFIG_DIR", t.TempDir())
	s := newStartupSyncService()
	s.diagnostics = true
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	s.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return nil, input_config.ModeLegacy, nil
	}
	c := newStartupCoordinatorWithSyncService(s)
	c.tryAcquire = func(string) (startupLease, bool, error) {
		now = now.Add(time.Second)
		return &fakeStartupLease{}, true, nil
	}
	c.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		now = now.Add(time.Second)
		return nil, nil, nil
	}
	var out bytes.Buffer
	r := c.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, &out, startupSnapshotRead)
	if r.Failure != nil {
		t.Fatal(r.Failure)
	}
	if got := diagnosticDuration(t, out.String(), "Lock Acquisition"); got != time.Second {
		t.Fatalf("lock=%v", got)
	}
	if got := diagnosticDuration(t, out.String(), "Index Prepare"); got != time.Second {
		t.Fatalf("prepare=%v", got)
	}
	total := diagnosticDuration(t, out.String(), "Total")
	if total != 2*time.Second {
		t.Fatalf("total=%v want 2s", total)
	}
	sum := time.Duration(0)
	for _, phase := range []string{"Lock Acquisition", "Index Prepare", "Discovery", "Metadata", "Hashing", "Parsing", "Database", "Unattributed"} {
		sum += diagnosticDuration(t, out.String(), phase)
	}
	if total != sum {
		t.Fatalf("total %v != phases %v: %s", total, sum, out.String())
	}
}

func TestStartupAccountingApplicablePaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BACKSCROLL_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name           string
		class          startupCommandClass
		acquired, wait bool
		phases         []string
	}{
		{"snapshot-follower", startupSnapshotRead, false, false, []string{"Lock Acquisition", "Index Prepare"}},
		{"metadata-follower", startupMetadataRead, false, false, []string{"Lock Acquisition"}},
		{"metadata-owner", startupMetadataRead, true, false, []string{"Lock Acquisition", "Index Prepare", "Discovery", "Metadata", "Hashing", "Parsing", "Database"}},
		{"mutation-waiter", startupMutation, false, true, []string{"Lock Acquisition", "Index Prepare", "Discovery", "Metadata", "Hashing", "Parsing", "Database"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStartupSyncService()
			s.diagnostics = true
			now := time.Unix(0, 0)
			s.now = func() time.Time { return now }
			s.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
				return nil, input_config.ModeLegacy, nil
			}
			c := newStartupCoordinatorWithSyncService(s)
			c.tryAcquire = func(string) (startupLease, bool, error) {
				now = now.Add(time.Second)
				return &fakeStartupLease{}, tc.acquired, nil
			}
			c.acquire = func(context.Context, string, time.Duration) (startupLease, error) {
				now = now.Add(2 * time.Second)
				return &fakeStartupLease{}, nil
			}
			c.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
				now = now.Add(time.Second)
				return nil, nil, nil
			}
			var out bytes.Buffer
			r := c.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, &out, tc.class)
			if r.Failure != nil {
				t.Fatal(r.Failure)
			}
			for _, phase := range []string{"Lock Acquisition", "Index Prepare", "Discovery", "Metadata", "Hashing", "Parsing", "Database"} {
				want := false
				for _, p := range tc.phases {
					if p == phase {
						want = true
					}
				}
				if strings.Contains(out.String(), phase+":") != want {
					t.Fatalf("phase %s applicability: %s", phase, out.String())
				}
			}
			wantTotal := time.Second
			if tc.name != "metadata-follower" {
				wantTotal += time.Second
			}
			if tc.wait {
				wantTotal += 2 * time.Second
			}
			if got := diagnosticDuration(t, out.String(), "Total"); got != wantTotal {
				t.Fatalf("total=%v want %v", got, wantTotal)
			}
			if tc.wait && diagnosticDuration(t, out.String(), "Lock Acquisition") != 3*time.Second {
				t.Fatal("initial attempt omitted from wait")
			}
			if strings.Contains(out.String(), "Overlap:") {
				t.Fatal(out.String())
			}
			_ = r.release(nil)
		})
	}
}

func TestStartupAccountingDisabledDoesNotReadClock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BACKSCROLL_CONFIG_DIR", t.TempDir())
	s := newStartupSyncService()
	s.diagnostics = false
	s.now = func() time.Time { t.Fatal("disabled diagnostics read clock"); return time.Time{} }
	s.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return nil, input_config.ModeLegacy, nil
	}
	c := newStartupCoordinatorWithSyncService(s)
	c.tryAcquire = func(string) (startupLease, bool, error) { return &fakeStartupLease{}, true, nil }
	c.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		return nil, nil, nil
	}
	var out bytes.Buffer
	r := c.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, &out, startupSnapshotRead)
	if r.Failure != nil {
		t.Fatal(r.Failure)
	}
	if strings.Contains(out.String(), "Startup diagnostics:") {
		t.Fatal(out.String())
	}
}

func TestStartupAccountingRecovery(t *testing.T) {
	for _, dry := range []bool{true, false} {
		t.Run(strconv.FormatBool(dry), func(t *testing.T) {
			_, cleanup := testEnv(t)
			defer cleanup()
			s := newStartupSyncService()
			s.diagnostics = true
			now := time.Unix(0, 0)
			s.now = func() time.Time { return now }
			s.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
				return nil, input_config.ModeLegacy, nil
			}
			c := newStartupCoordinatorWithSyncService(s)
			c.tryAcquire = func(string) (startupLease, bool, error) {
				now = now.Add(time.Second)
				return &fakeStartupLease{}, true, nil
			}
			c.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
				t.Fatal("recovery prepared index")
				return nil, nil, nil
			}
			var out, errout bytes.Buffer
			root := buildRootCmdWithDependencies(&out, &errout, c.defaultStartupPolicy, s, func(context.Context, recovery.Options) (recovery.Report, error) {
				now = now.Add(3 * time.Second)
				return recovery.Report{}, nil
			})
			args := []string{"recover", "--from", "stranded.db"}
			if dry {
				args = append(args, "--dry-run")
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if strings.Count(errout.String(), "Startup diagnostics:") != 1 {
				t.Fatal(errout.String())
			}
			if strings.Contains(errout.String(), "Index Prepare:") {
				t.Fatal(errout.String())
			}
			if strings.Contains(errout.String(), "Discovery:") == dry {
				t.Fatal(errout.String())
			}
			if diagnosticDuration(t, errout.String(), "Total") != 4*time.Second {
				t.Fatal(errout.String())
			}
			if diagnosticDuration(t, errout.String(), "Unattributed") != 3*time.Second {
				t.Fatal(errout.String())
			}
		})
	}
}

func TestStartupAccountingReportsOverlapExplicitly(t *testing.T) {
	s := newStartupSyncService()
	s.diagnostics = true
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	d := s.newDiagnostics()
	d.hasLock = true
	d.lock = time.Second
	now = now.Add(500 * time.Millisecond)
	var out bytes.Buffer
	d.render(&out)
	if diagnosticDuration(t, out.String(), "Overlap") != 500*time.Millisecond || diagnosticDuration(t, out.String(), "Unattributed") != 0 {
		t.Fatal(out.String())
	}
}

func TestStartupAccountingFailureReportsOnlyCompletedPhases(t *testing.T) {
	s := newStartupSyncService()
	s.diagnostics = true
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	c := newStartupCoordinatorWithSyncService(s)
	c.tryAcquire = func(string) (startupLease, bool, error) {
		now = now.Add(time.Second)
		return nil, false, errors.New("lock failed")
	}
	var out bytes.Buffer
	r := c.coordinate(context.Background(), &config.Config{DatabasePath: "unused"}, &out, startupMutation)
	if r.Failure == nil {
		t.Fatal("missing failure")
	}
	if diagnosticDuration(t, out.String(), "Total") != time.Second || strings.Contains(out.String(), "Index Prepare:") {
		t.Fatal(out.String())
	}
}

func BenchmarkStartupAccountingDisabled(b *testing.B) {
	s := newStartupSyncService()
	s.diagnostics = false
	c := newStartupCoordinatorWithSyncService(s)
	c.tryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	cfg := &config.Config{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.coordinate(context.Background(), cfg, io.Discard, startupMetadataRead)
	}
}

type accountingLease struct{ advance func() }

func (l accountingLease) Release() error { l.advance(); return nil }

type accountingReader struct {
	cancelingSyncReader
	advance func(time.Duration)
}

func (r *accountingReader) Discover(ctx context.Context, def input_config.InputDefinition) ([]string, error) {
	r.advance(100 * time.Millisecond)
	return r.cancelingSyncReader.Discover(ctx, def)
}
func (r *accountingReader) Hash(ctx context.Context, path string) (string, error) {
	r.advance(200 * time.Millisecond)
	return r.cancelingSyncReader.Hash(ctx, path)
}
func (r *accountingReader) Parse(ctx context.Context, path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	r.advance(300 * time.Millisecond)
	return r.cancelingSyncReader.Parse(ctx, path, def)
}

func TestStartupAccountingSequentialPhasesAndHandoff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BACKSCROLL_CONFIG_DIR", t.TempDir())
	s := newStartupSyncService()
	s.diagnostics = true
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	advance := func(d time.Duration) { now = now.Add(d) }
	report := s.newDiagnostics()
	advance(700 * time.Millisecond)
	ctx := context.WithValue(context.Background(), startupDiagnosticsContextKey{}, report)
	path := filepath.Join(t.TempDir(), "input.jsonl")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	r := &accountingReader{cancelingSyncReader: cancelingSyncReader{path: path}, advance: advance}
	s.newRegistry = func() *readers.Registry { reg := readers.NewRegistry(); reg.Register(r); return reg }
	s.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{ID: "timed", Source: "session", Active: true, Decode: input_config.DecodeConfig{Format: r.Name()}}}, input_config.ModeDeclarative, nil
	}
	open := s.open
	s.open = func(ctx context.Context, path string) (*storage.Database, error) {
		advance(500 * time.Millisecond)
		return open(ctx, path)
	}
	sync := s.syncFiles
	s.syncFiles = func(ctx context.Context, db *storage.Database, files []storage.IndexedFile) error {
		advance(400 * time.Millisecond)
		return sync(ctx, db, files)
	}
	c := newStartupCoordinatorWithSyncService(s)
	c.tryAcquire = func(string) (startupLease, bool, error) {
		advance(time.Second)
		return accountingLease{func() { advance(600 * time.Millisecond) }}, true, nil
	}
	c.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		advance(time.Second)
		return nil, nil, nil
	}
	var out bytes.Buffer
	result := c.coordinate(ctx, &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, &out, startupSnapshotRead)
	if result.Failure != nil {
		t.Fatal(result.Failure)
	}
	for phase, want := range map[string]time.Duration{"Lock Acquisition": time.Second, "Index Prepare": time.Second, "Discovery": 100 * time.Millisecond, "Metadata": 0, "Hashing": 200 * time.Millisecond, "Parsing": 300 * time.Millisecond, "Database": 400 * time.Millisecond, "Unattributed": 1800 * time.Millisecond, "Total": 4800 * time.Millisecond} {
		if got := diagnosticDuration(t, out.String(), phase); got != want {
			t.Fatalf("%s=%v want %v: %s", phase, got, want, out.String())
		}
	}
	if !strings.Contains(out.String(), "1 files hashed, 0 files skipped") || strings.Contains(out.String(), "Overlap:") {
		t.Fatal(out.String())
	}
}

func TestStartupAccountingCanceledLockIsMeasured(t *testing.T) {
	s := newStartupSyncService()
	s.diagnostics = true
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newStartupCoordinatorWithSyncService(s)
	c.tryAcquire = func(string) (startupLease, bool, error) {
		now = now.Add(time.Second)
		cancel()
		return nil, false, ctx.Err()
	}
	var out bytes.Buffer
	r := c.coordinate(ctx, &config.Config{DatabasePath: "unused"}, &out, startupSnapshotRead)
	if r.Failure == nil {
		t.Fatal("missing cancellation")
	}
	if diagnosticDuration(t, out.String(), "Lock Acquisition") != time.Second {
		t.Fatal(out.String())
	}
}
