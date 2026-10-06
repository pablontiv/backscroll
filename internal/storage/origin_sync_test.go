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
	if pending, err := db.PendingOriginPaths(CurrentOriginVersion, 10); err != nil || len(pending) != 0 {
		t.Fatalf("origin queue after successful replay = %v, err=%v", pending, err)
	}
}

func TestOriginPerennialZeroMessageReplayPreservesRowsAndToolEvents(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "origin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/sessions/zero.jsonl"
	file := IndexedFile{SourcePath: path, Source: "session", Hash: "h1", Messages: []IndexedMessage{{
		Ordinal: 0, UUID: "zero-origin", Role: "assistant", Origin: models.OriginAssistant,
		Text: "retained zero payload", ContentType: "tool", ToolName: "Bash", CommandHead: "go test", ExtractionVersion: 1,
	}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}

	var itemID, eventID int64
	if err := db.db.QueryRow(`SELECT id FROM search_items WHERE uuid = 'zero-origin'`).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT id FROM tool_events WHERE message_uuid = 'zero-origin'`).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET origin = 'unknown', origin_version = ? WHERE uuid = 'zero-origin'`, CurrentOriginVersion-1); err != nil {
		t.Fatal(err)
	}
	if pending, err := db.PendingOriginPaths(CurrentOriginVersion, 10); err != nil || len(pending) != 1 || pending[0] != path {
		t.Fatalf("origin queue before zero-message replay = %v, err=%v; want [%s]", pending, err, path)
	}

	file.Hash = "h2"
	file.Messages = nil
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("zero-message replay: %v", err)
	}

	var gotItemID int64
	var gotUUID, gotText string
	var gotOrigin models.MessageOrigin
	var gotVersion int
	if err := db.db.QueryRow(`
		SELECT id, uuid, text, origin, origin_version FROM search_items WHERE uuid = 'zero-origin'
	`).Scan(&gotItemID, &gotUUID, &gotText, &gotOrigin, &gotVersion); err != nil {
		t.Fatal(err)
	}
	if gotItemID != itemID || gotUUID != "zero-origin" || gotText != "retained zero payload" || gotOrigin != models.OriginUnknown || gotVersion != CurrentOriginVersion {
		t.Fatalf("retained row = (%d, %q, %q, %q, %d), want (%d, zero-origin, retained zero payload, %q, %d)", gotItemID, gotUUID, gotText, gotOrigin, gotVersion, itemID, models.OriginUnknown, CurrentOriginVersion)
	}
	var gotEventID int64
	var toolName, commandHead string
	if err := db.db.QueryRow(`SELECT id, tool_name, command_head FROM tool_events WHERE message_uuid = 'zero-origin'`).Scan(&gotEventID, &toolName, &commandHead); err != nil {
		t.Fatal(err)
	}
	if gotEventID != eventID || toolName != "Bash" || commandHead != "go test" {
		t.Fatalf("retained tool event = (%d, %q, %q), want (%d, Bash, go test)", gotEventID, toolName, commandHead, eventID)
	}
	if pending, err := db.PendingOriginPaths(CurrentOriginVersion, 10); err != nil || len(pending) != 0 {
		t.Fatalf("origin queue after zero-message replay = %v, err=%v; want empty", pending, err)
	}
}

func TestOriginReplaySyncFailureDoesNotCloseOrDelete(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "origin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const replayPath = "/sessions/rollback.jsonl"
	replay := IndexedFile{SourcePath: replayPath, Source: "session", Hash: "h1", Messages: []IndexedMessage{{
		Ordinal: 0, UUID: "rollback-origin", Role: "assistant", Origin: models.OriginAssistant,
		Text: "must survive rollback", ContentType: "tool", ToolName: "Bash", ExtractionVersion: 1,
	}}}
	conflict := IndexedFile{SourcePath: "/sessions/conflict-existing.jsonl", Source: "session", Hash: "c1", Messages: []IndexedMessage{{
		Ordinal: 0, UUID: "conflicting-origin", Role: "user", Origin: models.OriginHuman, Text: "human proof", ContentType: "text",
	}}}
	if err := db.SyncFiles([]IndexedFile{replay, conflict}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET origin = 'unknown', origin_version = NULL, search_echo = NULL WHERE uuid = 'rollback-origin'`); err != nil {
		t.Fatal(err)
	}

	replay.Hash = "h2"
	replay.Messages = nil
	conflict.SourcePath = "/sessions/conflict-new.jsonl"
	conflict.Hash = "c2"
	conflict.Messages[0].Origin = models.OriginAssistant
	err = db.SyncFiles([]IndexedFile{replay, conflict})
	if err == nil || !strings.Contains(err.Error(), "conflicting proven origins") {
		t.Fatalf("replay sync error = %v, want conflicting origin", err)
	}

	var version sql.NullInt64
	var echo sql.NullBool
	var itemCount, eventCount int
	if err := db.db.QueryRow(`SELECT origin_version, search_echo FROM search_items WHERE uuid = 'rollback-origin'`).Scan(&version, &echo); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE uuid = 'rollback-origin' AND text = 'must survive rollback'`).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM tool_events WHERE message_uuid = 'rollback-origin'`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if version.Valid || echo.Valid || itemCount != 1 || eventCount != 1 {
		t.Fatalf("failed sync mutated replay history: version=%+v echo=%+v items=%d events=%d", version, echo, itemCount, eventCount)
	}
}

func TestOriginMutableMixedReplayWipesAndVersionsEveryRow(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "origin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/sessions/mutable-mixed.jsonl"
	file := IndexedFile{SourcePath: path, Source: "session", Hash: "h1", Messages: []IndexedMessage{{
		Ordinal: 0, Role: "user", Origin: models.OriginHuman, Text: "old mutable row", ContentType: "text",
	}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET origin_version = NULL WHERE source_path = ?`, path); err != nil {
		t.Fatal(err)
	}

	file.Hash = "h2"
	file.Messages = []IndexedMessage{
		{Ordinal: 0, Role: "user", Origin: models.OriginHuman, Text: "new UUID-less row", ContentType: "text"},
		{Ordinal: 1, UUID: "mixed-origin", Role: "assistant", Origin: models.OriginAssistant, Text: "new UUID row", ContentType: "text"},
	}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("mixed mutable replay: %v", err)
	}

	var total, stale, oldPayload int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ?`, path).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ? AND (origin_version IS NULL OR origin_version < ?)`, path, CurrentOriginVersion).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ? AND text = 'old mutable row'`, path).Scan(&oldPayload); err != nil {
		t.Fatal(err)
	}
	if total != 2 || stale != 0 || oldPayload != 0 {
		t.Fatalf("mixed wipe/reload = total %d stale %d old payload %d; want 2, 0, 0", total, stale, oldPayload)
	}
	if pending, err := db.PendingOriginPaths(CurrentOriginVersion, 10); err != nil || len(pending) != 0 {
		t.Fatalf("origin queue after mixed wipe/reload = %v, err=%v; want empty", pending, err)
	}
}

func TestOriginPerennialReplayWithUUIDLessSessionClosesRetainedRows(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "origin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/sessions/uuid-less.jsonl"
	file := IndexedFile{SourcePath: path, Source: "session", Hash: "h1", Messages: []IndexedMessage{
		{Ordinal: 0, UUID: "retained-origin", Role: "user", Origin: models.OriginHuman, Text: "retained", ContentType: "text"},
	}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	var retainedID int64
	if err := db.db.QueryRow(`SELECT id FROM search_items WHERE uuid = 'retained-origin'`).Scan(&retainedID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET origin = 'unknown', origin_version = NULL WHERE uuid = 'retained-origin'`); err != nil {
		t.Fatal(err)
	}

	file.Hash = "h2"
	file.Messages = []IndexedMessage{
		{Ordinal: 0, Role: "user", Origin: models.OriginHuman, Text: "parser omitted identity", ContentType: "text"},
	}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("replay UUID-less session: %v", err)
	}

	var gotID int64
	var origin models.MessageOrigin
	var version sql.NullInt64
	if err := db.db.QueryRow(`SELECT id, origin, origin_version FROM search_items WHERE uuid = 'retained-origin'`).Scan(&gotID, &origin, &version); err != nil {
		t.Fatal(err)
	}
	if gotID != retainedID || origin != models.OriginUnknown || !version.Valid || version.Int64 != CurrentOriginVersion {
		t.Fatalf("UUID-less replay retained row = (id=%d origin=%q version=%+v), want (%d, %q, %d)", gotID, origin, version, retainedID, models.OriginUnknown, CurrentOriginVersion)
	}
	if pending, err := db.PendingOriginPaths(CurrentOriginVersion, 10); err != nil || len(pending) != 0 {
		t.Fatalf("origin queue after UUID-less replay = %v, err=%v; want empty", pending, err)
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
