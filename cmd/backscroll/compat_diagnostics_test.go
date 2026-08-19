package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestDirectReadRemainsAvailableButClaimsNoIndexFreshness(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	dbPath := newUnsupportedIndexedConsumerDB(t)
	before := readDBBytes(t, dbPath)

	stdout, stderr, err := runCmd("read", fixture)
	if err != nil {
		t.Fatalf("read with unsupported index configured failed: %v\nstderr: %s", err, stderr)
	}
	for _, want := range []string{"pi manifest fixture signal", "pi visible answer"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("read output missing decoded fixture content %q:\n%s", want, stdout)
		}
	}
	for _, forbidden := range []string{"usable", "fresh", "current index"} {
		if strings.Contains(strings.ToLower(stdout+stderr), forbidden) {
			t.Fatalf("direct read made index-freshness claim %q; stdout=%q stderr=%q", forbidden, stdout, stderr)
		}
	}
	if after := readDBBytes(t, dbPath); !bytes.Equal(after, before) {
		t.Fatal("direct read mutated unsupported database bytes")
	}
}

func TestStatusUnhealthyIsReadOnly(t *testing.T) {
	dbPath := newUnsupportedIndexedConsumerDB(t)
	before := snapshotSQLiteFiles(t, dbPath)

	stdout, stderr, err := runCmd("status")
	if err == nil {
		t.Fatalf("status succeeded on unsupported index; stdout=%q stderr=%q", stdout, stderr)
	}
	assertDiagnosticText(t, stdout+stderr, compat.CodeUnsupportedLineage, dbPath)
	assertSQLiteFilesUnchanged(t, dbPath, before)
}

func TestValidateUnhealthyIsReadOnly(t *testing.T) {
	dbPath := newUnsupportedIndexedConsumerDB(t)
	before := snapshotSQLiteFiles(t, dbPath)

	stdout, stderr, err := runCmd("validate")
	if err == nil {
		t.Fatalf("validate succeeded on unsupported index; stdout=%q stderr=%q", stdout, stderr)
	}
	assertDiagnosticText(t, stdout+stderr, compat.CodeUnsupportedLineage, dbPath)
	assertSQLiteFilesUnchanged(t, dbPath, before)
}

func TestValidateTextReportsSemanticRecoveryDiagnosticsReadOnly(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(t *testing.T) string
		wantCode compat.Code
	}{
		{
			name:     "recovery conflict",
			setup:    newRecoveryConflictDiagnosticDB,
			wantCode: compat.CodeRecoveryConflict,
		},
		{
			name:     "uninterpretable row",
			setup:    newUninterpretableRowDiagnosticDB,
			wantCode: compat.CodeUninterpretableRow,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := tc.setup(t)
			before := snapshotSQLiteFiles(t, dbPath)

			stdout, stderr, err := runCmd("validate")
			if err == nil {
				t.Fatalf("plain validate succeeded; want %s diagnostic; stdout=%q stderr=%q", tc.wantCode, stdout, stderr)
			}
			combined := stdout + stderr
			assertDiagnosticText(t, combined, tc.wantCode, dbPath)
			wantContinuation := fmt.Sprintf("continuation: recover --from %s --dry-run\n", dbPath)
			if !strings.Contains(combined, wantContinuation) {
				t.Fatalf("plain validate continuation mismatch: want exact line %q in %q", wantContinuation, combined)
			}
			assertSQLiteFilesUnchanged(t, dbPath, before)
		})
	}
}

func TestLiveWALDiagnosticDoesNotClaimMigrationFailureOrRecovery(t *testing.T) {
	dbPath, closeWriter := newLiveWALDiagnosticDB(t)
	defer closeWriter()
	before := snapshotSQLiteFiles(t, dbPath)

	for _, argv := range [][]string{{"status", "--json"}, {"validate", "--json"}} {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			got := runJSONDiagnosticAllowNoContinuation(t, argv, compat.CodeIndexStale)
			if len(got.Continuation) != 0 {
				t.Fatalf("%v continuation=%v, want none", argv, got.Continuation)
			}
			summary := strings.ToLower(got.Summary)
			for _, want := range []string{"cannot be inspected without side effects", "wal", "uncheckpointed frames"} {
				if !strings.Contains(summary, want) {
					t.Fatalf("%v summary missing %q: %q", argv, want, got.Summary)
				}
			}
			for _, forbidden := range []string{"migration_failed", "recover --from", "--dry-run"} {
				if strings.Contains(strings.ToLower(got.Code+" "+got.Summary+" "+strings.Join(got.Continuation, " ")), forbidden) {
					t.Fatalf("%v emitted false recovery/migration guidance %q in %+v", argv, forbidden, got)
				}
			}
			assertSQLiteFilesUnchanged(t, dbPath, before)
		})
	}
}

func TestBlockingDiagnosticsHaveExecutableContinuations(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(t *testing.T) string
		argv     []string
		wantCode compat.Code
	}{
		{
			name: "unsupported lineage",
			setup: func(t *testing.T) string {
				return newUnsupportedIndexedConsumerDB(t)
			},
			argv:     []string{"status", "--json"},
			wantCode: compat.CodeUnsupportedLineage,
		},
		{
			name: "migration failure",
			setup: func(t *testing.T) string {
				return newMigrationFailureDiagnosticDB(t)
			},
			argv:     []string{"search", "sentinel", "--json"},
			wantCode: compat.CodeMigrationFailed,
		},
		{
			name: "stale sync",
			setup: func(t *testing.T) string {
				return newStaleSyncDiagnosticDB(t)
			},
			argv:     []string{"search", "sentinel", "--json"},
			wantCode: compat.CodeIndexStale,
		},
		{
			name: "recovery conflict",
			setup: func(t *testing.T) string {
				return newRecoveryConflictDiagnosticDB(t)
			},
			argv:     []string{"validate", "--json"},
			wantCode: compat.CodeRecoveryConflict,
		},
		{
			name: "uninterpretable row",
			setup: func(t *testing.T) string {
				return newUninterpretableRowDiagnosticDB(t)
			},
			argv:     []string{"validate", "--json"},
			wantCode: compat.CodeUninterpretableRow,
		},
	}

	ran := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ran++
			dbPath := tc.setup(t)
			diagnostic := runJSONDiagnostic(t, tc.argv, tc.wantCode)
			want := []string{"recover", "--from", dbPath, "--dry-run"}
			if !equalStrings(diagnostic.Continuation, want) {
				t.Fatalf("continuation=%v, want %v", diagnostic.Continuation, want)
			}
			executeContinuationThroughCobra(t, diagnostic.Continuation)
		})
	}
	if ran != len(cases) {
		t.Fatalf("ran %d cases, want %d", ran, len(cases))
	}
}

type jsonDiagnosticPayload struct {
	Code         string   `json:"code"`
	Summary      string   `json:"summary"`
	Continuation []string `json:"continuation_argv"`
}

func runJSONDiagnostic(t *testing.T, argv []string, wantCode compat.Code) jsonDiagnosticPayload {
	t.Helper()
	got := runJSONDiagnosticAllowNoContinuation(t, argv, wantCode)
	if len(got.Continuation) == 0 {
		t.Fatalf("%v emitted empty continuation in diagnostic %+v", argv, got)
	}
	return got
}

func runJSONDiagnosticAllowNoContinuation(t *testing.T, argv []string, wantCode compat.Code) jsonDiagnosticPayload {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, argv)
	if err == nil {
		t.Fatalf("%v succeeded; stdout=%q stderr=%q", argv, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("%v JSON diagnostic wrote stderr: %q", argv, stderr.String())
	}
	var got jsonDiagnosticPayload
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &got); unmarshalErr != nil {
		t.Fatalf("%v emitted invalid JSON diagnostic %q: %v", argv, stdout.String(), unmarshalErr)
	}
	if got.Code != string(wantCode) {
		t.Fatalf("%v code=%q summary=%q, want %q", argv, got.Code, got.Summary, wantCode)
	}
	return got
}

func executeContinuationThroughCobra(t *testing.T, argv []string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := buildRootCmd(&stdout, &stderr)
	root.SetArgs(argv)
	err := root.Execute()
	if err != nil && isCobraResolutionError(err) {
		t.Fatalf("continuation %v did not resolve through Cobra: %v\nstdout=%q stderr=%q", argv, err, stdout.String(), stderr.String())
	}
}

func isCobraResolutionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unknown command") || strings.Contains(msg, "unknown flag") || strings.Contains(msg, "required flag") || strings.Contains(msg, "accepts")
}

func assertDiagnosticText(t *testing.T, text string, code compat.Code, dbPath string) {
	t.Helper()
	for _, want := range []string{string(code), "recover", "--from", dbPath, "--dry-run"} {
		if !strings.Contains(text, want) {
			t.Fatalf("diagnostic text missing %q: %q", want, text)
		}
	}
}

func newMigrationFailureDiagnosticDB(t *testing.T) string {
	t.Helper()
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
	return dbPath
}

func newStaleSyncDiagnosticDB(t *testing.T) string {
	t.Helper()
	dbPath := newSupportedIndexedConsumerDB(t)
	root := filepath.Join(t.TempDir(), "inputs-root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir input root: %v", err)
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	writeInputManifest(t, root, "claude", root, []string{"["}, nil)
	writeFile(t, filepath.Join(root, "session.jsonl"), `{"type":"message","message":{"role":"user","content":"fresh"}}`+"\n")
	return dbPath
}

func newLiveWALDiagnosticDB(t *testing.T) (string, func()) {
	t.Helper()
	dbPath, db := newEmptyCurrentDiagnosticDB(t)
	if err := db.SyncFiles([]storage.IndexedFile{{
		SourcePath: "/live-wal/session.jsonl",
		Source:     "session",
		Hash:       "live-wal-hash",
		Project:    "project",
		Messages: []storage.IndexedMessage{{
			Ordinal:     0,
			Role:        "user",
			Text:        "live WAL committed diagnostic row",
			UUID:        "44444444-4444-4444-8444-444444444444",
			Timestamp:   "2026-08-18T00:00:00Z",
			ContentType: "text",
		}},
	}}); err != nil {
		_ = db.Close()
		t.Fatalf("seed live WAL diagnostic db: %v", err)
	}
	wal, err := os.Stat(dbPath + "-wal")
	if err != nil {
		_ = db.Close()
		t.Fatalf("stat live WAL diagnostic sidecar: %v", err)
	}
	if wal.Size() == 0 {
		_ = db.Close()
		t.Fatal("live WAL diagnostic sidecar is empty; fixture did not keep committed WAL frames")
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	return dbPath, func() { _ = db.Close() }
}

func newRecoveryConflictDiagnosticDB(t *testing.T) string {
	t.Helper()
	dbPath, db := newEmptyCurrentDiagnosticDB(t)
	_, err := db.DB().Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, project, content_type)
		VALUES ('session', '/conflict/session.jsonl', 7, 'user', 'first conflicting payload', '2026-08-18T00:00:00Z', 'project', 'text'),
		       ('session', '/conflict/session.jsonl', 7, 'assistant', 'second conflicting payload', '2026-08-18T00:00:01Z', 'project', 'text')
	`)
	if err != nil {
		_ = db.Close()
		t.Fatalf("seed recovery conflict: %v", err)
	}
	if err := closeSeedDB(t, db); err != nil {
		t.Fatalf("close conflict db: %v", err)
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	return dbPath
}

func newUninterpretableRowDiagnosticDB(t *testing.T) string {
	t.Helper()
	dbPath, db := newEmptyCurrentDiagnosticDB(t)
	_, err := db.DB().Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, project, content_type)
		VALUES ('session', '', -1, 'user', 'missing identity payload', '2026-08-18T00:00:00Z', 'project', 'text')
	`)
	if err != nil {
		_ = db.Close()
		t.Fatalf("seed uninterpretable row: %v", err)
	}
	if err := closeSeedDB(t, db); err != nil {
		t.Fatalf("close uninterpretable db: %v", err)
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	return dbPath
}

func newEmptyCurrentDiagnosticDB(t *testing.T) (string, *storage.Database) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "index.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open current diagnostic db: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(dbPath)
	if err != nil {
		_ = db.Close()
		t.Fatalf("resolve current diagnostic db: %v", err)
	}
	return resolved, db
}

func closeSeedDB(t *testing.T, db *storage.Database) error {
	t.Helper()
	if _, err := db.DB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = db.Close()
		return fmt.Errorf("checkpoint: %w", err)
	}
	return db.Close()
}

func snapshotSQLiteFiles(t *testing.T, dbPath string) map[string][]byte {
	t.Helper()
	snapshot := make(map[string][]byte)
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm", dbPath + "-journal"} {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			snapshot[path] = nil
			continue
		}
		if err != nil {
			t.Fatalf("read sqlite file %s: %v", path, err)
		}
		snapshot[path] = append([]byte(nil), data...)
	}
	return snapshot
}

func assertSQLiteFilesUnchanged(t *testing.T, dbPath string, before map[string][]byte) {
	t.Helper()
	after := snapshotSQLiteFiles(t, dbPath)
	for path, want := range before {
		if !bytes.Equal(after[path], want) {
			t.Fatalf("sqlite file %s changed: before %d bytes after %d bytes", path, len(want), len(after[path]))
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
