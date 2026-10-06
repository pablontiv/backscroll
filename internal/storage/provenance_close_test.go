package storage

import (
	"database/sql"
	"path/filepath"
	"reflect"
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
