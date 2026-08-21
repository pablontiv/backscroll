package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/config"
)

func TestLegacySourcesBlockEveryExecutableCommand(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.jsonl")
	tests := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"search", "needle"}},
		{name: "list", args: []string{"list"}},
		{name: "patterns", args: []string{"patterns", "--kind", "commands"}},
		{name: "rebuild", args: []string{"rebuild"}},
		{name: "purge", args: []string{"purge", "--before", "2026-01-01"}},
		{name: "validate", args: []string{"validate"}},
		{name: "status", args: []string{"status"}},
		{name: "config", args: []string{"config"}},
		{name: "annotate", args: []string{"annotate", "--uuid", "test-uuid", "--kind", "correction", "--label", "false-positive"}},
		{name: "recover", args: []string{"recover", "--from", missingPath}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacySourcesCLIEnv(t)

			_, _, err := executeLegacySourcesCmd(tt.args...)
			if err == nil {
				t.Fatalf("%v returned nil error, want *config.LegacySourcesError", tt.args)
			}
			var legacyErr *config.LegacySourcesError
			if !errors.As(err, &legacyErr) {
				t.Fatalf("%v error = %T %[2]v, want *config.LegacySourcesError", tt.args, err)
			}
		})
	}
}

func TestLegacySourcesPreflightHasNoSideEffects(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.jsonl")
	tests := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"search", "needle"}},
		{name: "list", args: []string{"list"}},
		{name: "patterns", args: []string{"patterns", "--kind", "commands"}},
		{name: "rebuild", args: []string{"rebuild"}},
		{name: "purge", args: []string{"purge", "--before", "2026-01-01"}},
		{name: "validate", args: []string{"validate"}},
		{name: "status", args: []string{"status"}},
		{name: "config", args: []string{"config"}},
		{name: "annotate", args: []string{"annotate", "--uuid", "test-uuid", "--kind", "correction", "--label", "false-positive"}},
		{name: "recover", args: []string{"recover", "--from", missingPath}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := legacySourcesCLIEnv(t)

			_, _, err := executeLegacySourcesCmd(tt.args...)
			if err == nil {
				t.Fatalf("%v returned nil error, want *config.LegacySourcesError", tt.args)
			}
			var legacyErr *config.LegacySourcesError
			if !errors.As(err, &legacyErr) {
				t.Fatalf("%v error = %T %[2]v, want *config.LegacySourcesError", tt.args, err)
			}
			if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
				t.Fatalf("database path stat error = %v, want not exist at %s", statErr, dbPath)
			}
		})
	}
}

func TestLegacySourcesAllowsHelpAndVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "root help", args: []string{"--help"}, want: "Available Commands:"},
		{name: "root version", args: []string{"--version"}, want: "backscroll"},
		{name: "subcommand help", args: []string{"search", "--help"}, want: "Search performs a hybrid search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacySourcesCLIEnv(t)

			stdout, stderr, err := executeLegacySourcesCmd(tt.args...)
			if err != nil {
				t.Fatalf("%v returned error: %v\nstderr=%s", tt.args, err, stderr)
			}
			out := stdout + stderr
			if !strings.Contains(out, tt.want) {
				t.Fatalf("%v output = %q, want it to contain %q", tt.args, out, tt.want)
			}
		})
	}
}

func legacySourcesCLIEnv(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	configBase := filepath.Join(root, "config")
	wd := filepath.Join(root, "work")
	for _, path := range []string{home, configBase, wd} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", path, err)
		}
	}

	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configBase)
	dbPath := filepath.Join(root, "missing", "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	t.Chdir(wd)

	globalConfig := filepath.Join(home, ".config", "backscroll", "config.toml")
	if err := os.MkdirAll(filepath.Dir(globalConfig), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(globalConfig), err)
	}
	if err := os.WriteFile(globalConfig, []byte("[sources]\nke = [\"legacy-ke\"]\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", globalConfig, err)
	}

	return dbPath
}

func executeLegacySourcesCmd(args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	root := buildRootCmd(&stdout, &stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}
