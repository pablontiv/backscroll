package storage

import (
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
	for _, kind := range []string{"duplicate", "satellite", "negative"} {
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
			case "duplicate":
				_, err = db.db.Exec(`UPDATE search_items SET ordinal=0 WHERE uuid='b'`)
			case "satellite":
				_, err = db.db.Exec(`UPDATE tool_events SET ordinal=5 WHERE message_uuid='a'`)
			case "negative":
				file.Messages[0].Ordinal = -1
			}
			if err != nil {
				t.Fatal(err)
			}
			file.Hash = "invalid"
			if err := db.SyncFiles([]IndexedFile{file}); err == nil || !strings.Contains(err.Error(), "ordinal") {
				t.Fatalf("expected explicit ordinal error, got %v", err)
			}
			var hash string
			if err := db.db.QueryRow(`SELECT hash FROM indexed_files WHERE path='/session'`).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			if hash != "first" {
				t.Fatal("invalid batch committed")
			}
		})
	}
}
