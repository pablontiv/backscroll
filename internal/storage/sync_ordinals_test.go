package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestPerennialOrdinalReservations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ordinals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := IndexedFile{Source: "session", SourcePath: "/session", Hash: "first", Messages: []IndexedMessage{
		{UUID: "omitted", Ordinal: 0, Text: "retained", ContentType: "text"},
		{UUID: "future", Ordinal: 2, Text: "original", ContentType: "tool", ToolName: "Bash", CommandHead: "go", ExtractionVersion: 1},
	}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	var itemID, eventID int64
	if err := db.db.QueryRow(`SELECT s.id,e.id FROM search_items s JOIN tool_events e ON e.message_uuid=s.uuid WHERE s.uuid='future'`).Scan(&itemID, &eventID); err != nil {
		t.Fatal(err)
	}
	file.Hash = "second"
	file.Messages = []IndexedMessage{
		{UUID: "new", Ordinal: 2, Text: "new", ContentType: "tool", ToolName: "Bash", CommandHead: "make"},
		{UUID: "future", Ordinal: 1, Text: "changed", ContentType: "tool", ToolName: "Bash", CommandHead: "go"},
		{UUID: "free", Ordinal: 7, Text: "free", ContentType: "text"},
		{UUID: "collision", Ordinal: 2, Text: "collision", ContentType: "text"},
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := db.SyncFiles([]IndexedFile{file}); err != nil {
			t.Fatal(err)
		}
		for uuid, want := range map[string]int{"omitted": 0, "new": 1, "future": 2, "collision": 3, "free": 7} {
			var ordinal int
			if err := db.db.QueryRow(`SELECT ordinal FROM search_items WHERE uuid=?`, uuid).Scan(&ordinal); err != nil {
				t.Fatal(err)
			}
			if ordinal != want {
				t.Fatalf("%s ordinal=%d want %d", uuid, ordinal, want)
			}
		}
	}
	var afterItem, afterEvent int64
	var text string
	var epoch, ordinal int
	if err := db.db.QueryRow(`SELECT s.id,e.id,s.text,s.extraction_version,e.ordinal FROM search_items s JOIN tool_events e ON e.message_uuid=s.uuid WHERE s.uuid='future'`).Scan(&afterItem, &afterEvent, &text, &epoch, &ordinal); err != nil {
		t.Fatal(err)
	}
	if itemID != afterItem || eventID != afterEvent || text != "original" || epoch != 1 || ordinal != 2 {
		t.Fatal("retained anchors or payload changed")
	}
	var newOrdinal int
	if err := db.db.QueryRow(`SELECT ordinal FROM tool_events WHERE message_uuid='new'`).Scan(&newOrdinal); err != nil {
		t.Fatal(err)
	}
	if newOrdinal != 1 {
		t.Fatalf("new satellite ordinal=%d", newOrdinal)
	}
	if file.Messages[0].Ordinal != 2 {
		t.Fatal("mutated caller slice")
	}
}

func TestPerennialRejectsInvalidStoredCoordinates(t *testing.T) {
	for _, kind := range []string{"coordinate-conflict", "satellite", "negative", "negative-persisted"} {
		t.Run(kind, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "invalid.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			file := IndexedFile{Source: "session", SourcePath: "/session", Hash: "first", Messages: []IndexedMessage{{UUID: "a", Ordinal: 0, Text: "a", ContentType: "tool", ToolName: "Bash"}, {UUID: "b", Ordinal: 1, Text: "b", ContentType: "text"}}}
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "coordinate-conflict":
				_, err = db.db.Exec(`UPDATE search_items SET ordinal=0 WHERE uuid='b'`)
				file.Messages[1].ToolName = "Read"
			case "negative-persisted":
				_, err = db.db.Exec(`UPDATE search_items SET ordinal=-1 WHERE uuid='b'`)
			case "satellite":
				_, err = db.db.Exec(`UPDATE tool_events SET ordinal=5 WHERE message_uuid='a'`)
			case "negative":
				file.Messages[0].Ordinal = -1
			}
			if err != nil {
				t.Fatal(err)
			}
			file.Hash = "invalid"
			prefix := IndexedFile{Source: "session", SourcePath: "/prefix", Hash: "new", Messages: []IndexedMessage{{UUID: "prefix", Ordinal: 0, Text: "rollback me", ContentType: "text"}}}
			if err := db.SyncFiles([]IndexedFile{prefix, file}); err == nil || !strings.Contains(err.Error(), "ordinal") {
				t.Fatalf("expected explicit ordinal error, got %v", err)
			}
			var hash string
			if err := db.db.QueryRow(`SELECT hash FROM indexed_files WHERE path='/session'`).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path='/prefix'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("earlier file survived failed transaction")
			}
			if err := db.db.QueryRow(`SELECT COUNT(*) FROM indexed_files WHERE path='/prefix'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("earlier file metadata survived failed transaction")
			}
			if hash != "first" {
				t.Fatal("invalid batch committed")
			}
		})
	}
}

// Removing NULL-identity tolerance or satellite enrichment breaks this native
// lossy-backfill -> source-reappearance path, not merely a synthetic SQL shape.
func TestPerennialClaimsLossyBackfillSatellite(t *testing.T) {
	for _, kind := range []string{"unknown-signals", "known-signals", "empty-identity"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			db, err := Open(filepath.Join(t.TempDir(), "lossy.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			file := IndexedFile{Source: "session", SourcePath: "/lossy", Hash: "first", Messages: []IndexedMessage{{UUID: "call", Ordinal: 2, Text: "Bash command=go test ./...", ContentType: "tool"}}}
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			tx, err := db.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			count, err := db.backfillToolEventsForFileContext(context.Background(), tx, file.SourcePath, file.Messages)
			if err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			if count != 1 {
				tx.Rollback()
				t.Fatalf("backfill count=%d", count)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			var beforeID int64
			var identity sql.NullString
			if err := db.db.QueryRow(`SELECT id,message_uuid FROM tool_events WHERE source_path='/lossy'`).Scan(&beforeID, &identity); err != nil {
				t.Fatal(err)
			}
			if identity.Valid {
				t.Fatal("backfill unexpectedly proved UUID")
			}
			if kind == "known-signals" {
				if _, err := db.db.Exec(`UPDATE tool_events SET is_error=0,exit_code=0,extraction_version=9 WHERE source_path='/lossy'`); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "empty-identity" {
				if _, err := db.db.Exec(`UPDATE tool_events SET message_uuid='',tool_name='',command_head='' WHERE source_path='/lossy'`); err != nil {
					t.Fatal(err)
				}
			}
			// An unpaired lossy satellite still reserves its coordinate.
			if _, err := db.db.Exec(`INSERT INTO tool_events(source_path,ordinal,tool_name,command_head,extraction_version) VALUES('/lossy',0,'Read','',0)`); err != nil {
				t.Fatal(err)
			}
			incomingHead := "make"
			if kind == "empty-identity" {
				incomingHead = "go"
			}
			isError, exit := true, 7
			file.Hash = "returned"
			file.Messages = []IndexedMessage{
				{UUID: "new", Ordinal: 0, Text: "new message", ContentType: "text"},
				{UUID: "call", Ordinal: 0, Text: "different payload", ContentType: "tool", ToolName: "Bash", CommandHead: incomingHead, IsError: &isError, ExitCode: &exit, ExtractionVersion: 2},
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := db.SyncFiles([]IndexedFile{file}); err != nil {
					t.Fatal(err)
				}
			}
			var id int64
			var uuid, tool, head, text string
			var ordinal, errorSignal, exitCode, epoch int
			if err := db.db.QueryRow(`SELECT id,message_uuid,ordinal,tool_name,command_head,is_error,exit_code,extraction_version FROM tool_events WHERE message_uuid='call'`).Scan(&id, &uuid, &ordinal, &tool, &head, &errorSignal, &exitCode, &epoch); err != nil {
				t.Fatal(err)
			}
			wantError, wantExit, wantEpoch := 1, 7, 2
			if kind == "known-signals" {
				wantError, wantExit, wantEpoch = 0, 0, 9
			}
			if id != beforeID || uuid != "call" || ordinal != 2 || tool != "Bash" || head != "go" || errorSignal != wantError || exitCode != wantExit || epoch != wantEpoch {
				t.Fatalf("claimed satellite: id=%d uuid=%s ordinal=%d tool=%s head=%s error=%d exit=%d epoch=%d", id, uuid, ordinal, tool, head, errorSignal, exitCode, epoch)
			}
			if err := db.db.QueryRow(`SELECT ordinal FROM search_items WHERE uuid='new'`).Scan(&ordinal); err != nil {
				t.Fatal(err)
			}
			if ordinal != 1 {
				t.Fatalf("new ordinal=%d, want 1", ordinal)
			}
			if err := db.db.QueryRow(`SELECT text FROM search_items WHERE uuid='call'`).Scan(&text); err != nil {
				t.Fatal(err)
			}
			if text != "Bash command=go test ./..." || file.Messages[1].Ordinal != 0 {
				t.Fatal("payload or caller slice mutated")
			}
			commands, err := db.AggregateCommands(AggregateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, command := range commands {
				if command.ToolName == "Bash" && command.CommandHead == "go" && command.Count == 1 {
					found = true
				}
			}
			if !found {
				t.Fatalf("claimed event unavailable to command consumer: %v", commands)
			}
			failures, err := db.AggregateFailures(AggregateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			found = false
			for _, failure := range failures {
				if failure.ToolName == "Bash" && failure.IsError == (wantError == 1) && failure.ExitCode != nil && *failure.ExitCode == wantExit {
					found = true
				}
			}
			if wantError == 1 && !found {
				t.Fatalf("claimed proof unavailable to failure consumer: %v", failures)
			}
			if wantError == 0 && len(failures) != 0 {
				t.Fatalf("known success became failure: %v", failures)
			}
		})
	}
}

func TestPerennialRetainsHistoricalDuplicateSearchOrdinals(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "duplicates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := IndexedFile{Source: "session", SourcePath: "/duplicates", Hash: "first", Messages: []IndexedMessage{
		{UUID: "a", Ordinal: 0, Text: "a", ContentType: "text"},
		{UUID: "b", Ordinal: 1, Text: "b", ContentType: "text"},
	}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET ordinal=0 WHERE uuid='b'`); err != nil {
		t.Fatal(err)
	}
	// UUID-less search rows also reserve coordinates.
	if _, err := db.db.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,content_type) VALUES('session','/duplicates',1,'user','legacy','text')`); err != nil {
		t.Fatal(err)
	}
	file.Hash = "second"
	file.Messages = []IndexedMessage{
		{UUID: "new", Ordinal: 0, Text: "new", ContentType: "tool", ToolName: "Bash", CommandHead: "go"},
		{UUID: "b", Ordinal: 7, Text: "changed b", ContentType: "text"},
		{UUID: "a", Ordinal: 8, Text: "changed a", ContentType: "text"},
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := db.SyncFiles([]IndexedFile{file}); err != nil {
			t.Fatal(err)
		}
		for uuid, want := range map[string]int{"a": 0, "b": 0, "new": 2} {
			var ordinal int
			if err := db.db.QueryRow(`SELECT ordinal FROM search_items WHERE uuid=?`, uuid).Scan(&ordinal); err != nil {
				t.Fatal(err)
			}
			if ordinal != want {
				t.Fatalf("%s ordinal=%d want %d", uuid, ordinal, want)
			}
		}
	}
	var ordinal int
	if err := db.db.QueryRow(`SELECT ordinal FROM tool_events WHERE message_uuid='new'`).Scan(&ordinal); err != nil {
		t.Fatal(err)
	}
	if ordinal != 2 || file.Messages[0].Ordinal != 0 {
		t.Fatal("satellite or caller ordinal incorrect")
	}
	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path='/duplicates' AND uuid IS NULL`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("unknown historical anchor discarded")
	}
}
