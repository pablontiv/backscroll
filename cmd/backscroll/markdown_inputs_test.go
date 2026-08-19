package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

func TestDeclarativeMarkdownSectionsAutoSyncAndSearch(t *testing.T) {
	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "home")
	configDir := filepath.Join(tmp, "config")
	workDir := filepath.Join(tmp, "work")
	decisionRoot := filepath.Join(tmp, "decisions")
	dbPath := filepath.Join(tmp, "backscroll.db")

	for _, path := range []string{homeDir, configDir, workDir, decisionRoot} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	t.Chdir(workDir)

	inputsDir := filepath.Join(configDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatalf("mkdir inputs dir: %v", err)
	}
	manifest := fmt.Sprintf(`version = 1

[[inputs]]
id = "decisions-e2e"
source = "decision"
active = true

[inputs.discover]
roots = [%q]
include = ["**/*.md"]
exclude = []
follow_symlinks = false

[inputs.decode]
format = "markdown_sections"
`, decisionRoot)
	manifestPath := filepath.Join(inputsDir, "decisions.inputs.toml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	uniqueTerm := "hyperionfluxneedle"
	decisionPath := filepath.Join(decisionRoot, "adr-0001.md")
	decisionMarkdown := fmt.Sprintf(`# ADR 0001

## Accepted Migration

Use the declarative markdown_sections reader for %s decisions.

## Rejected Context

This section must not be returned by a section-filtered search.
`, uniqueTerm)
	if err := os.WriteFile(decisionPath, []byte(decisionMarkdown), 0o644); err != nil {
		t.Fatalf("write decision markdown: %v", err)
	}

	stdout, stderr, err := executeMarkdownInputsCmd("search", "--all-projects", "--source", "decision", uniqueTerm)
	if err != nil {
		t.Fatalf("first search error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Source: decision") {
		t.Fatalf("search output missing source filter evidence; stdout=%s", stdout)
	}
	if !strings.Contains(stdout, "## Accepted Migration") || !strings.Contains(stdout, uniqueTerm) {
		t.Fatalf("search output missing expected matching section; stdout=%s", stdout)
	}
	if strings.Contains(stdout, "## Rejected Context") {
		t.Fatalf("search output included non-matching section, want markdown_sections isolation; stdout=%s", stdout)
	}

	before := countMarkdownInputRows(t, dbPath)
	stdout, stderr, err = executeMarkdownInputsCmd("search", "--all-projects", "--source", "decision", uniqueTerm)
	if err != nil {
		t.Fatalf("second search error: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	after := countMarkdownInputRows(t, dbPath)
	if after != before {
		t.Fatalf("second autosync changed indexed counts: before=%+v after=%+v", before, after)
	}
}

type markdownInputCounts struct {
	IndexedFiles int
	SearchItems  int
}

func countMarkdownInputRows(t *testing.T, dbPath string) markdownInputCounts {
	t.Helper()
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db for counts: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close db for counts: %v", err)
		}
	}()

	var counts markdownInputCounts
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files`).Scan(&counts.IndexedFiles); err != nil {
		t.Fatalf("count indexed_files: %v", err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items`).Scan(&counts.SearchItems); err != nil {
		t.Fatalf("count search_items: %v", err)
	}
	return counts
}

func executeMarkdownInputsCmd(args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	root := buildRootCmd(&stdout, &stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}
