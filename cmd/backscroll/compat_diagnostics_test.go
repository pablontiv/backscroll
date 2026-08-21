package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestStatusUnhealthyReturnsDiagnostic(t *testing.T) {
	dbPath := newUnsupportedIndexedConsumerDB(t)

	stdout, stderr, err := runCmd("status")
	if err == nil {
		t.Fatalf("status succeeded on unsupported index; stdout=%q stderr=%q", stdout, stderr)
	}
	assertDiagnosticText(t, stdout+stderr, compat.CodeUnsupportedLineage, dbPath)
}

func TestValidateUnhealthyReturnsDiagnostic(t *testing.T) {
	dbPath := newUnsupportedIndexedConsumerDB(t)

	stdout, stderr, err := runCmd("validate")
	if err == nil {
		t.Fatalf("validate succeeded on unsupported index; stdout=%q stderr=%q", stdout, stderr)
	}
	assertDiagnosticText(t, stdout+stderr, compat.CodeUnsupportedLineage, dbPath)
}

func TestRecoveryDiagnosticsForIndexHonorsCanceledContext(t *testing.T) {
	dbPath := newRecoveryConflictDiagnosticDB(t)
	db, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open recovery diagnostics db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	diagnostics, err := recoveryDiagnosticsForIndex(ctx, db, dbPath)
	if err == nil {
		t.Fatalf("recovery diagnostics succeeded with canceled context; diagnostics=%+v", diagnostics)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("recovery diagnostics error = %v, want context canceled", err)
	}
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

func TestValidateTextReportsMultipleSemanticRecoveryDiagnosticsReadOnly(t *testing.T) {
	dbPath := newMixedRecoveryDiagnosticDB(t)
	before := snapshotSQLiteFiles(t, dbPath)

	stdout, stderr, err := runCmd("validate")
	if err == nil {
		t.Fatalf("plain validate succeeded; want multiple recovery diagnostics; stdout=%q stderr=%q", stdout, stderr)
	}
	combined := stdout + stderr
	for _, wantCode := range []compat.Code{compat.CodeRecoveryConflict, compat.CodeUninterpretableRow} {
		assertDiagnosticText(t, combined, wantCode, dbPath)
	}
	if got := strings.Count(combined, "continuation: recover --from "+dbPath+" --dry-run"); got < 2 {
		t.Fatalf("plain validate continuation count = %d, want at least 2 in %q", got, combined)
	}
	assertSQLiteFilesUnchanged(t, dbPath, before)
}

func TestStatusHealthyIndexReportsTextAndJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backscroll.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("create current index: %v", err)
	}
	if err := db.SyncFiles([]storage.IndexedFile{{
		SourcePath: "/sessions/status.jsonl",
		Source:     "session",
		Hash:       "status-hash",
		Project:    "project",
		Messages: []storage.IndexedMessage{{
			Ordinal: 0, Role: "user", Text: "status visible sentinel", UUID: "11111111-1111-4111-8111-111111111111",
			Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: storage.CurrentExtractionVersion,
		}},
	}}); err != nil {
		_ = db.Close()
		t.Fatalf("seed current index: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close current index: %v", err)
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())

	stdout, stderr, err := runCmd("status")
	if err != nil {
		t.Fatalf("status healthy index failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" || !strings.Contains(stdout, "Files indexed:    1") || !strings.Contains(stdout, "Messages indexed: 1") {
		t.Fatalf("status healthy index text output stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, err = runCmd("status", "--json")
	if err != nil {
		t.Fatalf("status --json healthy index failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("status --json healthy index wrote stderr: %q", stderr)
	}
	var payload struct {
		Database struct {
			Exists bool `json:"exists"`
			Size   int  `json:"size"`
		} `json:"database"`
		Index struct {
			Usable        bool `json:"usable"`
			TotalFiles    int  `json:"total_files"`
			TotalMessages int  `json:"total_messages"`
		} `json:"index"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("status --json healthy index emitted invalid JSON %q: %v", stdout, err)
	}
	if !payload.Database.Exists || payload.Database.Size == 0 || !payload.Index.Usable || payload.Index.TotalFiles != 1 || payload.Index.TotalMessages != 1 {
		t.Fatalf("status --json healthy index payload = %+v, want existing usable one-row index", payload)
	}
}

func TestAnnotatePathOrdinalFallbackPersistsThroughCobra(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backscroll.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("create current index: %v", err)
	}
	if err := db.SyncFiles([]storage.IndexedFile{{
		SourcePath: "/sessions/fallback.jsonl",
		Source:     "session",
		Hash:       "annotate-hash",
		Project:    "project",
		Messages: []storage.IndexedMessage{{
			Ordinal: 3, Role: "assistant", Text: "annotation fallback sentinel",
			Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: storage.CurrentExtractionVersion,
		}},
	}}); err != nil {
		_ = db.Close()
		t.Fatalf("seed current index: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close current index: %v", err)
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())

	stdout, stderr, err := runCmd("annotate", "--path", "/sessions/fallback.jsonl", "--ordinal", "3", "--kind", "correction", "--label", "needs-review")
	if err != nil {
		t.Fatalf("annotate path/ordinal fallback failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" || !strings.Contains(stdout, "Annotated /sessions/fallback.jsonl:3 as correction=needs-review") {
		t.Fatalf("annotate path/ordinal fallback output stdout=%q stderr=%q", stdout, stderr)
	}

	inspect, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open annotated index read-only: %v", err)
	}
	defer func() { _ = inspect.Close() }()
	var label, source string
	if err := inspect.DB().QueryRow(`SELECT label, source FROM annotations WHERE source_path = ? AND ordinal = ? AND kind = ?`, "/sessions/fallback.jsonl", 3, "correction").Scan(&label, &source); err != nil {
		t.Fatalf("query persisted annotation: %v", err)
	}
	if label != "needs-review" || source != "agent" {
		t.Fatalf("persisted annotation label/source = %q/%q", label, source)
	}
}

func TestValidateHealthyIndexReportsTextAndJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backscroll.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("create current index: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close current index: %v", err)
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())

	stdout, stderr, err := runCmd("validate")
	if err != nil {
		t.Fatalf("validate healthy index failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" || !strings.Contains(stdout, "Index validation passed") {
		t.Fatalf("validate healthy index text output stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, err = runCmd("validate", "--json")
	if err != nil {
		t.Fatalf("validate --json healthy index failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("validate --json healthy index wrote stderr: %q", stderr)
	}
	var payload struct {
		Valid          bool `json:"valid"`
		DatabaseExists bool `json:"database_exists"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("validate --json healthy index emitted invalid JSON %q: %v", stdout, err)
	}
	if !payload.Valid || !payload.DatabaseExists {
		t.Fatalf("validate --json healthy index payload = %+v, want valid existing database", payload)
	}
}

func TestConfigAndStatusDeclarativeInputsVisibleAfterStartup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "backscroll.db")
	cfgDir := filepath.Join(dir, "config")
	presetDir := filepath.Join(fixturesDir(), "claude-preset")
	setupInputsPreset(t, cfgDir, filepath.Join(presetDir, "projects"))
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	stdout, stderr, err := runCmd("config")
	if err != nil {
		t.Fatalf("config text with declarative input failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	for _, want := range []string{"Backscroll Configuration", "Inputs (declarative)", "id:      claude", "format:  claude"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("config text missing %q in:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("config text wrote stderr: %q", stderr)
	}

	stdout, stderr, err = runCmd("config", "--json")
	if err != nil {
		t.Fatalf("config --json with declarative input failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("config --json wrote stderr: %q", stderr)
	}
	var payload struct {
		Inputs struct {
			Mode     string `json:"mode"`
			Count    int    `json:"count"`
			Manifest []struct {
				ID      string   `json:"id"`
				Format  string   `json:"format"`
				Include []string `json:"include"`
			} `json:"manifest"`
		} `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("config --json emitted invalid JSON %q: %v", stdout, err)
	}
	if payload.Inputs.Mode != "declarative" || payload.Inputs.Count != 1 || len(payload.Inputs.Manifest) != 1 || payload.Inputs.Manifest[0].ID != "claude" || payload.Inputs.Manifest[0].Format != "claude" {
		t.Fatalf("config --json declarative payload = %+v", payload.Inputs)
	}

	stdout, stderr, err = runCmd("status")
	if err != nil {
		t.Fatalf("status text with declarative input failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	for _, want := range []string{"Files indexed:    1", "Messages indexed: 4", "Inputs: 1 active (declarative)", "- claude"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("status text missing %q in:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("status text wrote stderr: %q", stderr)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("mandatory startup did not prepare configured database: %v", err)
	}
}

func TestStatusAndValidateMissingIndexArePreparedByStartup(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backscroll.db")
	setIndexPolicyEnv(t, dbPath, t.TempDir())

	stdout, stderr, err := runCmd("validate")
	if err != nil {
		t.Fatalf("validate startup-prepared index failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" || !strings.Contains(stdout, "Index validation passed") {
		t.Fatalf("validate startup-prepared index text output stdout=%q stderr=%q", stdout, stderr)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("validate did not prepare configured database: %v", err)
	}

	stdout, stderr, err = runCmd("status")
	if err != nil {
		t.Fatalf("status startup-prepared index failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" || !strings.Contains(stdout, "Files indexed:    0") || !strings.Contains(stdout, "Messages indexed: 0") {
		t.Fatalf("status startup-prepared index text output stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, err = runCmd("validate", "--json")
	if err != nil {
		t.Fatalf("validate --json startup-prepared index failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("validate --json startup-prepared index wrote stderr: %q", stderr)
	}
	var validatePayload struct {
		Valid          bool `json:"valid"`
		DatabaseExists bool `json:"database_exists"`
	}
	if err := json.Unmarshal([]byte(stdout), &validatePayload); err != nil {
		t.Fatalf("validate --json startup-prepared index emitted invalid JSON %q: %v", stdout, err)
	}
	if !validatePayload.Valid || !validatePayload.DatabaseExists {
		t.Fatalf("validate --json startup-prepared index payload = %+v, want valid existing database", validatePayload)
	}

	stdout, stderr, err = runCmd("status", "--json")
	if err != nil {
		t.Fatalf("status --json startup-prepared index failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("status --json startup-prepared index wrote stderr: %q", stderr)
	}
	var statusPayload struct {
		Database struct {
			Exists bool `json:"exists"`
			Size   int  `json:"size"`
		} `json:"database"`
		Index struct {
			Usable        bool `json:"usable"`
			TotalFiles    int  `json:"total_files"`
			TotalMessages int  `json:"total_messages"`
		} `json:"index"`
	}
	if err := json.Unmarshal([]byte(stdout), &statusPayload); err != nil {
		t.Fatalf("status --json startup-prepared index emitted invalid JSON %q: %v", stdout, err)
	}
	if !statusPayload.Database.Exists || statusPayload.Database.Size == 0 {
		t.Fatalf("status --json startup-prepared database payload = %+v, want existing database", statusPayload.Database)
	}
	if statusPayload.Index.TotalFiles != 0 || statusPayload.Index.TotalMessages != 0 {
		t.Fatalf("status --json startup-prepared index counts = %+v, want zero files/messages", statusPayload.Index)
	}
	if statusPayload.Index.Usable {
		t.Fatalf("status --json startup-prepared index usable=%v with total_files=0; production contract requires usable=false for empty index", statusPayload.Index.Usable)
	}
}

func TestValidateCurrentIndexIntegrityFailureIsGenericAndReadOnly(t *testing.T) {
	dbPath := newOrphanedCurrentDiagnosticDB(t)
	before := snapshotSQLiteFiles(t, dbPath)

	stdout, stderr, err := runCmd("validate", "--json")
	if err == nil {
		t.Fatalf("validate --json orphaned index succeeded; stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("validate --json orphaned index wrote stderr: %q", stderr)
	}
	var payload struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("validate --json orphaned index emitted invalid JSON %q: %v", stdout, err)
	}
	if payload.Valid || !strings.Contains(payload.Error, "orphaned search_items") {
		t.Fatalf("validate --json orphaned index payload = %+v, want generic validation failure", payload)
	}
	assertSQLiteFilesUnchanged(t, dbPath, before)

	stdout, stderr, err = runCmd("validate")
	if err == nil {
		t.Fatalf("validate orphaned index succeeded; stdout=%q stderr=%q", stdout, stderr)
	}
	combined := strings.ToLower(stdout + stderr)
	if !strings.Contains(combined, "validation failed") || !strings.Contains(combined, "orphaned search_items") {
		t.Fatalf("validate orphaned index output = stdout=%q stderr=%q, want generic validation failure", stdout, stderr)
	}
	for _, forbidden := range []string{"diagnostic:", "continuation:", "recover --from"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("validate orphaned index emitted recovery diagnostic %q in stdout=%q stderr=%q", forbidden, stdout, stderr)
		}
	}
	assertSQLiteFilesUnchanged(t, dbPath, before)
}

func TestSearchRobotDiagnosticIsMachineReadableAndPreservesIndexBytes(t *testing.T) {
	dbPath := newUnsupportedIndexedConsumerDB(t)
	before := readDBBytes(t, dbPath)

	stdout, stderr, err := runCmd("search", "sentinel", "--robot")
	if err == nil {
		t.Fatalf("search --robot unsupported index succeeded; stdout=%q stderr=%q", stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("search --robot diagnostic wrote stderr: %q", stderr)
	}
	for _, want := range []string{
		"diagnostic_code=" + string(compat.CodeUnsupportedLineage),
		"diagnostic_summary=",
		fmt.Sprintf(`diagnostic_continuation_argv=["recover","--from","%s","--dry-run"]`, dbPath),
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("search --robot diagnostic missing %q in %q", want, stdout)
		}
	}
	if after := readDBBytes(t, dbPath); !bytes.Equal(after, before) {
		t.Fatal("search --robot diagnostic mutated unsupported database bytes")
	}
}

func TestLiveWALStartupUsesCompatibleIndexWithoutRecoveryDiagnostic(t *testing.T) {
	dbPath, closeWriter := newLiveWALDiagnosticDB(t)
	defer closeWriter()

	t.Run("status --json", func(t *testing.T) {
		stdout, stderr, err := runCmd("status", "--json")
		if err != nil {
			t.Fatalf("status --json failed with live WAL; stdout=%q stderr=%q err=%v", stdout, stderr, err)
		}
		if stderr != "" {
			t.Fatalf("status --json wrote stderr: %q", stderr)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("status --json emitted invalid JSON %q: %v", stdout, err)
		}
		if _, hasCode := payload["code"]; hasCode {
			t.Fatalf("status --json unexpectedly emitted diagnostic payload: %q", stdout)
		}
		database, ok := payload["database"].(map[string]any)
		if !ok {
			t.Fatalf("status --json missing database payload: %q", stdout)
		}
		index, ok := payload["index"].(map[string]any)
		if !ok {
			t.Fatalf("status --json missing index payload: %q", stdout)
		}
		if exists, _ := database["exists"].(bool); !exists {
			t.Fatalf("status --json database.exists=false, payload=%v", database)
		}
		totalFiles, _ := index["total_files"].(float64)
		if totalFiles < 1 {
			t.Fatalf("status --json expected indexed rows, payload=%v", index)
		}
	})

	t.Run("validate --json", func(t *testing.T) {
		stdout, stderr, err := runCmd("validate", "--json")
		if err != nil {
			t.Fatalf("validate --json failed with live WAL; stdout=%q stderr=%q err=%v", stdout, stderr, err)
		}
		if stderr != "" {
			t.Fatalf("validate --json wrote stderr: %q", stderr)
		}
		if strings.Contains(strings.ToLower(stdout), "diagnostic") {
			t.Fatalf("validate --json unexpectedly emitted diagnostic payload: %q", stdout)
		}
		var payload struct {
			Valid bool `json:"valid"`
		}
		if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
			t.Fatalf("validate --json emitted invalid JSON %q: %v", stdout, err)
		}
		if !payload.Valid {
			t.Fatalf("validate --json payload = %+v, want valid=true", payload)
		}
	})

	t.Run("status text", func(t *testing.T) {
		stdout, stderr, err := runCmd("status")
		if err != nil {
			t.Fatalf("status text failed with live WAL; stdout=%q stderr=%q err=%v", stdout, stderr, err)
		}
		if stderr != "" {
			t.Fatalf("status text wrote stderr: %q", stderr)
		}
		out := strings.ToLower(stdout)
		if strings.Contains(out, "diagnostic:") || strings.Contains(out, "continuation:") || strings.Contains(out, "recover --from") {
			t.Fatalf("status text emitted unexpected recovery/diagnostic guidance: %q", stdout)
		}
		for _, want := range []string{"files indexed", "messages indexed"} {
			if !strings.Contains(out, want) {
				t.Fatalf("status text missing %q in %q", want, stdout)
			}
		}
	})

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("live WAL database path missing after startup: %v", err)
	}
}

func TestRecoveryContinuationExecutesInConfiguredSamePathContextWithEmptyWAL(t *testing.T) {
	dbPath := newFixtureIndexDB(t, "v13-development-alter-built.sql")
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	walPath := dbPath + "-wal"
	if err := os.WriteFile(walPath, nil, 0o600); err != nil {
		t.Fatalf("write empty WAL: %v", err)
	}
	before := snapshotSQLiteFiles(t, dbPath)
	walBefore, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("stat empty WAL before continuation: %v", err)
	}

	emptyInputs := filepath.Join(t.TempDir(), "empty-inputs")
	if err := os.MkdirAll(emptyInputs, 0o755); err != nil {
		t.Fatalf("mkdir empty recovery inputs: %v", err)
	}
	cfg := &config.Config{DatabasePath: dbPath, SessionDirs: []string{emptyInputs}}
	startupDiagnostic := continuationFor(compat.Diagnostic{Code: compat.CodeUnsupportedLineage, Summary: "fixture diagnostic"}, dbPath)
	startupErr := indexDiagnosticError{diagnostic: startupDiagnostic}

	var stdout, stderr bytes.Buffer
	root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer, startupCommandClass) startupResult {
		return startupResult{Config: cfg, Failure: &startupFailure{
			Stage:       startupStageIndexPrepare,
			Cause:       startupErr,
			Diagnostic:  startupDiagnostic,
			Recoverable: true,
		}}
	})
	root.SetArgs(startupDiagnostic.Continuation)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute continuation %v after startup diagnostic %s: %v\nstdout=%q stderr=%q", startupDiagnostic.Continuation, startupDiagnostic.Code, err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "recovery dry run") {
		t.Fatalf("continuation output = %q, want recovery dry run", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("continuation stderr = %q, want empty", stderr.String())
	}
	assertSQLiteFilesUnchanged(t, dbPath, before)
	walAfter, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("stat empty WAL after continuation: %v", err)
	}
	if walAfter.Size() != 0 || walAfter.Mode() != walBefore.Mode() || !walAfter.ModTime().Equal(walBefore.ModTime()) {
		t.Fatalf("empty WAL metadata changed: before=%+v after=%+v", walBefore, walAfter)
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

func newMixedRecoveryDiagnosticDB(t *testing.T) string {
	t.Helper()
	dbPath, db := newEmptyCurrentDiagnosticDB(t)
	_, err := db.DB().Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, project, content_type)
		VALUES ('session', '/conflict/session.jsonl', 7, 'user', 'first conflicting payload', '2026-08-18T00:00:00Z', 'project', 'text'),
		       ('session', '/conflict/session.jsonl', 7, 'assistant', 'second conflicting payload', '2026-08-18T00:00:01Z', 'project', 'text'),
		       ('session', '', -1, 'user', 'missing identity payload', '2026-08-18T00:00:02Z', 'project', 'text')
	`)
	if err != nil {
		_ = db.Close()
		t.Fatalf("seed mixed recovery diagnostics: %v", err)
	}
	if err := closeSeedDB(t, db); err != nil {
		t.Fatalf("close mixed diagnostics db: %v", err)
	}
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	return dbPath
}

func newOrphanedCurrentDiagnosticDB(t *testing.T) string {
	t.Helper()
	dbPath, db := newEmptyCurrentDiagnosticDB(t)
	_, err := db.DB().Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
		VALUES ('session', '/orphaned/session.jsonl', 0, 'user', 'orphaned validation sentinel', '2026-08-18T00:00:00Z', '11111111-1111-4111-8111-111111111111', 'project', 'text')
	`)
	if err != nil {
		_ = db.Close()
		t.Fatalf("seed orphaned current db: %v", err)
	}
	if err := closeSeedDB(t, db); err != nil {
		t.Fatalf("close orphaned db: %v", err)
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
