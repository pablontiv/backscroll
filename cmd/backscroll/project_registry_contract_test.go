package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryFatalBeforeStartupSideEffects(t *testing.T) {
	for _, command := range [][]string{{"status", "--json"}, {"search", "--text", "needle", "--all-projects"}, {"rebuild"}, {"config", "--json"}} {
		for _, state := range []string{"malformed", "directory", "unreadable"} {
			t.Run(strings.Join(command, "_")+"/"+state, func(t *testing.T) {
				dbPath, cleanup := testEnv(t)
				defer cleanup()
				if err := os.Remove(dbPath); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(os.Getenv("HOME"), ".config", "backscroll", "projects.toml")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if state == "directory" {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				} else {
					mode := os.FileMode(0600)
					if state == "unreadable" {
						mode = 0
					}
					if err := os.WriteFile(path, []byte("not valid toml ==="), mode); err != nil {
						t.Fatal(err)
					}
					if state == "unreadable" {
						if _, err := os.ReadFile(path); err == nil {
							t.Skip("permissions bypassed by execution identity")
						}
					}
				}
				var out, stderr bytes.Buffer
				if code := run(&out, &stderr, command); code == nil {
					t.Fatalf("expected fatal registry error, stdout=%s stderr=%s", &out, &stderr)
				}
				if !strings.Contains(out.String()+stderr.String(), path) {
					t.Fatalf("missing actionable registry path: %s", &stderr)
				}
				for _, suffix := range []string{"", "-wal", "-shm", ".startup-sync.lock"} {
					if _, err := os.Stat(dbPath + suffix); !os.IsNotExist(err) {
						t.Fatalf("unexpected startup artifact %s: %v", dbPath+suffix, err)
					}
				}
			})
		}
	}
}

func TestRegistryAbsentAndValidStartupRebuild(t *testing.T) {
	for _, valid := range []bool{false, true} {
		name := "absent"
		if valid {
			name = "valid"
		}
		t.Run(name, func(t *testing.T) {
			dbPath, cleanup := testEnv(t)
			defer cleanup()
			cwd := filepath.Join(t.TempDir(), "fallback-project")
			if err := os.MkdirAll(cwd, 0700); err != nil {
				t.Fatal(err)
			}
			old := currentWorkingDirectory
			currentWorkingDirectory = func() (string, error) { return cwd, nil }
			defer func() { currentWorkingDirectory = old }()
			want := "fallback-project"
			if valid {
				path := filepath.Join(os.Getenv("HOME"), ".config", "backscroll", "projects.toml")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(fmt.Sprintf("[[projects]]\nid = %q\nroots = [%q]\n", "canonical", cwd)), 0600); err != nil {
					t.Fatal(err)
				}
				want = "canonical"
			}
			if got, err := effectiveProject("", false); err != nil || got != want {
				t.Fatalf("scope: %q, %v; want %s", got, err, want)
			}
			sessions := t.TempDir()
			t.Setenv("BACKSCROLL_SESSION_DIRS", sessions)
			record := fmt.Sprintf(`{"type":"user","uuid":"registry-message","sessionId":"registry-session","cwd":%q,"timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"registrycontractprobe"}}`, cwd)
			if err := os.WriteFile(filepath.Join(sessions, "session.jsonl"), []byte(record+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{"status", "rebuild"} {
				var out, stderr bytes.Buffer
				if err := run(&out, &stderr, []string{command}); err != nil {
					t.Fatalf("%s: %v, %s", command, err, &stderr)
				}
				db, err := storage.OpenReadOnly(dbPath)
				if err != nil {
					t.Fatal(err)
				}
				var got string
				err = db.DB().QueryRow("SELECT project FROM search_items WHERE text = 'registrycontractprobe'").Scan(&got)
				db.Close()
				if err != nil || got != want {
					t.Fatalf("%s attribution: %q, %v; want %s", command, got, err, want)
				}
			}
		})
	}
}

// Direct runners must validate even when invoked outside the root startup policy.
func TestRegistryDirectRunnersRejectBeforeOpeningDatabase(t *testing.T) {
	for _, state := range []string{"malformed", "directory", "unreadable"} {
		t.Run(state, func(t *testing.T) {
			dbPath, cleanup := testEnv(t)
			defer cleanup()
			before, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(os.Getenv("HOME"), ".config", "backscroll", "projects.toml")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if state == "directory" {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				mode := os.FileMode(0600)
				if state == "unreadable" {
					mode = 0
				}
				if err := os.WriteFile(path, []byte("not valid toml ==="), mode); err != nil {
					t.Fatal(err)
				}
				if state == "unreadable" {
					if _, err := os.ReadFile(path); err == nil {
						t.Skip("permissions bypassed by execution identity")
					}
				}
			}
			cfg := &config.Config{DatabasePath: dbPath}
			if got, err := effectiveProject("", false); err == nil || got != "" {
				t.Fatalf("scope silently fell back: %q, %v", got, err)
			}
			service := newStartupSyncService()
			service.open = func(context.Context, string) (*storage.Database, error) {
				t.Fatal("ingest opened database before registry validation")
				return nil, nil
			}
			if err := service.sync(context.Background(), cfg, io.Discard, startupPhaseTiming{}); err == nil {
				t.Fatal("ingest accepted invalid registry")
			}
			runner := newRebuildRunner(&rebuildDependencies{prepareIndex: func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
				t.Fatal("rebuild prepared database before registry validation")
				return nil, nil, nil
			}})
			if err := runner.runRebuild(context.Background(), io.Discard, io.Discard, cfg); err == nil {
				t.Fatal("rebuild accepted invalid registry")
			}
			var out, stderr bytes.Buffer
			if err := run(&out, &stderr, []string{"rebuild"}); err == nil {
				t.Fatal("CLI rebuild accepted invalid registry")
			}
			after, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("fatal registry changed existing database")
			}
		})
	}
}
