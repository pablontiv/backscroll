package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestStartupAccountingPreflightFailureRendersOnce(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	inputsDir, err := input_config.InputsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(inputsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(inputsDir, "broken.inputs.toml"), []byte("active = ["), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKSCROLL_STARTUP_DIAGNOSTICS", "1")
	stdout, stderr, err := runCmd("status", "--json")
	if err == nil {
		t.Fatal("malformed manifest succeeded")
	}
	if strings.Count(stderr, "Startup diagnostics:") != 1 {
		t.Fatalf("missing unique diagnostics: %s", stderr)
	}
	_ = diagnosticDuration(t, stderr, "Total")
	if !strings.Contains(stdout, "migration_failed") {
		t.Fatal(stdout)
	}
	t.Setenv("BACKSCROLL_STARTUP_DIAGNOSTICS", "")
	off, offErr, err := runCmd("status", "--json")
	if err == nil || off != stdout {
		t.Fatalf("machine contract changed: %s / %s", stdout, off)
	}
	if strings.Contains(offErr, "Startup diagnostics:") {
		t.Fatal(offErr)
	}
}

type failingAccountingReader struct {
	accountingReader
	fail   string
	cancel context.CancelFunc
}

func (r *failingAccountingReader) failure(phase string) error {
	if r.fail == phase {
		if r.cancel != nil {
			r.cancel()
			return context.Canceled
		}
		return errors.New("injected " + phase)
	}
	return nil
}
func (r *failingAccountingReader) Discover(ctx context.Context, d input_config.InputDefinition) ([]string, error) {
	v, e := r.accountingReader.Discover(ctx, d)
	if f := r.failure("Discovery"); f != nil {
		return nil, f
	}
	return v, e
}
func (r *failingAccountingReader) Hash(ctx context.Context, p string) (string, error) {
	v, e := r.accountingReader.Hash(ctx, p)
	if f := r.failure("Hashing"); f != nil {
		return "", f
	}
	return v, e
}
func (r *failingAccountingReader) Parse(ctx context.Context, p string, d input_config.InputDefinition) (models.ParsedFile, error) {
	v, e := r.accountingReader.Parse(ctx, p, d)
	if f := r.failure("Parsing"); f != nil {
		return models.ParsedFile{}, f
	}
	return v, e
}

func TestStartupAccountingFailedSyncKeepsMeasuredPhases(t *testing.T) {
	for _, phase := range []string{"Discovery", "Hashing", "Parsing", "Database"} {
		for _, canceled := range []bool{false, true} {
			t.Run(phase+"/"+map[bool]string{false: "error", true: "canceled"}[canceled], func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("BACKSCROLL_CONFIG_DIR", t.TempDir())
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s := newStartupSyncService()
				s.diagnostics = true
				now := time.Unix(0, 0)
				s.now = func() time.Time { return now }
				advance := func(d time.Duration) { now = now.Add(d) }
				r := &failingAccountingReader{accountingReader: accountingReader{advance: advance}, fail: phase}
				if canceled {
					r.cancel = cancel
				}
				s.newRegistry = func() *readers.Registry { reg := readers.NewRegistry(); reg.Register(r); return reg }
				s.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
					return []input_config.InputDefinition{{ID: "timed", Source: "session", Active: true, Decode: input_config.DecodeConfig{Format: r.Name()}}}, input_config.ModeDeclarative, nil
				}
				s.syncFiles = func(context.Context, *storage.Database, []storage.IndexedFile) error {
					advance(400 * time.Millisecond)
					if phase == "Database" {
						if canceled {
							cancel()
							return ctx.Err()
						}
						return errors.New("injected database")
					}
					return nil
				}
				d := s.newDiagnostics()
				timing := startupPhaseTiming{report: d}
				err := s.sync(ctx, &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, &bytes.Buffer{}, timing)
				if err == nil {
					t.Fatal("injected failure succeeded")
				}
				var out bytes.Buffer
				d.render(&out)
				// Replay-queue database reads precede discovery on every attempt.
				want := map[string]time.Duration{"Database": 0, "Discovery": 100 * time.Millisecond}
				if phase != "Discovery" {
					want["Metadata"] = 0
					want["Hashing"] = 200 * time.Millisecond
				}
				if phase == "Parsing" || phase == "Database" {
					want["Parsing"] = 300 * time.Millisecond
				}
				if phase == "Database" {
					want["Database"] = 400 * time.Millisecond
				}
				if !strings.Contains(out.String(), "Phase Outcome:   "+phase+" incomplete") {
					t.Fatalf("missing failure label: %s", out.String())
				}
				total := time.Duration(0)
				for name, duration := range want {
					total += duration
					if got := diagnosticDuration(t, out.String(), name); got != duration {
						t.Fatalf("%s=%v want %v: %s", name, got, duration, out.String())
					}
				}
				if diagnosticDuration(t, out.String(), "Total") != total || diagnosticDuration(t, out.String(), "Unattributed") != 0 {
					t.Fatal(out.String())
				}
				for _, name := range []string{"Discovery", "Metadata", "Hashing", "Parsing", "Database"} {
					if _, ok := want[name]; !ok && strings.Contains(out.String(), name+":") {
						t.Fatalf("unattempted %s: %s", name, out.String())
					}
				}
			})
		}
	}
}

func TestStartupAccountingNonStartupInvocationsDoNotRender(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("BACKSCROLL_STARTUP_DIAGNOSTICS", "1")
	for _, args := range [][]string{{"--help"}, {"--version"}, {"recover"}, {"no-such-command"}} {
		_, stderr, _ := runCmd(args...)
		if strings.Contains(stderr, "Startup diagnostics:") {
			t.Fatalf("non-startup invocation %v rendered: %s", args, stderr)
		}
	}
}

func TestStartupAccountingDisabledClassMatrix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BACKSCROLL_CONFIG_DIR", t.TempDir())
	for _, class := range []startupCommandClass{startupSnapshotRead, startupMetadataRead, startupMutation, startupRemediation} {
		for _, acquired := range []bool{false, true} {
			t.Run(string(class)+"/"+map[bool]string{false: "busy", true: "owner"}[acquired], func(t *testing.T) {
				s := newStartupSyncService()
				s.diagnostics = false
				s.now = func() time.Time { t.Fatal("disabled clock read"); return time.Time{} }
				s.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
					return nil, input_config.ModeLegacy, nil
				}
				c := newStartupCoordinatorWithSyncService(s)
				c.tryAcquire = func(string) (startupLease, bool, error) { return &fakeStartupLease{}, acquired, nil }
				c.acquire = func(context.Context, string, time.Duration) (startupLease, error) { return &fakeStartupLease{}, nil }
				c.prepareIndex = func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
					return nil, nil, nil
				}
				var out bytes.Buffer
				r := c.coordinate(context.Background(), &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}, &out, class)
				if r.Failure != nil {
					t.Fatal(r.Failure)
				}
				if strings.Contains(out.String(), "Startup diagnostics:") {
					t.Fatal(out.String())
				}
				if err := r.release(nil); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
