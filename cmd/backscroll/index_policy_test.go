package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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
				if !strings.Contains(out, "diagnostic_continuation_argv=recover --from "+dbPath+" --dry-run") {
					t.Fatalf("robot diagnostic missing exact continuation: %q", out)
				}
			}
		})
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
	assertSQLiteContainsIndexedText(t, dbPath, path, "perennial sqlite sentinel")
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
	assertSQLiteContainsIndexedText(t, dbPath, path, "section sentinel omega")
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
			for _, argv := range [][]string{{"search", "sentinel"}, {"search", "sentinel", "--robot"}} {
				t.Run(strings.Join(argv, " "), func(t *testing.T) {
					dbPath := newSupportedIndexedConsumerDB(t)
					root := filepath.Join(t.TempDir(), "inputs-root")
					if err := os.MkdirAll(root, 0o755); err != nil {
						t.Fatalf("mkdir input root: %v", err)
					}
					setIndexPolicyEnv(t, dbPath, t.TempDir())
					tc.setup(t, root)

					var stdout, stderr bytes.Buffer
					err := run(&stdout, &stderr, argv)
					if err == nil {
						t.Fatalf("auto-sync %s failure succeeded; stdout=%q stderr=%q", tc.name, stdout.String(), stderr.String())
					}
					combined := stdout.String() + stderr.String()
					if !strings.Contains(combined, string(compat.CodeIndexStale)) || !strings.Contains(combined, tc.wantError) {
						t.Fatalf("missing %s diagnostic; stdout=%q stderr=%q err=%v", tc.wantError, stdout.String(), stderr.String(), err)
					}
					if strings.Contains(combined, "sentinel cached") {
						t.Fatalf("auto-sync %s failure emitted cached sentinel row: stdout=%q stderr=%q", tc.name, stdout.String(), stderr.String())
					}
					if strings.Contains(stdout.String(), "result_") {
						t.Fatalf("auto-sync %s failure emitted cached result rows: stdout=%q stderr=%q", tc.name, stdout.String(), stderr.String())
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
	dbPath := newSupportedIndexedConsumerDB(t)
	root := filepath.Join(t.TempDir(), "inputs-root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir input root: %v", err)
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	writeInputManifest(t, root, "claude", root, []string{"*.jsonl"}, nil)
	writeFile(t, filepath.Join(root, "session.jsonl"), `{"type":"message","message":{"role":"user","content":"fresh machine"}}`+"\n")

	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"json", []string{"search", "fresh", "--json", "--all-projects"}},
		{"robot", []string{"search", "fresh", "--robot", "--all-projects"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := run(&stdout, &stderr, tc.argv); err != nil {
				t.Fatalf("machine search failed: %v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("machine mode wrote progress to stderr: %q", stderr.String())
			}
			switch tc.name {
			case "json":
				var rows []minimalSearchResult
				if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
					t.Fatalf("startup contaminated JSON stdout %q: %v", stdout.String(), err)
				}
				if len(rows) == 0 {
					t.Fatalf("JSON machine search returned no rows: stdout=%q", stdout.String())
				}
			case "robot":
				assertRobotResultLines(t, stdout.String())
			}
		})
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

func assertSQLiteContainsIndexedText(t *testing.T, dbPath, sourcePath, text string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open index db: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close index db: %v", err)
		}
	}()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ? AND text LIKE ?`, sourcePath, "%"+text+"%").Scan(&count); err != nil {
		t.Fatalf("query indexed markdown rows: %v", err)
	}
	if count == 0 {
		t.Fatalf("SQLite index has no markdown row for %s containing %q", sourcePath, text)
	}
}

func assertRobotResultLines(t *testing.T, robotStdout string) {
	t.Helper()
	if strings.TrimSpace(robotStdout) == "" {
		t.Fatal("robot machine search returned empty stdout")
	}
	resultLine := regexp.MustCompile(`^result_[0-9]+_[a-z_]+=`)
	resultLines := 0
	for _, line := range strings.Split(strings.TrimSpace(robotStdout), "\n") {
		if strings.HasPrefix(line, "result_") {
			resultLines++
			if !resultLine.MatchString(line) {
				t.Fatalf("invalid robot line %q", line)
			}
		}
	}
	if resultLines == 0 {
		t.Fatalf("robot machine search returned no result lines: %q", robotStdout)
	}
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

func readDBBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read db bytes: %v", err)
	}
	return data
}
