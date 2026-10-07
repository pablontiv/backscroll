package storage

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"modernc.org/sqlite"
)

const cancelSyncFunction = "backscroll_test_cancel_sync_context"

var (
	cancelSyncMu sync.Mutex
	cancelSync   context.CancelFunc
)

func init() {
	sqlite.MustRegisterScalarFunction(cancelSyncFunction, 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
		cancelSyncMu.Lock()
		cancel := cancelSync
		cancelSyncMu.Unlock()
		if cancel != nil {
			cancel()
		}
		return int64(0), nil
	})
}

func setSyncCancellation(t *testing.T, cancel context.CancelFunc) {
	t.Helper()
	cancelSyncMu.Lock()
	cancelSync = cancel
	cancelSyncMu.Unlock()
	t.Cleanup(func() {
		cancelSyncMu.Lock()
		cancelSync = nil
		cancelSyncMu.Unlock()
	})
}

func TestSyncFilesContextAlreadyCanceled(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := db.SyncFilesContext(ctx, []IndexedFile{{
		SourcePath: "/cancel/already.jsonl",
		Source:     "session",
		Hash:       "already-canceled",
		Messages:   []IndexedMessage{{Ordinal: 0, UUID: "already-canceled", Role: "user", Text: "not written", ContentType: "text"}},
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SyncFilesContext error = %v, want context.Canceled", err)
	}
	assertTableCount(t, db, "search_items", 0)
	assertTableCount(t, db, "indexed_files", 0)
}

func TestSyncFilesContextCancellationDuringBatchRollsBackEverything(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	setSyncCancellation(t, cancel)
	if _, err := db.db.Exec(fmt.Sprintf(`
		CREATE TRIGGER cancel_sync_during_batch
		AFTER INSERT ON search_items
		WHEN NEW.source_path = '/cancel/second.jsonl'
		BEGIN
			SELECT %s();
		END
	`, cancelSyncFunction)); err != nil {
		t.Fatalf("create cancellation trigger: %v", err)
	}

	isError := true
	err := db.SyncFilesContext(ctx, []IndexedFile{
		{
			SourcePath: "/cancel/first.jsonl",
			Source:     "session",
			Hash:       "first-hash",
			Tags:       []string{"rollback-tag"},
			Messages: []IndexedMessage{
				{Ordinal: 0, UUID: "cancel-first-user", Role: "user", Text: "this is wrong", ContentType: "text", ExtractionVersion: CurrentExtractionVersion},
				{Ordinal: 1, UUID: "cancel-first-tool", Role: "assistant", Text: "error: operation failed code=7", ContentType: "tool", ToolName: "Bash", IsError: &isError, ExtractionVersion: CurrentExtractionVersion},
			},
		},
		{
			SourcePath: "/cancel/second.jsonl",
			Source:     "session",
			Hash:       "second-hash",
			Messages:   []IndexedMessage{{Ordinal: 0, UUID: "cancel-second", Role: "user", Text: "trigger cancellation", ContentType: "text"}},
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SyncFilesContext error = %v, want context.Canceled", err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("context error = %v, want context.Canceled", ctx.Err())
	}

	for _, table := range []string{
		"search_items",
		"tool_events",
		"message_templates",
		"template_matches",
		"correction_signals",
		"session_tags",
		"indexed_files",
		"dynamic_stopwords",
	} {
		assertTableCount(t, db, table, 0)
	}
}

func TestSyncTransactionGateNormalizesSQLiteAutomaticCancellationRollback(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if _, err := db.db.Exec(`CREATE TABLE cancellation_rollback_probe (value INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create rollback probe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	setSyncCancellation(t, cancel)
	tx, err := db.db.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	gate := newSyncTransactionGate(ctx, tx)

	// Hold the gate until ExecContext returns. The SQLite interrupt therefore
	// completes its automatic rollback before the gate calls Rollback.
	gate.mu.Lock()
	_, writeErr := tx.ExecContext(ctx, fmt.Sprintf(`
		WITH RECURSIVE seq(x) AS (
			VALUES(1)
			UNION ALL
			SELECT x + 1 FROM seq WHERE x < 100000
		)
		INSERT INTO cancellation_rollback_probe(value)
		SELECT CASE WHEN x = 1000 THEN %s() ELSE x END FROM seq
	`, cancelSyncFunction))
	ctxErr := ctx.Err()
	if ctxErr != nil {
		<-gate.callbackStarted
	}
	gate.mu.Unlock()
	if ctxErr != context.Canceled {
		t.Fatalf("context error = %v, want context.Canceled; write error = %v", ctxErr, writeErr)
	}
	<-gate.callbackDone

	gate.mu.Lock()
	rawRollbackErr := gate.rollbackErr
	gate.mu.Unlock()
	var sqliteErr *sqlite.Error
	if !errors.As(rawRollbackErr, &sqliteErr) {
		t.Fatalf("raw rollback error = %T %v, want *sqlite.Error", rawRollbackErr, rawRollbackErr)
	}
	const completedRollbackMessage = "SQL logic error: cannot rollback - no transaction is active (1)"
	if sqliteErr.Code() != 1 || sqliteErr.Error() != completedRollbackMessage {
		t.Fatalf("raw rollback error = code %d, %q; want code 1, %q", sqliteErr.Code(), sqliteErr.Error(), completedRollbackMessage)
	}
	if rollbackErr := gate.rollback(); rollbackErr != nil {
		t.Fatalf("normalized gate rollback = %v, want nil", rollbackErr)
	}

	finalErr := writeErr
	if cancelErr := gate.cancellationBeforeCommit(); cancelErr != nil && !errors.Is(finalErr, cancelErr) {
		finalErr = errors.Join(cancelErr, finalErr)
	}
	if !errors.Is(finalErr, context.Canceled) {
		t.Fatalf("final error = %v, want context.Canceled", finalErr)
	}
	if strings.Contains(finalErr.Error(), "rollback transaction") {
		t.Fatalf("final error contains benign rollback failure: %v", finalErr)
	}

	assertTableCount(t, db, "cancellation_rollback_probe", 0)
}

func TestSyncFilesContextStopwordFailureRollsBackSyncAndStopwords(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if _, err := db.db.Exec(`INSERT INTO dynamic_stopwords(term) VALUES ('legacy-stopword')`); err != nil {
		t.Fatalf("seed stopword: %v", err)
	}
	if _, err := db.db.Exec(`
		CREATE TRIGGER reject_refreshed_stopwords
		BEFORE INSERT ON dynamic_stopwords
		WHEN NEW.term <> 'legacy-stopword'
		BEGIN
			SELECT RAISE(ABORT, 'stopword refresh rejected');
		END
	`); err != nil {
		t.Fatalf("create stopword trigger: %v", err)
	}

	err := db.SyncFilesContext(context.Background(), []IndexedFile{{
		SourcePath: "/rollback/stopwords.jsonl",
		Source:     "session",
		Hash:       "stopword-hash",
		Messages: []IndexedMessage{{
			Ordinal: 0, UUID: "stopword-row", Role: "user",
			Text: "alpha alpha alpha beta gamma", ContentType: "text",
		}},
	}})
	if err == nil {
		t.Fatal("SyncFilesContext succeeded despite stopword insertion trigger")
	}

	assertTableCount(t, db, "search_items", 0)
	assertTableCount(t, db, "indexed_files", 0)
	var term string
	if err := db.db.QueryRow(`SELECT term FROM dynamic_stopwords`).Scan(&term); err != nil {
		t.Fatalf("read rolled-back stopword: %v", err)
	}
	if term != "legacy-stopword" {
		t.Fatalf("stopword after rollback = %q, want legacy-stopword", term)
	}
}

func TestSyncFilesContextMatchesLegacyWrapper(t *testing.T) {
	contextDB, contextCleanup := newTestDB(t)
	defer contextCleanup()
	legacyDB, legacyCleanup := newTestDB(t)
	defer legacyCleanup()

	isError := true
	files := []IndexedFile{{
		SourcePath: "/equivalent/session.jsonl",
		Source:     "session",
		Hash:       "equivalent-hash",
		Project:    "equivalent-project",
		Tags:       []string{"equivalent-tag"},
		Messages: []IndexedMessage{
			{Ordinal: 0, UUID: "equivalent-user", Role: "user", Text: "that's wrong", ContentType: "text", ExtractionVersion: CurrentExtractionVersion},
			{Ordinal: 1, UUID: "equivalent-tool", Role: "assistant", Text: "error: equivalent failure", ContentType: "tool", ToolName: "Bash", CommandHead: "go", IsError: &isError, ExtractionVersion: CurrentExtractionVersion},
		},
	}}

	if err := contextDB.SyncFilesContext(context.Background(), files); err != nil {
		t.Fatalf("SyncFilesContext: %v", err)
	}
	if err := legacyDB.SyncFiles(files); err != nil {
		t.Fatalf("legacy SyncFiles wrapper: %v", err)
	}

	got := readSyncOutcome(t, contextDB)
	want := readSyncOutcome(t, legacyDB)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("context outcome differs from legacy wrapper\ncontext: %#v\nlegacy:  %#v", got, want)
	}
	if got.counts["search_items"] == 0 || got.counts["indexed_files"] == 0 {
		t.Fatalf("normal sync did not persist expected rows: %#v", got.counts)
	}
}

type syncOutcome struct {
	counts     map[string]int
	items      []string
	fileHashes map[string]string
	stopwords  []string
}

func readSyncOutcome(t *testing.T, db *Database) syncOutcome {
	t.Helper()
	out := syncOutcome{counts: make(map[string]int)}
	for _, table := range []string{
		"search_items", "tool_events", "message_templates", "template_matches",
		"correction_signals", "session_tags", "indexed_files", "dynamic_stopwords",
	} {
		out.counts[table] = tableCount(t, db, table)
	}

	rows, err := db.db.Query(`SELECT source_path, ordinal, text, COALESCE(uuid, '') FROM search_items ORDER BY source_path, ordinal`)
	if err != nil {
		t.Fatalf("query sync outcome: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var path, text, uuid string
		var ordinal int
		if err := rows.Scan(&path, &ordinal, &text, &uuid); err != nil {
			t.Fatalf("scan sync outcome: %v", err)
		}
		out.items = append(out.items, fmt.Sprintf("%s|%d|%s|%s", path, ordinal, text, uuid))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sync outcome: %v", err)
	}

	out.fileHashes, err = db.GetFileHashes()
	if err != nil {
		t.Fatalf("get file hashes: %v", err)
	}
	stopwords, err := db.loadStopwords()
	if err != nil {
		t.Fatalf("load stopwords: %v", err)
	}
	for term := range stopwords {
		out.stopwords = append(out.stopwords, term)
	}
	sort.Strings(out.stopwords)
	return out
}

func assertTableCount(t *testing.T, db *Database, table string, want int) {
	t.Helper()
	if got := tableCount(t, db, table); got != want {
		t.Fatalf("%s row count = %d, want %d", table, got, want)
	}
}

func tableCount(t *testing.T, db *Database, table string) int {
	t.Helper()
	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}
