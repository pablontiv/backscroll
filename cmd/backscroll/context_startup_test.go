package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestRootContextRunsStartupOnceAndSupportsAllFormats(t *testing.T) {
	cfg := newContextTestIndex(t)
	tests := []struct {
		name       string
		args       []string
		check      func(*testing.T, string)
		wantStderr string
	}{
		{
			name: "text",
			args: []string{"context", "--uuid", "anchor", "--before", "0", "--after", "0", "--max-tokens", "16384"},
			check: func(t *testing.T, output string) {
				t.Helper()
				if !strings.Contains(output, "Context anchor:") || !strings.Contains(output, "is_anchor: true") {
					t.Fatalf("text context output is incomplete: %q", output)
				}
			},
			wantStderr: "startup-progress\n",
		},
		{
			name: "json",
			args: []string{"context", "--uuid", "anchor", "--before", "0", "--after", "0", "--max-tokens", "16384", "--json"},
			check: func(t *testing.T, output string) {
				t.Helper()
				var envelope contextEnvelope
				if err := json.Unmarshal([]byte(output), &envelope); err != nil {
					t.Fatalf("root JSON context is invalid: %v; output=%q", err, output)
				}
				if len(envelope.Records) != 1 || !envelope.Records[0].IsAnchor {
					t.Fatalf("root JSON context envelope=%+v", envelope)
				}
			},
		},
		{
			name: "robot",
			args: []string{"context", "--uuid", "anchor", "--before", "0", "--after", "0", "--max-tokens", "16384", "--robot"},
			check: func(t *testing.T, output string) {
				t.Helper()
				fields := robotDiagnosticFields(t, output)
				if fields["records"] != "1" || fields["record_0_is_anchor"] != "true" {
					t.Fatalf("root robot context fields=%v", fields)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			startupCalls := 0
			root := buildRootCmdWithStartup(&stdout, &stderr, func(_ context.Context, progress io.Writer, class startupCommandClass) startupResult {
				startupCalls++
				if class != startupSnapshotRead {
					t.Fatalf("context startup class=%q want %q", class, startupSnapshotRead)
				}
				if _, err := io.WriteString(progress, "startup-progress\n"); err != nil {
					t.Fatalf("write startup progress: %v", err)
				}
				return startupResult{Config: cfg}
			})
			root.SetArgs(tc.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("execute root context %v: %v; stdout=%q stderr=%q", tc.args, err, stdout.String(), stderr.String())
			}
			if startupCalls != 1 {
				t.Fatalf("startup calls=%d want 1", startupCalls)
			}
			if got := stderr.String(); got != tc.wantStderr {
				t.Fatalf("stderr=%q want %q", got, tc.wantStderr)
			}
			if strings.Contains(stdout.String(), "startup-progress") {
				t.Fatalf("startup progress contaminated stdout: %q", stdout.String())
			}
			tc.check(t, stdout.String())
		})
	}
}

func TestRootContextEmitsCommandDiagnosticsInEveryFormat(t *testing.T) {
	cfg := newContextTestIndex(t)
	tests := []struct {
		name string
		args []string
		mode string
	}{
		{name: "text", args: []string{"context", "--uuid", "absent"}, mode: "text"},
		{name: "json", args: []string{"context", "--uuid", "absent", "--json"}, mode: "json"},
		{name: "robot", args: []string{"context", "--uuid", "absent", "--robot"}, mode: "robot"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			startupCalls := 0
			root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer, startupCommandClass) startupResult {
				startupCalls++
				return startupResult{Config: cfg}
			})
			root.SetArgs(tc.args)

			err := root.Execute()
			if err == nil {
				t.Fatalf("missing root context unexpectedly succeeded: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if startupCalls != 1 {
				t.Fatalf("startup calls=%d want 1", startupCalls)
			}
			switch tc.mode {
			case "text":
				if stdout.Len() != 0 || !strings.Contains(stderr.String(), "diagnostic: context_not_found:") {
					t.Fatalf("text diagnostic stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			case "json":
				if stderr.Len() != 0 {
					t.Fatalf("JSON diagnostic contaminated stderr: %q", stderr.String())
				}
				var diagnostic struct {
					Code string `json:"code"`
				}
				if decodeErr := json.Unmarshal(stdout.Bytes(), &diagnostic); decodeErr != nil || diagnostic.Code != "context_not_found" {
					t.Fatalf("JSON diagnostic code=%q decode_err=%v output=%q", diagnostic.Code, decodeErr, stdout.String())
				}
			case "robot":
				if stderr.Len() != 0 || !strings.Contains(stdout.String(), "diagnostic_code=context_not_found\n") {
					t.Fatalf("robot diagnostic stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			}
		})
	}
}

func TestRootContextBusyFollowerUsesCommittedSnapshot(t *testing.T) {
	coordinator := newStartupCoordinator()
	cfg := newContextTestIndex(t)
	originalPrepare := coordinator.prepareIndex
	prepareCalls := 0
	coordinator.tryAcquire = func(string) (startupLease, bool, error) { return nil, false, nil }
	coordinator.acquire = func(context.Context, string, time.Duration) (startupLease, error) {
		t.Fatal("snapshot reader must not wait for the startup lock")
		return nil, errors.New("unexpected startup lock wait")
	}
	coordinator.prepareIndex = func(ctx context.Context, gotCfg *config.Config, class indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
		prepareCalls++
		if class != indexDataRead {
			t.Fatalf("follower prepare class=%q want %q", class, indexDataRead)
		}
		return originalPrepare(ctx, gotCfg, class)
	}
	coordinator.syncService.open = func(context.Context, string) (*storage.Database, error) {
		t.Fatal("busy snapshot follower must not synchronize")
		return nil, nil
	}

	var stdout, stderr bytes.Buffer
	startupCalls := 0
	root := buildRootCmdWithStartup(&stdout, &stderr, func(ctx context.Context, progress io.Writer, class startupCommandClass) startupResult {
		startupCalls++
		return coordinator.coordinate(ctx, cfg, progress, class)
	})
	root.SetArgs([]string{"context", "--uuid", "anchor", "--before", "0", "--after", "0", "--max-tokens", "16384", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("busy follower context: %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if startupCalls != 1 || prepareCalls != 1 {
		t.Fatalf("startup calls=%d prepare calls=%d want 1 each", startupCalls, prepareCalls)
	}
	if !strings.Contains(stderr.String(), "warning: sync_in_progress:") || !strings.Contains(stderr.String(), "last committed index snapshot") {
		t.Fatalf("busy follower warning=%q", stderr.String())
	}
	var envelope contextEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("busy follower JSON is invalid: %v; output=%q", err, stdout.String())
	}
	if len(envelope.Records) != 1 || !envelope.Records[0].IsAnchor {
		t.Fatalf("busy follower context envelope=%+v", envelope)
	}
}

func TestInvalidRootContextDoesNotCreateDatabaseOrStartupSidecar(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing selector", args: []string{"context"}},
		{name: "positional argument", args: []string{"context", "unexpected", "--uuid", "u"}},
		{name: "both selectors", args: []string{"context", "--uuid", "u", "--source-path", "/session", "--ordinal", "1"}},
		{name: "path without ordinal", args: []string{"context", "--source-path", "/session"}},
		{name: "ordinal without path", args: []string{"context", "--ordinal", "1"}},
		{name: "empty uuid", args: []string{"context", "--uuid="}},
		{name: "empty path", args: []string{"context", "--source-path=", "--ordinal", "1"}},
		{name: "invalid before", args: []string{"context", "--uuid", "u", "--before", "-1"}},
		{name: "invalid after", args: []string{"context", "--uuid", "u", "--after", "51"}},
		{name: "invalid token budget", args: []string{"context", "--uuid", "u", "--max-tokens", "63"}},
		{name: "conflicting formats", args: []string{"context", "--uuid", "u", "--json", "--robot"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			homeDir := filepath.Join(dir, "home")
			configDir := filepath.Join(dir, "config")
			sessionsDir := filepath.Join(dir, "sessions")
			for _, path := range []string{homeDir, configDir, sessionsDir} {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", path, err)
				}
			}
			dbPath := filepath.Join(dir, "index.db")
			t.Setenv("HOME", homeDir)
			t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)
			t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
			t.Setenv("BACKSCROLL_SESSION_DIRS", sessionsDir)

			root := buildRootCmd(io.Discard, io.Discard)
			root.SetArgs(tc.args)
			if err := root.Execute(); err == nil {
				t.Fatalf("invalid context %v succeeded", tc.args)
			}
			for _, path := range []string{dbPath, dbPath + ".startup-sync.lock"} {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("invalid context created %s: stat error=%v", path, err)
				}
			}
		})
	}
}
