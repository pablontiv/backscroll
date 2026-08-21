package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestStaleIndexBlocksIndexBackedCommands(t *testing.T) {
	cases := []struct {
		name     string
		argv     []string
		mutation bool
	}{
		{"search", []string{"search", "sentinel"}, false},
		{"search-json", []string{"search", "sentinel", "--json"}, false},
		{"search-robot", []string{"search", "sentinel", "--robot"}, false},
		{"list", []string{"list"}, false},
		{"list-json", []string{"list", "--json"}, false},
		{"patterns", []string{"patterns", "--kind", "commands"}, false},
		{"rebuild", []string{"rebuild"}, true},
		{"purge", []string{"purge", "--before", "2030-01-01"}, true},
		{"annotate", []string{"annotate", "--uuid", "u", "--kind", "correction", "--label", "x"}, true},
	}

	ran := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ran++
			dbPath := newUnsupportedIndexedConsumerDB(t)
			before := readDBBytes(t, dbPath)

			var stdout, stderr bytes.Buffer
			err := run(&stdout, &stderr, tc.argv)
			if err == nil {
				t.Fatalf("%v succeeded; stdout=%q stderr=%q", tc.argv, stdout.String(), stderr.String())
			}

			combined := stdout.String() + stderr.String()
			if strings.Contains(combined, "sentinel") {
				t.Fatalf("%v emitted cached sentinel output after stale refusal; stdout=%q stderr=%q", tc.argv, stdout.String(), stderr.String())
			}
			if !strings.Contains(combined, string(compat.CodeUnsupportedLineage)) {
				t.Fatalf("%v did not emit typed diagnostic %q; stdout=%q stderr=%q err=%v", tc.argv, compat.CodeUnsupportedLineage, stdout.String(), stderr.String(), err)
			}
			wantContinuation := []string{"recover", "--from", dbPath, "--dry-run"}
			for _, part := range wantContinuation {
				if !strings.Contains(combined, part) {
					t.Fatalf("%v diagnostic missing continuation part %q; stdout=%q stderr=%q", tc.argv, part, stdout.String(), stderr.String())
				}
			}
			if strings.Contains(combined, "<resolved-active-path>") || strings.Contains(combined, "--from  --dry-run") {
				t.Fatalf("%v emitted placeholder/empty continuation; stdout=%q stderr=%q", tc.argv, stdout.String(), stderr.String())
			}

			if tc.mutation {
				after := readDBBytes(t, dbPath)
				if !bytes.Equal(after, before) {
					t.Fatalf("%v mutated the unsupported database before policy refusal", tc.argv)
				}
			}
		})
	}
	if ran != len(cases) {
		t.Fatalf("ran %d cases, want %d", ran, len(cases))
	}
}

func TestMachineModesCarryDiagnosticCodeAndContinuation(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"json", []string{"search", "sentinel", "--json"}},
		{"robot", []string{"search", "sentinel", "--robot"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := newUnsupportedIndexedConsumerDB(t)
			var stdout, stderr bytes.Buffer
			err := run(&stdout, &stderr, tc.argv)
			if err == nil {
				t.Fatalf("machine mode succeeded; stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("machine diagnostic contaminated stderr: %q", stderr.String())
			}
			if strings.Contains(stdout.String(), "sentinel") {
				t.Fatalf("machine diagnostic emitted cached sentinel rows: %q", stdout.String())
			}
			switch tc.name {
			case "json":
				var got struct {
					Code         string   `json:"code"`
					Continuation []string `json:"continuation_argv"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
					t.Fatalf("invalid JSON diagnostic %q: %v", stdout.String(), err)
				}
				assertDiagnosticFields(t, got.Code, got.Continuation, dbPath)
			case "robot":
				out := stdout.String()
				if !strings.Contains(out, "diagnostic_code="+string(compat.CodeUnsupportedLineage)) {
					t.Fatalf("robot diagnostic missing code: %q", out)
				}
				wantContinuation := fmt.Sprintf(`diagnostic_continuation_argv=["recover","--from","%s","--dry-run"]`, dbPath)
				if !strings.Contains(out, wantContinuation) {
					t.Fatalf("robot diagnostic missing encoded continuation %q: %q", wantContinuation, out)
				}
			}
		})
	}
}

func TestPrepareIndexDataReadReturnsReadOnlyConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	writer, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	db, diag, err := prepareIndex(context.Background(), &config.Config{DatabasePath: dbPath}, indexDataRead)
	if err != nil || diag != nil {
		t.Fatalf("prepare read db=%v diag=%+v err=%v", db, diag, err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(`CREATE TABLE must_not_exist (id INTEGER)`); err == nil {
		t.Fatal("indexDataRead returned a write-capable connection")
	}
}

func TestPrepareIndexDataReadDoesNotApplyPendingMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	writer, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.DB().Exec(`DELETE FROM schema_migrations WHERE version = 13`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	db, diag, err := prepareIndex(context.Background(), &config.Config{DatabasePath: dbPath}, indexDataRead)
	if db != nil {
		_ = db.Close()
		t.Fatal("read preparation returned DB requiring migration")
	}
	if err == nil && diag == nil {
		t.Fatal("read preparation accepted pending migration")
	}

	inspect, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	var count int
	if err := inspect.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 13`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("read path applied migration 13 count=%d", count)
	}
}

func TestSnapshotReadCommandsDoNotCreateMissingDatabase(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{name: "list", argv: []string{"list"}},
		{name: "search", argv: []string{"search", "sentinel"}},
		{name: "patterns", argv: []string{"patterns", "--kind", "commands"}},
		{name: "status", argv: []string{"status"}},
		{name: "validate", argv: []string{"validate"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "missing", "index.db")
			setIndexPolicyEnv(t, dbPath, filepath.Join(dir, "config"))

			var stdout, stderr bytes.Buffer
			err := run(&stdout, &stderr, tc.argv)
			if err == nil {
				t.Fatalf("%v succeeded against missing DB; stdout=%q stderr=%q", tc.argv, stdout.String(), stderr.String())
			}
			assertMissingDatabaseArtifacts(t, dbPath)
			combined := stdout.String() + stderr.String()
			if !strings.Contains(combined, string(compat.CodeMigrationFailed)) {
				t.Fatalf("%v missing diagnostic code %q; stdout=%q stderr=%q err=%v", tc.argv, compat.CodeMigrationFailed, stdout.String(), stderr.String(), err)
			}
		})
	}
}

func TestConfigDoesNotCreateMissingDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "missing", "index.db")
	setIndexPolicyEnv(t, dbPath, filepath.Join(dir, "config"))

	var stdout, stderr bytes.Buffer
	if err := run(&stdout, &stderr, []string{"config", "--json"}); err != nil {
		t.Fatalf("config failed: %v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	assertMissingDatabaseArtifacts(t, dbPath)
	if !strings.Contains(stdout.String(), dbPath) {
		t.Fatalf("config output missing db path %q: stdout=%q", dbPath, stdout.String())
	}
}

func TestHumanDiagnosticRenderedOnceWithoutCobraEcho(t *testing.T) {
	dbPath := newUnsupportedIndexedConsumerDB(t)
	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, []string{"search", "sentinel"})
	if err == nil {
		t.Fatalf("human diagnostic command succeeded; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "sentinel") {
		t.Fatalf("human diagnostic emitted cached sentinel rows: %q", stdout.String())
	}
	diagnosticLines := 0
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.HasPrefix(line, "diagnostic:") {
			diagnosticLines++
		}
	}
	if diagnosticLines != 1 {
		t.Fatalf("diagnostic lines=%d, want 1; stderr=%q", diagnosticLines, stderr.String())
	}
	if strings.Contains(stderr.String(), "Error:") {
		t.Fatalf("stderr duplicated by Cobra error echo: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "continuation: recover --from "+dbPath+" --dry-run") {
		t.Fatalf("stderr missing continuation path: %q", stderr.String())
	}
}

func TestMandatoryStartupIndexesMarkdownDocumentForSearch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	root := filepath.Join(t.TempDir(), "notes")
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	writeInputManifest(t, root, "markdown_document", root, []string{"*.md"}, nil)
	path := filepath.Join(root, "decision.md")
	writeFile(t, path, "# Decision\n\nperennial sqlite sentinel\n")

	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, []string{"search", "perennial sqlite sentinel", "--all-projects", "--source-path", path, "--json"})
	if err != nil {
		t.Fatalf("search: %v stderr=%q", err, stderr.String())
	}
	var rows []minimalSearchResult
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout.String(), err)
	}
	if len(rows) != 1 || rows[0].SourcePath != path {
		t.Fatalf("rows=%+v, want indexed markdown path %s", rows, path)
	}
}

func TestMandatoryStartupMarkdownSearchSurvivesMissingSourceFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	root := filepath.Join(t.TempDir(), "notes")
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	writeInputManifest(t, root, "markdown_document", root, []string{"*.md"}, nil)
	path := filepath.Join(root, "decision.md")
	writeFile(t, path, "# Decision\n\nperennial sqlite sentinel\n")

	// First run must trigger mandatory startup ingestion from markdown.
	var firstStdout, firstStderr bytes.Buffer
	if err := run(&firstStdout, &firstStderr, []string{"search", "perennial sqlite sentinel", "--all-projects", "--source-path", path, "--json"}); err != nil {
		t.Fatalf("initial search: %v stdout=%q stderr=%q", err, firstStdout.String(), firstStderr.String())
	}
	var firstRows []minimalSearchResult
	if err := json.Unmarshal(firstStdout.Bytes(), &firstRows); err != nil {
		t.Fatalf("initial invalid JSON %q: %v", firstStdout.String(), err)
	}
	if len(firstRows) != 1 || firstRows[0].SourcePath != path {
		t.Fatalf("initial rows=%+v, want indexed markdown path %s", firstRows, path)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove markdown source: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("source path still available after removal: stat err=%v", err)
	}

	// Second run proves retrieval from the perennial SQLite index via public search API,
	// even when the original markdown file is unavailable.
	var secondStdout, secondStderr bytes.Buffer
	if err := run(&secondStdout, &secondStderr, []string{"search", "perennial sqlite sentinel", "--all-projects", "--source-path", path, "--json"}); err != nil {
		t.Fatalf("search after source removal: %v stdout=%q stderr=%q", err, secondStdout.String(), secondStderr.String())
	}
	var secondRows []minimalSearchResult
	if err := json.Unmarshal(secondStdout.Bytes(), &secondRows); err != nil {
		t.Fatalf("post-removal invalid JSON %q: %v", secondStdout.String(), err)
	}
	if len(secondRows) != 1 || secondRows[0].SourcePath != path {
		t.Fatalf("post-removal rows=%+v, want indexed markdown path %s", secondRows, path)
	}
	if !strings.Contains(secondRows[0].Snippet, "sqlite sentinel") {
		t.Fatalf("post-removal snippet missing sentinel: rows=%+v", secondRows)
	}
}

func TestMandatoryStartupIndexesMarkdownSectionsForSearch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	root := filepath.Join(t.TempDir(), "notes")
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	writeInputManifest(t, root, "markdown_sections", root, []string{"*.md"}, nil)
	path := filepath.Join(root, "decisions.md")
	writeFile(t, path, "# Decisions\n\n## First\nalpha\n\n## Second\nsection sentinel omega\n")

	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, []string{"search", "section sentinel omega", "--all-projects", "--source-path", path, "--json"})
	if err != nil {
		t.Fatalf("search: %v stderr=%q", err, stderr.String())
	}
	var rows []minimalSearchResult
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout.String(), err)
	}
	if len(rows) != 1 || rows[0].SourcePath != path || !strings.Contains(rows[0].Snippet, "sentinel") {
		t.Fatalf("rows=%+v, want second indexed section from %s", rows, path)
	}
}

func TestAutoSyncFailuresBlockCachedConsumers(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(t *testing.T, root string)
		wantError string
	}{
		{
			name: "discovery",
			setup: func(t *testing.T, root string) {
				writeInputManifest(t, root, "claude", root, []string{"*.jsonl"}, []string{"["})
				writeFile(t, filepath.Join(root, "session.jsonl"), `{"type":"message","message":{"role":"user","content":"fresh"}}`+"\n")
			},
			wantError: "discover input",
		},
		{
			name: "hash",
			setup: func(t *testing.T, root string) {
				writeInputManifest(t, root, "opencode", root, []string{"*.db"}, nil)
				writeFile(t, filepath.Join(root, "not-sqlite.db"), "not sqlite")
			},
			wantError: "hash",
		},
		{
			name: "parse",
			setup: func(t *testing.T, root string) {
				writeInputManifest(t, root, "opencode", root, []string{"*.db"}, nil)
				path := filepath.Join(root, "opencode.db")
				raw, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatalf("open opencode fixture: %v", err)
				}
				defer func() { _ = raw.Close() }()
				if _, err := raw.Exec(`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT, data TEXT, time_created INTEGER, time_updated INTEGER)`); err != nil {
					t.Fatalf("create message table: %v", err)
				}
			},
			wantError: "parse",
		},
		{
			name: "sync",
			setup: func(t *testing.T, root string) {
				writeInputManifest(t, root, "claude", root, []string{"*.jsonl"}, nil)
				writeFile(t, filepath.Join(root, "session.jsonl"), `{"type":"message","message":{"role":"user","content":"fresh"}}`+"\n")
				orig := maybeAutoSyncSyncFiles
				maybeAutoSyncSyncFiles = func(*storage.Database, []storage.IndexedFile) error { return fmt.Errorf("injected sync failure") }
				t.Cleanup(func() { maybeAutoSyncSyncFiles = orig })
			},
			wantError: "sync files",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modes := []struct {
				name    string
				argv    []string
				machine bool
				json    bool
			}{
				{name: "text", argv: []string{"search", "sentinel"}},
				{name: "robot", argv: []string{"search", "sentinel", "--robot"}, machine: true},
				{name: "json", argv: []string{"search", "sentinel", "--json"}, machine: true, json: true},
			}
			for _, mode := range modes {
				t.Run(mode.name, func(t *testing.T) {
					dbPath := newSupportedIndexedConsumerDB(t)
					root := filepath.Join(t.TempDir(), "inputs-root")
					if err := os.MkdirAll(root, 0o755); err != nil {
						t.Fatalf("mkdir input root: %v", err)
					}
					setIndexPolicyEnv(t, dbPath, t.TempDir())
					tc.setup(t, root)

					var stdout, stderr bytes.Buffer
					err := run(&stdout, &stderr, mode.argv)
					if err == nil {
						t.Fatalf("auto-sync %s failure succeeded; stdout=%q stderr=%q", tc.name, stdout.String(), stderr.String())
					}
					combined := stdout.String() + stderr.String()
					if strings.Contains(combined, "sentinel cached") {
						t.Fatalf("auto-sync %s failure emitted cached sentinel row: stdout=%q stderr=%q", tc.name, stdout.String(), stderr.String())
					}
					if strings.Contains(stdout.String(), "result_") {
						t.Fatalf("auto-sync %s failure emitted cached result rows: stdout=%q stderr=%q", tc.name, stdout.String(), stderr.String())
					}

					if mode.machine && stderr.Len() != 0 {
						t.Fatalf("auto-sync %s %s diagnostic wrote stderr: %q", tc.name, mode.name, stderr.String())
					}

					if mode.json {
						var diag struct {
							Code         string   `json:"code"`
							Summary      string   `json:"summary"`
							Continuation []string `json:"continuation_argv"`
						}
						if err := json.Unmarshal(stdout.Bytes(), &diag); err != nil {
							t.Fatalf("auto-sync %s JSON diagnostic is invalid: %v stdout=%q", tc.name, err, stdout.String())
						}
						if diag.Code != string(compat.CodeIndexStale) {
							t.Fatalf("auto-sync %s JSON code=%q, want %q", tc.name, diag.Code, compat.CodeIndexStale)
						}
						if !strings.Contains(diag.Summary, tc.wantError) {
							t.Fatalf("auto-sync %s JSON summary=%q missing %q", tc.name, diag.Summary, tc.wantError)
						}
						if len(diag.Continuation) == 0 {
							t.Fatalf("auto-sync %s JSON continuation is empty: stdout=%q", tc.name, stdout.String())
						}
						if strings.Contains(stdout.String(), `"source_path"`) {
							t.Fatalf("auto-sync %s JSON diagnostic leaked result payload: %q", tc.name, stdout.String())
						}
						return
					}

					if !strings.Contains(combined, string(compat.CodeIndexStale)) || !strings.Contains(combined, tc.wantError) {
						t.Fatalf("missing %s diagnostic; stdout=%q stderr=%q err=%v", tc.wantError, stdout.String(), stderr.String(), err)
					}
				})
			}
		})
	}
}

func TestMigrationFailureBlocksCachedConsumer(t *testing.T) {
	dbPath := newFixtureIndexDB(t, "v8.sql")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	_, err = raw.Exec(`
		INSERT INTO tool_events (message_uuid, source_path, ordinal, tool_name, command_head, is_error, extraction_version)
		VALUES ('dup-tool-uuid', '/dup-a.jsonl', 1, 'Bash', 'echo one', 0, 8),
		       ('dup-tool-uuid', '/dup-b.jsonl', 2, 'Bash', 'echo two', 0, 8)
	`)
	closeErr := raw.Close()
	if err != nil {
		t.Fatalf("seed duplicate tool events: %v", err)
	}
	if closeErr != nil {
		t.Fatalf("close fixture: %v", closeErr)
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())

	var stdout, stderr bytes.Buffer
	err = run(&stdout, &stderr, []string{"search", "sentinel"})
	if err == nil {
		t.Fatalf("migration failure succeeded; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, string(compat.CodeMigrationFailed)) || !strings.Contains(combined, "conflicting tool_events") {
		t.Fatalf("missing migration-failure diagnostic; stdout=%q stderr=%q err=%v", stdout.String(), stderr.String(), err)
	}
	if strings.Contains(combined, "sentinel") {
		t.Fatalf("migration failure emitted cached sentinel: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestMachineModesSuppressAutoSyncProgressStderr(t *testing.T) {
	for _, tc := range []struct {
		name     string
		flag     string
		query    string
		content  string
		validate func(t *testing.T, stdout string)
	}{
		{
			name:    "json",
			flag:    "--json",
			query:   "fresh machine json sentinel",
			content: "fresh machine json sentinel",
			validate: func(t *testing.T, stdout string) {
				t.Helper()
				var rows []minimalSearchResult
				if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
					t.Fatalf("startup contaminated JSON stdout %q: %v", stdout, err)
				}
				if len(rows) == 0 {
					t.Fatalf("JSON machine search returned no rows: stdout=%q", stdout)
				}
			},
		},
		{
			name:    "robot",
			flag:    "--robot",
			query:   "fresh machine robot sentinel",
			content: "fresh machine robot sentinel",
			validate: func(t *testing.T, stdout string) {
				t.Helper()
				assertRobotResultLines(t, stdout)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := newSupportedIndexedConsumerDB(t)
			root := filepath.Join(t.TempDir(), "inputs-root")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatalf("mkdir input root: %v", err)
			}
			setIndexPolicyEnv(t, dbPath, t.TempDir())
			writeInputManifest(t, root, "claude", root, []string{"*.jsonl"}, nil)
			writeFile(t, filepath.Join(root, "session.jsonl"), fmt.Sprintf(`{"type":"message","message":{"role":"user","content":%q}}`+"\n", tc.content))

			var stdout, stderr bytes.Buffer
			argv := []string{"search", tc.query, tc.flag, "--all-projects"}
			if err := run(&stdout, &stderr, argv); err != nil {
				t.Fatalf("machine search failed: %v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("machine mode wrote progress to stderr: %q", stderr.String())
			}
			tc.validate(t, stdout.String())
		})
	}
}

func TestValidateRobotResultLinesRejectsNonResultLines(t *testing.T) {
	err := validateRobotResultLines("result_0_source=session\nthis is not a result line")
	if err == nil {
		t.Fatal("validateRobotResultLines accepted non-result nonempty output")
	}
}

func TestRebuildFailsOnDerivedMaintenanceError(t *testing.T) {
	dbPath := newSupportedIndexedConsumerDB(t)
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	orig := rebuildBackfillDerived
	rebuildBackfillDerived = func(*storage.Database, storage.BackfillDerivedOpts) error {
		return fmt.Errorf("injected derived failure")
	}
	t.Cleanup(func() { rebuildBackfillDerived = orig })

	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, []string{"rebuild"})
	if err == nil {
		t.Fatalf("rebuild succeeded despite derived failure; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "warning:") {
		t.Fatalf("derived failure was downgraded to warning: %q", stderr.String())
	}
	if !strings.Contains(err.Error(), "backfill derived") {
		t.Fatalf("error = %v, want backfill derived", err)
	}
}

func TestResolveActiveIndexPathPropagatesBrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.db")
	if err := os.Symlink(filepath.Join(dir, "missing-target.db"), broken); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, diag, err := prepareIndex(context.Background(), &config.Config{DatabasePath: broken}, indexDataRead)
	if err == nil {
		t.Fatalf("broken symlink resolved successfully; diagnostic=%+v", diag)
	}
	if diag == nil || !strings.Contains(diag.Summary, "no such file") {
		t.Fatalf("diagnostic=%+v err=%v, want EvalSymlinks error", diag, err)
	}
}

func assertRobotResultLines(t *testing.T, robotStdout string) {
	t.Helper()
	if err := validateRobotResultLines(robotStdout); err != nil {
		t.Fatal(err)
	}
}

func validateRobotResultLines(robotStdout string) error {
	if strings.TrimSpace(robotStdout) == "" {
		return fmt.Errorf("robot machine search returned empty stdout")
	}
	resultLine := regexp.MustCompile(`^result_[0-9]+_[a-z_]+=.*$`)
	resultLines := 0
	for _, raw := range strings.Split(strings.TrimSpace(robotStdout), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !resultLine.MatchString(line) {
			return fmt.Errorf("invalid robot output line %q; expected result_N_field=value", raw)
		}
		if strings.Contains(line, "\r") {
			return fmt.Errorf("robot output contains unescaped carriage return: %q", raw)
		}
		resultLines++
	}
	if resultLines == 0 {
		return fmt.Errorf("robot machine search returned no result lines: %q", robotStdout)
	}
	return nil
}

func assertDiagnosticFields(t *testing.T, code string, continuation []string, dbPath string) {
	t.Helper()
	if code != string(compat.CodeUnsupportedLineage) {
		t.Fatalf("code=%q, want %q", code, compat.CodeUnsupportedLineage)
	}
	want := []string{"recover", "--from", dbPath, "--dry-run"}
	if len(continuation) != len(want) {
		t.Fatalf("continuation=%v, want %v", continuation, want)
	}
	for i := range want {
		if continuation[i] != want[i] {
			t.Fatalf("continuation=%v, want %v", continuation, want)
		}
	}
}

func newSupportedIndexedConsumerDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	messages := []storage.IndexedMessage{
		{Ordinal: 0, UUID: "u", Role: "user", Text: "sentinel cached text", Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: storage.CurrentExtractionVersion},
		{Ordinal: 1, UUID: "tool-u", Role: "assistant", Text: "sentinel cached command", Timestamp: "2026-01-01T00:00:01Z", ContentType: "tool", ToolName: "Bash", CommandHead: "sentinel", ExtractionVersion: storage.CurrentExtractionVersion},
	}
	if err := db.SyncFiles([]storage.IndexedFile{{
		SourcePath: "/fixture/session.jsonl",
		Source:     "session",
		Hash:       "hash1",
		Project:    "project",
		Messages:   messages,
		Tags:       []string{"policy"},
	}}); err != nil {
		_ = db.Close()
		t.Fatalf("seed sync: %v", err)
	}
	if _, err := db.DB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = db.Close()
		t.Fatalf("checkpoint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(dbPath)
	if err != nil {
		t.Fatalf("resolve db path: %v", err)
	}
	return resolved
}

func newFixtureIndexDB(t *testing.T, fixture string) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), fixture+".db")
	scriptPath := filepath.Join("..", "..", "internal", "compat", "testdata", "release-schemas", fixture)
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture, err)
	}
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	if _, err := raw.Exec(string(script)); err != nil {
		_ = raw.Close()
		t.Fatalf("execute fixture %s: %v", fixture, err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(dbPath)
	if err != nil {
		t.Fatalf("resolve fixture db: %v", err)
	}
	return resolved
}

func setIndexPolicyEnv(t *testing.T, dbPath, cfgDir string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	emptyInputs := filepath.Join(t.TempDir(), "empty-inputs")
	if err := os.MkdirAll(emptyInputs, 0o755); err != nil {
		t.Fatalf("mkdir empty inputs: %v", err)
	}
	t.Setenv("BACKSCROLL_SESSION_DIRS", emptyInputs)
}

func writeInputManifest(t *testing.T, root, format string, discoverRoot string, include, exclude []string) {
	t.Helper()
	cfgDir := os.Getenv("BACKSCROLL_CONFIG_DIR")
	if cfgDir == "" {
		t.Fatal("BACKSCROLL_CONFIG_DIR not set")
	}
	inputsDir := filepath.Join(cfgDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatalf("mkdir inputs dir: %v", err)
	}
	quoteList := func(values []string) string {
		parts := make([]string, 0, len(values))
		for _, v := range values {
			parts = append(parts, fmt.Sprintf("%q", v))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	manifest := fmt.Sprintf(`version = 1
[[inputs]]
id = "policy"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = %s
exclude = %s
follow_symlinks = false
[inputs.decode]
format = %q
`, discoverRoot, quoteList(include), quoteList(exclude), format)
	writeFile(t, filepath.Join(inputsDir, "policy.inputs.toml"), manifest)
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newUnsupportedIndexedConsumerDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	cfgDir := filepath.Join(dir, "config")
	emptyInputs := filepath.Join(dir, "empty-inputs")
	for _, path := range []string{home, cfgDir, emptyInputs} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	dbPath := filepath.Join(dir, "index.db")
	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	t.Setenv("BACKSCROLL_SESSION_DIRS", emptyInputs)
	t.Chdir(dir)

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	messages := []storage.IndexedMessage{
		{Ordinal: 0, UUID: "u", Role: "user", Text: "policy fixture text", Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: storage.CurrentExtractionVersion},
		{Ordinal: 1, UUID: "tool-u", Role: "assistant", Text: "go test ./...", Timestamp: "2026-01-01T00:00:01Z", ContentType: "tool", ToolName: "Bash", CommandHead: "sentinelcmd", ExtractionVersion: storage.CurrentExtractionVersion},
	}
	if err := db.SyncFiles([]storage.IndexedFile{{
		SourcePath: "/fixture/session.jsonl",
		Source:     "session",
		Hash:       "hash1",
		Project:    "project",
		Messages:   messages,
		Tags:       []string{"policy"},
	}}); err != nil {
		_ = db.Close()
		t.Fatalf("seed sync: %v", err)
	}
	if _, err := db.DB().Exec(`CREATE TABLE unexpected_shape_marker (id INTEGER PRIMARY KEY)`); err != nil {
		_ = db.Close()
		t.Fatalf("make unsupported: %v", err)
	}
	if _, err := db.DB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = db.Close()
		t.Fatalf("checkpoint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(dbPath)
	if err != nil {
		t.Fatalf("resolve db path: %v", err)
	}
	return resolved
}

func assertMissingDatabaseArtifacts(t *testing.T, path string) {
	t.Helper()
	for _, artifact := range []string{path, path + "-wal", path + "-shm"} {
		if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("artifact %s stat err=%v, want not-exist", artifact, err)
		}
	}
}

func readDBBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read db bytes: %v", err)
	}
	return data
}
