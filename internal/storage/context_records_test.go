package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestQueryContextRecordsSelectorsWindowAndIsolation(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	const pathA = "/sessions/context-a.jsonl"
	const pathB = "/sessions/context-b.jsonl"
	insertContextRecord(t, db, 101, "session", pathA, 10, "user", models.OriginHuman, "first full text", contextString("a-10"), contextString("2026-01-01T00:00:10Z"), "text")
	insertContextRecord(t, db, 102, "session", pathA, 20, "assistant", models.OriginAssistant, "repeat first", contextString("a-20-first"), contextString("2026-01-01T00:00:20Z"), "text")
	insertContextRecord(t, db, 103, "session", pathA, 20, "user", models.OriginAssistant, "repeat second with complete indexed text", nil, nil, "code")
	insertContextRecord(t, db, 104, "plan", pathA, 40, "system", models.OriginSystem, "gap anchor", contextString("opaque:not-a-uuid"), contextString("2026-01-01T00:00:40Z"), "text/markdown")
	insertContextRecord(t, db, 105, "session", pathA, 100, "assistant", models.OriginAutomation, "last", contextString("a-100"), nil, "tool")
	insertContextRecord(t, db, 106, "session", pathB, 20, "user", models.OriginHuman, "other path must not leak", contextString("b-20"), nil, "text")

	byUUID, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{
		UUID: contextString("a-20-first"), Before: 1, After: 2,
	})
	if err != nil {
		t.Fatalf("QueryContextRecords by UUID: %v", err)
	}
	assertContextRecordOrder(t, byUUID, []string{"first full text", "repeat first", "repeat second with complete indexed text", "gap anchor"})
	assertSingleContextAnchor(t, byUUID, "repeat first")
	for _, record := range byUUID {
		if record.SourcePath != pathA {
			t.Fatalf("UUID window leaked source_path %q", record.SourcePath)
		}
	}

	ordinal := int64(40)
	byPathOrdinal, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{
		SourcePath: contextString(pathA), Ordinal: &ordinal, Before: 2, After: 1,
	})
	if err != nil {
		t.Fatalf("QueryContextRecords by path/ordinal: %v", err)
	}
	assertContextRecordOrder(t, byPathOrdinal, []string{"repeat first", "repeat second with complete indexed text", "gap anchor", "last"})
	assertSingleContextAnchor(t, byPathOrdinal, "gap anchor")
	anchor := byPathOrdinal[2]
	if anchor.UUID == nil || *anchor.UUID != "opaque:not-a-uuid" || anchor.Source != "plan" || anchor.ContentType != "text/markdown" {
		t.Fatalf("anchor canonical fields = %+v", anchor)
	}
	if anchor.Timestamp == nil || *anchor.Timestamp != "2026-01-01T00:00:40Z" || anchor.Origin != models.OriginSystem {
		t.Fatalf("anchor timestamp/origin = %v/%q", anchor.Timestamp, anchor.Origin)
	}
	if got := byPathOrdinal[1]; got.UUID != nil || got.Timestamp != nil || got.Origin != models.OriginAssistant || got.Role != "user" {
		t.Fatalf("nullable fields or persisted origin changed: %+v", got)
	}
}

func TestQueryContextRecordsEdgesAndGapsUsePosition(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	const sourcePath = "/sessions/context-edges.jsonl"
	for i, row := range []struct {
		ordinal int64
		uuid    string
	}{
		{2, "edge-2"},
		{50, "edge-50"},
		{900, "edge-900"},
	} {
		insertContextRecord(t, db, int64(201+i), "session", sourcePath, row.ordinal, "user", models.OriginHuman, row.uuid, contextString(row.uuid), nil, "text")
	}

	first, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: contextString("edge-2"), Before: 50, After: 1})
	if err != nil {
		t.Fatal(err)
	}
	assertContextRecordOrder(t, first, []string{"edge-2", "edge-50"})
	assertSingleContextAnchor(t, first, "edge-2")

	last, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: contextString("edge-900"), Before: 1, After: 50})
	if err != nil {
		t.Fatal(err)
	}
	assertContextRecordOrder(t, last, []string{"edge-50", "edge-900"})
	assertSingleContextAnchor(t, last, "edge-900")
}

func TestQueryContextRecordsTypedResolutionErrors(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	const sourcePath = "/sessions/context-ambiguous.jsonl"
	insertContextRecord(t, db, 301, "session", sourcePath, 7, "user", models.OriginHuman, "duplicate one", nil, nil, "text")
	insertContextRecord(t, db, 302, "session", sourcePath, 7, "assistant", models.OriginAssistant, "duplicate two", nil, nil, "text")
	ordinal := int64(7)
	_, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{SourcePath: contextString(sourcePath), Ordinal: &ordinal})
	if !errors.Is(err, ErrContextRecordAmbiguous) {
		t.Fatalf("path/ordinal error = %v, want ErrContextRecordAmbiguous", err)
	}
	var ambiguous *ContextRecordAmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("path/ordinal error type = %T, want *ContextRecordAmbiguousError", err)
	}

	missingOrdinal := int64(99)
	_, err = db.QueryContextRecords(context.Background(), ContextRecordQuery{SourcePath: contextString(sourcePath), Ordinal: &missingOrdinal})
	if !errors.Is(err, ErrContextRecordNotFound) {
		t.Fatalf("path/ordinal missing error = %v, want ErrContextRecordNotFound", err)
	}
	var notFound *ContextRecordNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("path/ordinal missing error type = %T, want *ContextRecordNotFoundError", err)
	}

	_, err = db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: contextString("missing opaque identity")})
	if !errors.Is(err, ErrContextRecordNotFound) || !errors.As(err, &notFound) {
		t.Fatalf("UUID missing error = %v, want typed not-found", err)
	}
}

func TestQueryContextRecordsTreatsDuplicateUUIDDefensively(t *testing.T) {
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`
		CREATE TABLE search_items (
			id INTEGER PRIMARY KEY,
			uuid TEXT,
			source_path TEXT NOT NULL,
			ordinal INTEGER NOT NULL
		);
		INSERT INTO search_items (id, uuid, source_path, ordinal) VALUES
			(1, 'duplicate-uuid', '/one', 1),
			(2, 'duplicate-uuid', '/two', 2),
			(3, 'duplicate-uuid', '/three', 3);
	`); err != nil {
		t.Fatal(err)
	}
	db := &Database{db: raw}
	_, err = db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: contextString("duplicate-uuid")})
	if !errors.Is(err, ErrContextRecordAmbiguous) {
		t.Fatalf("duplicate UUID error = %v, want ErrContextRecordAmbiguous", err)
	}
	var typed *ContextRecordAmbiguousError
	if !errors.As(err, &typed) {
		t.Fatalf("duplicate UUID error type = %T", err)
	}
}

func TestQueryContextRecordsUsesSQLiteForDeletedAndRecoverySources(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	deletedPath := filepath.Join(t.TempDir(), "deleted-source.jsonl")
	if err := os.WriteFile(deletedPath, []byte("source payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO indexed_files (path, hash) VALUES (?, 'deleted-hash')`, deletedPath); err != nil {
		t.Fatal(err)
	}
	insertContextRecord(t, db, 401, "session", deletedPath, 1, "user", models.OriginHuman, "deleted source record", contextString("deleted-record"), nil, "text")
	if err := os.Remove(deletedPath); err != nil {
		t.Fatal(err)
	}

	deleted, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: contextString("deleted-record")})
	if err != nil {
		t.Fatalf("query deleted source: %v", err)
	}
	assertContextRecordOrder(t, deleted, []string{"deleted source record"})

	const recoveryPath = "/recovery/no-indexed-file-entry.jsonl"
	insertContextRecord(t, db, 402, "decision", recoveryPath, 77, "assistant", models.OriginAutomation, "recovery destination record", contextString("recovery-record"), nil, "reasoning")
	var indexedFiles int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files WHERE path = ?`, recoveryPath).Scan(&indexedFiles); err != nil {
		t.Fatal(err)
	}
	if indexedFiles != 0 {
		t.Fatalf("recovery fixture unexpectedly has indexed_files row")
	}
	recovered, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: contextString("recovery-record")})
	if err != nil {
		t.Fatalf("query recovery record: %v", err)
	}
	if len(recovered) != 1 || recovered[0].Source != "decision" || recovered[0].Origin != models.OriginAutomation || recovered[0].Text != "recovery destination record" {
		t.Fatalf("recovery context = %+v", recovered)
	}
}

func TestQueryContextRecordsRejectsInvalidSelectorsAndLimits(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	ordinal := int64(0)
	validUUID := contextString("opaque")
	validPath := contextString("/session")
	tests := []struct {
		name  string
		query ContextRecordQuery
	}{
		{name: "no selector"},
		{name: "both selectors", query: ContextRecordQuery{UUID: validUUID, SourcePath: validPath, Ordinal: &ordinal}},
		{name: "path without ordinal", query: ContextRecordQuery{SourcePath: validPath}},
		{name: "ordinal without path", query: ContextRecordQuery{Ordinal: &ordinal}},
		{name: "uuid and path partial", query: ContextRecordQuery{UUID: validUUID, SourcePath: validPath}},
		{name: "negative before", query: ContextRecordQuery{UUID: validUUID, Before: -1}},
		{name: "before over max", query: ContextRecordQuery{UUID: validUUID, Before: 51}},
		{name: "negative after", query: ContextRecordQuery{UUID: validUUID, After: -1}},
		{name: "after over max", query: ContextRecordQuery{UUID: validUUID, After: 51}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.QueryContextRecords(context.Background(), tc.query)
			if !errors.Is(err, ErrInvalidContextRecordQuery) {
				t.Fatalf("error = %v, want ErrInvalidContextRecordQuery", err)
			}
		})
	}
}

func TestQueryContextRecordsHonorsCanceledContext(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := db.QueryContextRecords(ctx, ContextRecordQuery{UUID: contextString("anything")})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func insertContextRecord(t *testing.T, db *Database, id int64, source, sourcePath string, ordinal int64, role string, origin models.MessageOrigin, text string, uuid, timestamp *string, contentType string) {
	t.Helper()
	if _, err := db.DB().Exec(`
		INSERT INTO search_items
			(id, source, source_path, ordinal, role, origin, text, uuid, timestamp, content_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, source, sourcePath, ordinal, role, origin, text, uuid, timestamp, contentType); err != nil {
		t.Fatalf("insert context record %d: %v", id, err)
	}
}

func contextString(value string) *string { return &value }

func assertContextRecordOrder(t *testing.T, records []ContextRecord, want []string) {
	t.Helper()
	got := make([]string, len(records))
	for i, record := range records {
		got[i] = record.Text
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("context order = %v, want %v", got, want)
	}
}

func assertSingleContextAnchor(t *testing.T, records []ContextRecord, wantText string) {
	t.Helper()
	anchors := 0
	for _, record := range records {
		if record.Anchor {
			anchors++
			if record.Text != wantText {
				t.Fatalf("anchor text = %q, want %q", record.Text, wantText)
			}
		}
	}
	if anchors != 1 {
		t.Fatalf("anchor count = %d, want 1", anchors)
	}
}
