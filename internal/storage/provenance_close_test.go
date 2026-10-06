package storage

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestPerennialProvenanceClosureIsMonotonicAndDrainsEchoQueue(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "closure.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/sessions/closure.jsonl"
	file := IndexedFile{SourcePath: path, Source: "session", Hash: "h1", Messages: []IndexedMessage{
		{Ordinal: 0, UUID: "closure-human", Role: "user", Origin: models.OriginHuman, Text: "human payload", ContentType: "text"},
		{Ordinal: 1, UUID: "closure-bash", Role: "assistant", Origin: models.OriginAssistant, Text: "Bash command=backscroll search --text orchard", ContentType: "tool"},
		{Ordinal: 2, UUID: "closure-shell", Role: "assistant", Origin: models.OriginAssistant, Text: `shell command=["/bin/bash","-lc","backscroll search --text orchard"]`, ContentType: "tool"},
		{Ordinal: 3, UUID: "closure-proven", Role: "assistant", Origin: models.OriginAssistant, Text: "exec_command cmd=backscroll search --text orchard", ContentType: "tool", SearchEcho: true},
		{Ordinal: 4, UUID: "closure-ordinary", Role: "assistant", Origin: models.OriginAssistant, Text: "Bash command=echo backscroll search", ContentType: "tool"},
	}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}

	if _, err := db.db.Exec(`UPDATE search_items SET origin_version = 0 WHERE uuid = 'closure-human'`); err != nil {
		t.Fatal(err)
	}
	for _, update := range []struct {
		uuid  string
		value any
	}{
		{uuid: "closure-bash", value: nil},
		{uuid: "closure-shell", value: 0},
		{uuid: "closure-proven", value: 1},
		{uuid: "closure-ordinary", value: nil},
	} {
		if _, err := db.db.Exec(`UPDATE search_items SET search_echo = ? WHERE uuid = ?`, update.value, update.uuid); err != nil {
			t.Fatal(err)
		}
	}

	file.Hash = "h2"
	file.Messages = nil
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("close omitted provenance: %v", err)
	}

	var origin models.MessageOrigin
	var originVersion int
	if err := db.db.QueryRow(`SELECT origin, origin_version FROM search_items WHERE uuid = 'closure-human'`).Scan(&origin, &originVersion); err != nil {
		t.Fatal(err)
	}
	if origin != models.OriginHuman || originVersion != CurrentOriginVersion {
		t.Fatalf("omitted human provenance = (%q, %d), want (%q, %d)", origin, originVersion, models.OriginHuman, CurrentOriginVersion)
	}

	for _, want := range []struct {
		uuid string
		echo int
	}{
		{uuid: "closure-bash", echo: 1},
		{uuid: "closure-shell", echo: 1},
		{uuid: "closure-proven", echo: 1},
		{uuid: "closure-ordinary", echo: 0},
	} {
		var echo int
		if err := db.db.QueryRow(`SELECT search_echo FROM search_items WHERE uuid = ?`, want.uuid).Scan(&echo); err != nil {
			t.Fatal(err)
		}
		if echo != want.echo {
			t.Errorf("%s search_echo = %d, want %d", want.uuid, echo, want.echo)
		}
	}
	if pending, err := db.PendingSearchEchoPaths(); err != nil || len(pending) != 0 {
		t.Fatalf("echo queue after closure = %v, err=%v; want empty", pending, err)
	}
}

func TestPerennialProvenanceClosureClassifiesRetainedPayloadAfterEchoDrift(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "echo-drift.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/sessions/echo-drift.jsonl"
	file := IndexedFile{SourcePath: path, Source: "session", Hash: "h1", Messages: []IndexedMessage{
		{Ordinal: 0, UUID: "drift-direct-zero", Role: "assistant", Text: "Bash command=backscroll search --text retained-zero", ContentType: "tool", ExtractionVersion: 1},
		{Ordinal: 1, UUID: "drift-direct-null", Role: "assistant", Text: `shell command=["/bin/bash","-lc","backscroll search --text retained-null"]`, ContentType: "tool", ExtractionVersion: 1},
		{Ordinal: 2, UUID: "drift-nondirect", Role: "assistant", Text: "Bash command=echo backscroll search", ContentType: "tool", ExtractionVersion: 1},
		{Ordinal: 3, UUID: "drift-positive", Role: "assistant", Text: "retained paired result", ContentType: "tool", ExtractionVersion: 1, SearchEcho: true},
	}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET search_echo = NULL WHERE uuid IN ('drift-direct-null', 'drift-nondirect')`); err != nil {
		t.Fatal(err)
	}

	type retainedRow struct {
		id                int64
		text              string
		contentType       string
		extractionVersion int
		searchEcho        int
		origin            models.MessageOrigin
		originVersion     int
	}
	readRow := func(uuid string) retainedRow {
		t.Helper()
		var row retainedRow
		if err := db.db.QueryRow(`
			SELECT id, text, content_type, extraction_version,
			       COALESCE(search_echo, -1), origin, origin_version
			FROM search_items WHERE uuid = ?
		`, uuid).Scan(
			&row.id, &row.text, &row.contentType, &row.extractionVersion,
			&row.searchEcho, &row.origin, &row.originVersion,
		); err != nil {
			t.Fatal(err)
		}
		return row
	}

	before := make(map[string]retainedRow, len(file.Messages))
	for _, message := range file.Messages {
		before[message.UUID] = readRow(message.UUID)
	}
	if pending, err := db.PendingSearchEchoPaths(); err != nil || !reflect.DeepEqual(pending, []string{path}) {
		t.Fatalf("echo queue before drift replay = %v, err=%v; want [%s]", pending, err, path)
	}

	file.Hash = "h2"
	file.Messages = []IndexedMessage{
		{Ordinal: 0, UUID: "drift-direct-zero", Role: "assistant", Origin: models.OriginAssistant, Text: "rewritten parser payload zero", ContentType: "tool", ExtractionVersion: CurrentExtractionVersion, SearchEcho: true},
		{Ordinal: 1, UUID: "drift-direct-null", Role: "assistant", Origin: models.OriginAssistant, Text: "rewritten parser payload null", ContentType: "tool", ExtractionVersion: CurrentExtractionVersion, SearchEcho: true},
		{Ordinal: 2, UUID: "drift-nondirect", Role: "assistant", Origin: models.OriginAssistant, Text: "rewritten parser payload non-direct", ContentType: "tool", ExtractionVersion: CurrentExtractionVersion, SearchEcho: true},
		{Ordinal: 3, UUID: "drift-positive", Role: "assistant", Origin: models.OriginAssistant, Text: "rewritten parser payload positive", ContentType: "tool", ExtractionVersion: CurrentExtractionVersion},
	}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("first drift replay: %v", err)
	}

	wantEcho := map[string]int{
		"drift-direct-zero": 1,
		"drift-direct-null": 1,
		"drift-nondirect":   0,
		"drift-positive":    1,
	}
	assertConverged := func(stage string) {
		t.Helper()
		for uuid, want := range wantEcho {
			got := readRow(uuid)
			original := before[uuid]
			if got.id != original.id || got.text != original.text || got.contentType != original.contentType || got.extractionVersion != original.extractionVersion {
				t.Errorf("%s changed retained payload for %s: got=%+v before=%+v", stage, uuid, got, original)
			}
			if got.searchEcho != want {
				t.Errorf("%s search_echo for %s = %d, want %d", stage, uuid, got.searchEcho, want)
			}
			if got.origin != models.OriginAssistant || got.originVersion != CurrentOriginVersion {
				t.Errorf("%s origin for %s = (%q,%d), want (%q,%d)", stage, uuid, got.origin, got.originVersion, models.OriginAssistant, CurrentOriginVersion)
			}
		}
		if pending, err := db.PendingSearchEchoPaths(); err != nil || len(pending) != 0 {
			t.Errorf("%s echo queue = %v, err=%v; want empty", stage, pending, err)
		}
	}
	assertConverged("first replay")

	file.Hash = "h3"
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("second drift replay: %v", err)
	}
	assertConverged("second replay")
}

func TestIdentifiedEmptyAndMixedReplaysPreserveUUIDNullHistory(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/sessions/mixed-history.jsonl"
	anchor := IndexedMessage{
		Ordinal: 0, UUID: "history-anchor", Role: "assistant", Origin: models.OriginAssistant,
		Text: "anchor payload", ContentType: "tool", ToolName: "Read", CommandHead: "cat", ExtractionVersion: 1,
	}
	file := IndexedFile{SourcePath: path, Source: "session", Hash: "h1", Messages: []IndexedMessage{anchor}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO search_items
			(source, source_path, ordinal, role, origin, text, uuid, content_type, extraction_version, search_echo, origin_version)
		VALUES ('session', ?, 7, 'assistant', 'automation', 'legacy payload', NULL, 'tool', 0, NULL, 0)
	`, path); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO tool_events
			(message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version)
		VALUES (NULL, ?, 7, 'LegacyTool', 'legacy-head', 1, 23, 0)
	`, path); err != nil {
		t.Fatal(err)
	}

	type itemSnapshot struct {
		id          int64
		uuid        sql.NullString
		ordinal     int
		role        string
		origin      string
		text        string
		contentType string
	}
	type eventSnapshot struct {
		id          int64
		messageUUID sql.NullString
		ordinal     int
		toolName    string
		commandHead string
		isError     sql.NullBool
		exitCode    sql.NullInt64
	}
	readHistory := func() ([]itemSnapshot, []eventSnapshot) {
		t.Helper()
		itemRows, err := db.db.Query(`
			SELECT id, uuid, ordinal, role, origin, text, content_type
			FROM search_items WHERE source_path = ? ORDER BY id
		`, path)
		if err != nil {
			t.Fatal(err)
		}
		var items []itemSnapshot
		for itemRows.Next() {
			var item itemSnapshot
			if err := itemRows.Scan(&item.id, &item.uuid, &item.ordinal, &item.role, &item.origin, &item.text, &item.contentType); err != nil {
				t.Fatal(err)
			}
			items = append(items, item)
		}
		if err := itemRows.Err(); err != nil {
			_ = itemRows.Close()
			t.Fatal(err)
		}
		if err := itemRows.Close(); err != nil {
			t.Fatal(err)
		}

		eventRows, err := db.db.Query(`
			SELECT id, message_uuid, ordinal, tool_name, command_head, is_error, exit_code
			FROM tool_events WHERE source_path = ? ORDER BY id
		`, path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = eventRows.Close() }()
		var events []eventSnapshot
		for eventRows.Next() {
			var event eventSnapshot
			if err := eventRows.Scan(&event.id, &event.messageUUID, &event.ordinal, &event.toolName, &event.commandHead, &event.isError, &event.exitCode); err != nil {
				t.Fatal(err)
			}
			events = append(events, event)
		}
		if err := eventRows.Err(); err != nil {
			t.Fatal(err)
		}
		return items, events
	}

	wantItems, wantEvents := readHistory()
	file.Hash = "h2"
	file.Messages = nil
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("empty replay: %v", err)
	}
	if gotItems, gotEvents := readHistory(); !reflect.DeepEqual(gotItems, wantItems) || !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("empty replay changed history:\nitems got=%+v want=%+v\nevents got=%+v want=%+v", gotItems, wantItems, gotEvents, wantEvents)
	}

	file.Hash = "h3"
	file.Messages = []IndexedMessage{
		anchor,
		{Ordinal: 8, Role: "user", Origin: models.OriginHuman, Text: "temporary UUID-less parse", ContentType: "text"},
	}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("mixed replay: %v", err)
	}
	if gotItems, gotEvents := readHistory(); !reflect.DeepEqual(gotItems, wantItems) || !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("mixed replay changed history:\nitems got=%+v want=%+v\nevents got=%+v want=%+v", gotItems, wantItems, gotEvents, wantEvents)
	}
}

func TestPureLegacyTransitionCrossPathUUIDRollsBackWholeBatch(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "legacy-cross-path.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const (
		legacyPath   = "/sessions/legacy-cross-path.jsonl"
		externalPath = "/sessions/external-owner.jsonl"
		conflictUUID = "legacy-external-conflict"
		freeUUID     = "legacy-free-replacement"
	)
	if err := db.SyncFiles([]IndexedFile{{
		SourcePath: externalPath, Source: "session", Hash: "external-hash", Messages: []IndexedMessage{{
			Ordinal: 0, UUID: conflictUUID, Role: "assistant", Origin: models.OriginUnknown,
			Text: "foreign payload", ContentType: "tool", ExtractionVersion: 1,
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SyncFiles([]IndexedFile{{
		SourcePath: legacyPath, Source: "session", Hash: "legacy-hash", Tags: []string{"old-tag"}, Messages: []IndexedMessage{{
			Ordinal: 0, Role: "assistant", Origin: models.OriginUnknown, Text: "legacy payload",
			ContentType: "tool", ToolName: "LegacyTool", CommandHead: "old", ExtractionVersion: 0,
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET origin_version = NULL, search_echo = NULL WHERE source_path = ?`, legacyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET origin_version = 0 WHERE uuid = ?`, conflictUUID); err != nil {
		t.Fatal(err)
	}

	err = db.SyncFiles([]IndexedFile{{
		SourcePath: legacyPath, Source: "session", Hash: "replacement-hash", Tags: []string{"new-tag"}, Messages: []IndexedMessage{
			{
				Ordinal: 0, UUID: conflictUUID, Role: "assistant", Origin: models.OriginAssistant,
				Text: "conflicting replacement", ContentType: "tool", ToolName: "Bash", CommandHead: "crossed", SearchEcho: true,
			},
			{
				Ordinal: 1, UUID: freeUUID, Role: "assistant", Origin: models.OriginAssistant,
				Text: "free replacement", ContentType: "tool", ToolName: "Read", CommandHead: "free",
			},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), conflictUUID) || !strings.Contains(err.Error(), externalPath) {
		t.Fatalf("cross-path transition error = %v, want UUID ownership error", err)
	}

	var legacyRows, freeRows, crossedEvents int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ? AND uuid IS NULL AND text = 'legacy payload'`, legacyPath).Scan(&legacyRows); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE uuid = ?`, freeUUID).Scan(&freeRows); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM tool_events WHERE source_path = ? AND message_uuid IN (?, ?)`, legacyPath, conflictUUID, freeUUID).Scan(&crossedEvents); err != nil {
		t.Fatal(err)
	}
	var legacyOriginVersion sql.NullInt64
	var legacyEcho sql.NullBool
	if err := db.db.QueryRow(`SELECT origin_version, search_echo FROM search_items WHERE source_path = ? AND uuid IS NULL`, legacyPath).Scan(&legacyOriginVersion, &legacyEcho); err != nil {
		t.Fatal(err)
	}
	var foreignOrigin models.MessageOrigin
	var foreignOriginVersion int
	if err := db.db.QueryRow(`SELECT origin, origin_version FROM search_items WHERE uuid = ?`, conflictUUID).Scan(&foreignOrigin, &foreignOriginVersion); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := db.db.QueryRow(`SELECT hash FROM indexed_files WHERE path = ?`, legacyPath).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	var oldTags, newTags int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM session_tags WHERE source_path = ? AND tag = 'old-tag'`, legacyPath).Scan(&oldTags); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM session_tags WHERE source_path = ? AND tag = 'new-tag'`, legacyPath).Scan(&newTags); err != nil {
		t.Fatal(err)
	}
	if legacyRows != 1 || freeRows != 0 || crossedEvents != 0 || legacyOriginVersion.Valid || legacyEcho.Valid ||
		foreignOrigin != models.OriginUnknown || foreignOriginVersion != 0 || hash != "legacy-hash" || oldTags != 1 || newTags != 0 {
		t.Fatalf("failed transition mutated state: legacy=%d free=%d events=%d legacyVersion=%+v legacyEcho=%+v foreign=(%q,%d) hash=%q tags=(%d,%d)",
			legacyRows, freeRows, crossedEvents, legacyOriginVersion, legacyEcho, foreignOrigin, foreignOriginVersion, hash, oldTags, newTags)
	}
}

func TestIdentifiedPathCrossPathUUIDDoesNotEnrichOrCrossToolEvent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "identified-cross-path.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const (
		identifiedPath = "/sessions/identified-owner.jsonl"
		externalPath   = "/sessions/identified-external.jsonl"
		anchorUUID     = "identified-anchor"
		conflictUUID   = "identified-external-conflict"
	)
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: identifiedPath, Source: "session", Hash: "identified-hash", Tags: []string{"old-tag"}, Messages: []IndexedMessage{{
				Ordinal: 0, UUID: anchorUUID, Role: "user", Origin: models.OriginHuman,
				Text: "anchor payload", ContentType: "text", ExtractionVersion: 1,
			}},
		},
		{
			SourcePath: externalPath, Source: "session", Hash: "external-hash", Messages: []IndexedMessage{{
				Ordinal: 0, UUID: conflictUUID, Role: "assistant", Origin: models.OriginUnknown,
				Text: "foreign payload", ContentType: "tool", ExtractionVersion: 1,
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET origin_version = 0, search_echo = NULL WHERE uuid = ?`, anchorUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET origin_version = 0 WHERE uuid = ?`, conflictUUID); err != nil {
		t.Fatal(err)
	}

	err = db.SyncFiles([]IndexedFile{{
		SourcePath: identifiedPath, Source: "session", Hash: "changed-hash", Tags: []string{"new-tag"}, Messages: []IndexedMessage{
			{Ordinal: 0, UUID: anchorUUID, Role: "user", Origin: models.OriginHuman, Text: "anchor payload", ContentType: "text"},
			{
				Ordinal: 1, UUID: conflictUUID, Role: "assistant", Origin: models.OriginAssistant,
				Text: "foreign replay", ContentType: "tool", ToolName: "Bash", CommandHead: "crossed", SearchEcho: true,
			},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), conflictUUID) || !strings.Contains(err.Error(), externalPath) {
		t.Fatalf("identified cross-path error = %v, want UUID ownership error", err)
	}

	var anchorVersion int
	var anchorEcho sql.NullBool
	if err := db.db.QueryRow(`SELECT origin_version, search_echo FROM search_items WHERE uuid = ?`, anchorUUID).Scan(&anchorVersion, &anchorEcho); err != nil {
		t.Fatal(err)
	}
	var foreignOrigin models.MessageOrigin
	var foreignVersion, crossedEvents int
	if err := db.db.QueryRow(`SELECT origin, origin_version FROM search_items WHERE uuid = ?`, conflictUUID).Scan(&foreignOrigin, &foreignVersion); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM tool_events WHERE source_path = ? AND message_uuid = ?`, identifiedPath, conflictUUID).Scan(&crossedEvents); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := db.db.QueryRow(`SELECT hash FROM indexed_files WHERE path = ?`, identifiedPath).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	var oldTags, newTags int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM session_tags WHERE source_path = ? AND tag = 'old-tag'`, identifiedPath).Scan(&oldTags); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM session_tags WHERE source_path = ? AND tag = 'new-tag'`, identifiedPath).Scan(&newTags); err != nil {
		t.Fatal(err)
	}
	if anchorVersion != 0 || anchorEcho.Valid || foreignOrigin != models.OriginUnknown || foreignVersion != 0 ||
		crossedEvents != 0 || hash != "identified-hash" || oldTags != 1 || newTags != 0 {
		t.Fatalf("failed identified replay mutated state: anchor=(%d,%+v) foreign=(%q,%d) events=%d hash=%q tags=(%d,%d)",
			anchorVersion, anchorEcho, foreignOrigin, foreignVersion, crossedEvents, hash, oldTags, newTags)
	}
}

func TestNonTransitionSyncRejectsDuplicateUUIDs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "non-transition-duplicate.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const duplicateUUID = "plan-duplicate"
	err = db.SyncFiles([]IndexedFile{{
		SourcePath: "/plans/duplicate.md", Source: "plan", Hash: "plan-hash", Messages: []IndexedMessage{
			{Ordinal: 0, UUID: duplicateUUID, Role: "user", Text: "first", ContentType: "text"},
			{Ordinal: 1, UUID: duplicateUUID, Role: "assistant", Text: "second", ContentType: "text", ToolName: "Read"},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), `duplicate UUID "plan-duplicate"`) {
		t.Fatalf("non-transition duplicate error = %v, want duplicate UUID error", err)
	}

	var items, events, indexed int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE uuid = ?`, duplicateUUID).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM tool_events WHERE message_uuid = ?`, duplicateUUID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM indexed_files WHERE path = '/plans/duplicate.md'`).Scan(&indexed); err != nil {
		t.Fatal(err)
	}
	if items != 0 || events != 0 || indexed != 0 {
		t.Fatalf("duplicate non-transition sync mutated state: items=%d events=%d indexed=%d", items, events, indexed)
	}
}

func TestPureLegacyTransitionRejectsDuplicateUUIDsWithoutChanges(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "duplicate-transition.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/sessions/duplicate-transition.jsonl"
	legacy := IndexedFile{SourcePath: path, Source: "session", Hash: "legacy-hash", Messages: []IndexedMessage{
		{
			Ordinal: 0, Role: "assistant", Origin: models.OriginAutomation, Text: "legacy first",
			ContentType: "tool", ToolName: "LegacyRead", CommandHead: "old-read", ExtractionVersion: 0,
		},
		{
			Ordinal: 1, Role: "assistant", Origin: models.OriginAssistant, Text: "legacy second",
			ContentType: "tool", ToolName: "LegacyBash", CommandHead: "old-bash", ExtractionVersion: 1,
		},
	}}
	if err := db.SyncFiles([]IndexedFile{legacy}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`
		UPDATE search_items
		SET origin_version = CASE ordinal WHEN 0 THEN NULL ELSE 1 END,
		    search_echo = CASE ordinal WHEN 0 THEN NULL ELSE 1 END
		WHERE source_path = ?
	`, path); err != nil {
		t.Fatal(err)
	}

	type itemSnapshot struct {
		id                int64
		ordinal           int
		role              string
		origin            models.MessageOrigin
		text              string
		uuid              sql.NullString
		contentType       string
		extractionVersion sql.NullInt64
		searchEcho        sql.NullBool
		originVersion     sql.NullInt64
	}
	type eventSnapshot struct {
		id                int64
		messageUUID       sql.NullString
		ordinal           int
		toolName          string
		commandHead       string
		isError           sql.NullBool
		exitCode          sql.NullInt64
		extractionVersion sql.NullInt64
	}
	readState := func() ([]itemSnapshot, []eventSnapshot, string) {
		t.Helper()
		itemRows, err := db.db.Query(`
			SELECT id, ordinal, role, origin, text, uuid, content_type,
			       extraction_version, search_echo, origin_version
			FROM search_items WHERE source_path = ? ORDER BY id
		`, path)
		if err != nil {
			t.Fatal(err)
		}
		var items []itemSnapshot
		for itemRows.Next() {
			var item itemSnapshot
			if err := itemRows.Scan(
				&item.id, &item.ordinal, &item.role, &item.origin, &item.text,
				&item.uuid, &item.contentType, &item.extractionVersion,
				&item.searchEcho, &item.originVersion,
			); err != nil {
				_ = itemRows.Close()
				t.Fatal(err)
			}
			items = append(items, item)
		}
		if err := itemRows.Err(); err != nil {
			_ = itemRows.Close()
			t.Fatal(err)
		}
		if err := itemRows.Close(); err != nil {
			t.Fatal(err)
		}

		eventRows, err := db.db.Query(`
			SELECT id, message_uuid, ordinal, tool_name, command_head,
			       is_error, exit_code, extraction_version
			FROM tool_events WHERE source_path = ? ORDER BY id
		`, path)
		if err != nil {
			t.Fatal(err)
		}
		var events []eventSnapshot
		for eventRows.Next() {
			var event eventSnapshot
			if err := eventRows.Scan(
				&event.id, &event.messageUUID, &event.ordinal, &event.toolName,
				&event.commandHead, &event.isError, &event.exitCode, &event.extractionVersion,
			); err != nil {
				_ = eventRows.Close()
				t.Fatal(err)
			}
			events = append(events, event)
		}
		if err := eventRows.Err(); err != nil {
			_ = eventRows.Close()
			t.Fatal(err)
		}
		if err := eventRows.Close(); err != nil {
			t.Fatal(err)
		}

		var hash string
		if err := db.db.QueryRow(`SELECT hash FROM indexed_files WHERE path = ?`, path).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		return items, events, hash
	}

	wantItems, wantEvents, wantHash := readState()
	duplicate := IndexedFile{SourcePath: path, Source: "session", Hash: "replacement-hash", Messages: []IndexedMessage{
		{
			Ordinal: 0, UUID: "duplicate-transition-uuid", Role: "assistant", Origin: models.OriginAssistant,
			Text: "replacement first", ContentType: "tool", ToolName: "Read", CommandHead: "new-read", ExtractionVersion: CurrentExtractionVersion,
		},
		{
			Ordinal: 1, UUID: "duplicate-transition-uuid", Role: "assistant", Origin: models.OriginAssistant,
			Text: "replacement second", ContentType: "tool", ToolName: "Bash", CommandHead: "new-bash", ExtractionVersion: CurrentExtractionVersion,
		},
	}}
	err = db.SyncFiles([]IndexedFile{duplicate})
	if err == nil || !strings.Contains(err.Error(), `duplicate UUID "duplicate-transition-uuid"`) {
		t.Fatalf("duplicate transition error = %v, want duplicate UUID error", err)
	}

	gotItems, gotEvents, gotHash := readState()
	if !reflect.DeepEqual(gotItems, wantItems) {
		t.Fatalf("duplicate transition changed search history:\ngot=%+v\nwant=%+v", gotItems, wantItems)
	}
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("duplicate transition changed tool history:\ngot=%+v\nwant=%+v", gotEvents, wantEvents)
	}
	if gotHash != wantHash || gotHash != "legacy-hash" {
		t.Fatalf("duplicate transition changed hash: got %q, want %q", gotHash, wantHash)
	}
}

func TestPureLegacyToCompleteUUIDTransitionStillCleansLegacyHistory(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "transition.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	const path = "/sessions/legacy-transition.jsonl"
	file := IndexedFile{SourcePath: path, Source: "session", Hash: "h1", Messages: []IndexedMessage{{
		Ordinal: 0, Role: "assistant", Origin: models.OriginAutomation, Text: "legacy payload",
		ContentType: "tool", ToolName: "LegacyTool", CommandHead: "legacy", ExtractionVersion: 0,
	}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}

	file.Hash = "h2"
	file.Messages = []IndexedMessage{{
		Ordinal: 0, UUID: "transition-uuid", Role: "assistant", Origin: models.OriginAutomation, Text: "replacement payload",
		ContentType: "tool", ToolName: "CurrentTool", CommandHead: "current", ExtractionVersion: CurrentExtractionVersion,
	}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatalf("legacy transition: %v", err)
	}

	var nullItems, nullEvents, replacements int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ? AND uuid IS NULL`, path).Scan(&nullItems); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM tool_events WHERE source_path = ? AND message_uuid IS NULL`, path).Scan(&nullEvents); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`
		SELECT COUNT(*) FROM search_items si
		JOIN tool_events te ON te.message_uuid = si.uuid
		WHERE si.source_path = ? AND si.uuid = 'transition-uuid'
		  AND si.text = 'replacement payload' AND te.tool_name = 'CurrentTool'
	`, path).Scan(&replacements); err != nil {
		t.Fatal(err)
	}
	if nullItems != 0 || nullEvents != 0 || replacements != 1 {
		t.Fatalf("transition state: null items=%d null events=%d replacements=%d; want 0, 0, 1", nullItems, nullEvents, replacements)
	}
}
