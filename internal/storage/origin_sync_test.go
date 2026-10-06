package storage

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestOriginPersistenceNormalizesAndVersionsCurrentRows(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "origin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	messages := []IndexedMessage{
		{Ordinal: 0, UUID: "origin-human", Role: "user", Origin: models.OriginHuman, Text: "human", ContentType: "text", ExtractionVersion: 2},
		{Ordinal: 1, UUID: "origin-empty", Role: "user", Text: "empty", ContentType: "text", ExtractionVersion: 2},
		{Ordinal: 2, UUID: "origin-invalid", Role: "user", Origin: models.MessageOrigin("operator"), Text: "invalid", ContentType: "text", ExtractionVersion: 2},
	}
	if err := db.SyncFiles([]IndexedFile{{SourcePath: "/sessions/origins.jsonl", Source: "session", Hash: "h1", Messages: messages}}); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		uuid string
		want models.MessageOrigin
	}{
		{uuid: "origin-human", want: models.OriginHuman},
		{uuid: "origin-empty", want: models.OriginUnknown},
		{uuid: "origin-invalid", want: models.OriginUnknown},
	} {
		var origin models.MessageOrigin
		var originVersion, extractionVersion int
		if err := db.db.QueryRow(`SELECT origin, origin_version, extraction_version FROM search_items WHERE uuid = ?`, tt.uuid).
			Scan(&origin, &originVersion, &extractionVersion); err != nil {
			t.Fatalf("read %s: %v", tt.uuid, err)
		}
		if origin != tt.want || originVersion != CurrentOriginVersion || extractionVersion != 2 {
			t.Errorf("%s persisted (%q, %d, %d), want (%q, %d, 2)", tt.uuid, origin, originVersion, extractionVersion, tt.want, CurrentOriginVersion)
		}
	}
	if CurrentExtractionVersion != 3 {
		t.Fatalf("general extraction version changed to %d", CurrentExtractionVersion)
	}
}

func TestOriginPerennialEnrichmentPreservesPayloadAndDoesNotDegrade(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "origin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	file := IndexedFile{SourcePath: "/sessions/perennial.jsonl", Source: "session", Hash: "h1", Messages: []IndexedMessage{{
		Ordinal: 0, UUID: "stable-origin", Role: "user", Origin: models.OriginUnknown,
		Text: "original text", ContentType: "text", ExtractionVersion: 1,
	}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}

	var originalID int64
	if err := db.db.QueryRow(`SELECT id FROM search_items WHERE uuid = 'stable-origin'`).Scan(&originalID); err != nil {
		t.Fatal(err)
	}

	file.Hash = "h2"
	file.Messages[0].Origin = models.OriginHuman
	file.Messages[0].Text = "parser drift must not replace text"
	file.Messages[0].ExtractionVersion = 99
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("enrich origin: %v", err)
	}
	assertOriginRow(t, db, "stable-origin", originalID, models.OriginHuman, "original text", 1)

	file.Hash = "h3"
	file.Messages[0].Origin = models.OriginUnknown
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("sync partial origin: %v", err)
	}
	assertOriginRow(t, db, "stable-origin", originalID, models.OriginHuman, "original text", 1)
}

func TestOriginPerennialReplayClosesRetainedRowsWithoutEvidence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "origin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/sessions/retained.jsonl"
	file := IndexedFile{SourcePath: path, Source: "session", Hash: "h1", Messages: []IndexedMessage{
		{Ordinal: 0, UUID: "historical-origin", Role: "user", Origin: models.OriginHuman, Text: "retained payload", ContentType: "text", ExtractionVersion: 1},
		{Ordinal: 1, UUID: "current-origin", Role: "assistant", Origin: models.OriginAssistant, Text: "still emitted", ContentType: "text", ExtractionVersion: 1},
	}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}

	var retainedID int64
	if err := db.db.QueryRow(`SELECT id FROM search_items WHERE uuid = 'historical-origin'`).Scan(&retainedID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET origin = 'unknown', origin_version = NULL WHERE uuid = 'historical-origin'`); err != nil {
		t.Fatal(err)
	}

	file.Hash = "h2"
	file.Messages = file.Messages[1:]
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("replay current parser output: %v", err)
	}

	var gotID int64
	var gotUUID, gotText string
	var gotOrigin models.MessageOrigin
	var gotVersion int
	if err := db.db.QueryRow(`
		SELECT id, uuid, text, origin, origin_version
		FROM search_items WHERE uuid = 'historical-origin'
	`).Scan(&gotID, &gotUUID, &gotText, &gotOrigin, &gotVersion); err != nil {
		t.Fatal(err)
	}
	if gotID != retainedID || gotUUID != "historical-origin" || gotText != "retained payload" {
		t.Fatalf("retained payload changed: id=%d uuid=%q text=%q; want id=%d uuid=%q text=%q",
			gotID, gotUUID, gotText, retainedID, "historical-origin", "retained payload")
	}
	if gotOrigin != models.OriginUnknown || gotVersion != CurrentOriginVersion {
		t.Fatalf("retained provenance = (%q, %d), want (%q, %d)", gotOrigin, gotVersion, models.OriginUnknown, CurrentOriginVersion)
	}
	if pending, err := db.PendingOriginPaths(10); err != nil || len(pending) != 0 {
		t.Fatalf("origin queue after successful replay = %v, err=%v", pending, err)
	}
}

func TestOriginRejectsContradictoryProofForIdentity(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "origin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	first := IndexedFile{SourcePath: "/sessions/first.jsonl", Source: "session", Hash: "h1", Messages: []IndexedMessage{{
		Ordinal: 0, UUID: "shared-origin", Role: "user", Origin: models.OriginHuman, Text: "retained", ContentType: "text",
	}}}
	if err := db.SyncFiles([]IndexedFile{first}); err != nil {
		t.Fatal(err)
	}

	contradiction := IndexedFile{SourcePath: "/sessions/second.jsonl", Source: "session", Hash: "h2", Messages: []IndexedMessage{{
		Ordinal: 0, UUID: "shared-origin", Role: "assistant", Origin: models.OriginAssistant, Text: "collision", ContentType: "text",
	}}}
	err = db.SyncFiles([]IndexedFile{contradiction})
	if err == nil || !strings.Contains(err.Error(), "conflicting proven origins") {
		t.Fatalf("contradictory proof error = %v", err)
	}

	var origin models.MessageOrigin
	var text string
	if err := db.db.QueryRow(`SELECT origin, text FROM search_items WHERE uuid = 'shared-origin'`).Scan(&origin, &text); err != nil {
		t.Fatal(err)
	}
	if origin != models.OriginHuman || text != "retained" {
		t.Fatalf("conflict mutated retained row to origin=%q text=%q", origin, text)
	}
	var indexed int
	err = db.db.QueryRow(`SELECT COUNT(*) FROM indexed_files WHERE path = '/sessions/second.jsonl'`).Scan(&indexed)
	if err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if indexed != 0 {
		t.Fatalf("conflicting file was indexed: %d rows", indexed)
	}
}

func assertOriginRow(t *testing.T, db *Database, uuid string, wantID int64, wantOrigin models.MessageOrigin, wantText string, wantExtractionVersion int) {
	t.Helper()
	var id int64
	var origin models.MessageOrigin
	var text string
	var originVersion, extractionVersion int
	if err := db.db.QueryRow(`
		SELECT id, origin, text, origin_version, extraction_version
		FROM search_items WHERE uuid = ?
	`, uuid).Scan(&id, &origin, &text, &originVersion, &extractionVersion); err != nil {
		t.Fatal(err)
	}
	if id != wantID || origin != wantOrigin || text != wantText || originVersion != CurrentOriginVersion || extractionVersion != wantExtractionVersion {
		t.Fatalf("row = (id=%d origin=%q text=%q origin_version=%d extraction_version=%d), want (%d, %q, %q, %d, %d)",
			id, origin, text, originVersion, extractionVersion,
			wantID, wantOrigin, wantText, CurrentOriginVersion, wantExtractionVersion)
	}
}
