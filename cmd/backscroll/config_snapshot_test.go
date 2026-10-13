package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/startuplock"
	"github.com/pablontiv/backscroll/internal/storage"
)

// Replace the pathname before delivering the first bytes, so any second open
// deterministically observes a different file (without sleeps or timing races).
func changingConfigFIFO(t *testing.T, path, first, second string) <-chan error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX FIFO")
	}
	if err := exec.Command("mkfifo", path).Run(); err != nil {
		t.Fatal(err)
	}
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, []byte(second), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0600)
		if err != nil {
			done <- err
			return
		}
		if err = os.Rename(replacement, path); err == nil {
			_, err = io.WriteString(f, first)
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		done <- err
	}()
	return done
}

func snapshotManifest(root, id, format string) string {
	return fmt.Sprintf("version = 1\n[[inputs]]\nid = %q\nsource = \"custom-label\"\nactive = true\n[inputs.discover]\nroots = [%q]\ninclude = [\"**/*.md\"]\n[inputs.decode]\nformat = %q\n", id, root, format)
}

func TestStartupApplicationConfigChangingFIFO(t *testing.T) {
	home, wd := t.TempDir(), t.TempDir()
	t.Chdir(wd)
	db := filepath.Join(home, "first.db")
	setIndexPolicyEnv(t, "", home)
	t.Setenv("BACKSCROLL_DATABASE_PATH", "")
	t.Setenv("BACKSCROLL_SESSION_DIRS", t.TempDir())
	done := changingConfigFIFO(t, "backscroll.toml", fmt.Sprintf("database_path = %q\n", db), "[sources]\n")
	var out, errout bytes.Buffer
	root := buildRootCmd(&out, &errout)
	root.SetArgs([]string{"config", "--json"})
	err := root.Execute()
	if writeErr := <-done; writeErr != nil {
		t.Fatal(writeErr)
	}
	if err != nil {
		t.Fatalf("startup used replacement application config: %v", err)
	}
	if !strings.Contains(out.String(), db) {
		t.Fatalf("output lost original config: %s", &out)
	}
}

func TestStartupManifestChangingFIFO(t *testing.T) {
	home, wd := t.TempDir(), t.TempDir()
	t.Chdir(wd)
	setIndexPolicyEnv(t, filepath.Join(home, "index.db"), home)
	dir := filepath.Join(home, "backscroll", "inputs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	done := changingConfigFIFO(t, filepath.Join(dir, "live.inputs.toml"), snapshotManifest(t.TempDir(), "original", "markdown_document"), snapshotManifest(t.TempDir(), "replacement", "markdown_sections"))
	var out, errout bytes.Buffer
	root := buildRootCmd(&out, &errout)
	root.SetArgs([]string{"config", "--json"})
	err := root.Execute()
	if writeErr := <-done; writeErr != nil {
		t.Fatal(writeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "original") || strings.Contains(out.String(), "replacement") {
		t.Fatalf("config reopened manifest: %s", &out)
	}
}

func TestStartupMalformedManifestHasNoSideEffects(t *testing.T) {
	for _, tc := range []struct{ name, manifest, diagnostic string }{
		{"malformed TOML", "[[inputs]\n", "parse TOML"},
		{"version", strings.ReplaceAll(snapshotManifest("/absent", "test", "claude"), "version = 1", "version = 2"), "version"},
		{"reader", snapshotManifest("/absent", "test", "unknown-reader"), "decode.format"},
		{"include", strings.ReplaceAll(snapshotManifest("/absent", "test", "claude"), "**/*.md", "["), "discover.include"},
		{"exclude", strings.ReplaceAll(snapshotManifest("/absent", "test", "claude"), "include = [\"**/*.md\"]", "include = [\"**/*.md\"]\nexclude = [\"no-match/[\"]"), "discover.exclude"},
		{"id", snapshotManifest("/absent", "", "claude"), "id"},
		{"roots", strings.ReplaceAll(snapshotManifest("/absent", "test", "claude"), "roots = [\"/absent\"]", "roots = []"), "discover.roots"},
		{"missing include", strings.ReplaceAll(snapshotManifest("/absent", "test", "claude"), "include = [\"**/*.md\"]", "include = []"), "discover.include"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			home := t.TempDir()
			dbDir := filepath.Join(home, "index-parent")
			db := filepath.Join(dbDir, "index.db")
			if err := os.MkdirAll(dbDir, 0700); err != nil {
				t.Fatal(err)
			}
			setIndexPolicyEnv(t, db, home)
			dir := filepath.Join(home, "backscroll", "inputs")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "bad.inputs.toml")
			if err := os.WriteFile(path, []byte(tc.manifest), 0600); err != nil {
				t.Fatal(err)
			}
			c := newStartupCoordinator()
			lockCalls := 0
			c.tryAcquire = func(path string) (startupLease, bool, error) { lockCalls++; return startuplock.TryAcquire(path) }
			var out, errout bytes.Buffer
			root := buildRootCmdWithCoordinator(&out, &errout, c)
			root.SetArgs([]string{"config", "--json"})
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("error = %v; want path and %s", err, tc.diagnostic)
			}
			if lockCalls != 0 {
				t.Errorf("acquired lock %d times", lockCalls)
			}
			entries, err := os.ReadDir(dbDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("lock/index/database side effects: %v, %v", entries, err)
			}
		})
	}
}

func TestStartupFileSwapSnapshotFeedsDiscoveryIngestAndConfig(t *testing.T) {
	for _, follower := range []bool{false, true} {
		t.Run(fmt.Sprintf("follower=%v", follower), func(t *testing.T) {
			t.Chdir(t.TempDir())
			home := t.TempDir()
			dbPath := filepath.Join(home, "index.db")
			setIndexPolicyEnv(t, dbPath, home)
			originalRoot, replacementRoot := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(originalRoot, "evidence.md"), []byte("snapshotoriginalsentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(replacementRoot, "evidence.md"), []byte("snapshotreplacementsentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "backscroll", "inputs")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "swap.inputs.toml")
			if err := os.WriteFile(path, []byte(snapshotManifest(originalRoot, "original", "markdown_document")), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("backscroll.toml", []byte(fmt.Sprintf("database_path = %q\n", dbPath)), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("BACKSCROLL_DATABASE_PATH", "")
			c := newStartupCoordinator()
			c.tryAcquire = func(pathArg string) (startupLease, bool, error) {
				// This boundary runs after validation, before any startup side effects.
				replacement := path + ".new"
				if err := os.WriteFile(replacement, []byte(snapshotManifest(replacementRoot, "replacement", "markdown_sections")), 0600); err != nil {
					return nil, false, err
				}
				if err := os.Rename(replacement, path); err != nil {
					return nil, false, err
				}
				if err := os.WriteFile("backscroll.toml", []byte("[sources]\n"), 0600); err != nil {
					return nil, false, err
				}
				if follower {
					return nil, false, nil
				}
				return startuplock.TryAcquire(pathArg)
			}
			var out, errout bytes.Buffer
			root := buildRootCmdWithCoordinator(&out, &errout, c)
			root.SetArgs([]string{"config", "--json"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "original") || strings.Contains(out.String(), "replacement") || !strings.Contains(out.String(), dbPath) {
				t.Fatalf("config not from snapshot: %s", &out)
			}
			if follower {
				if !strings.Contains(errout.String(), "sync_in_progress") {
					t.Fatalf("missing follower warning: %s", &errout)
				}
				if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
					t.Fatalf("metadata follower created DB: %v", err)
				}
				return
			}
			db, err := storage.OpenReadOnly(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			results, err := db.Search("snapshotoriginalsentinel", models.SearchOptions{Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 || results[0].Source != "custom-label" || results[0].SourcePath != filepath.Join(originalRoot, "evidence.md") {
				t.Fatalf("ingest lost original contract: %+v", results)
			}
			results, err = db.Search("snapshotreplacementsentinel", models.SearchOptions{Limit: 10})
			if err != nil || len(results) != 0 {
				t.Fatalf("replacement ingested: %+v, %v", results, err)
			}
		})
	}
}
