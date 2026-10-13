package storage

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

// Exercise the production storage API on a real temporary database, including
// native SQL NULLs that the string-valued ingestion API cannot express.
func TestCommandHeadNullStorageAPISmoke(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "commands.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	heads := []string{"", "", "go", "go", "git", "git", ""}
	messages := make([]IndexedMessage, len(heads))
	for i, head := range heads {
		timestamp := "2026-01-05T00:00:00Z"
		if i == 6 {
			timestamp = "2025-12-29T00:00:00Z"
		}
		messages[i] = IndexedMessage{Ordinal: i, UUID: fmt.Sprintf("u%d", i), Role: "assistant", Text: "tool output", ContentType: "tool", ToolName: "Bash", CommandHead: head, ExtractionVersion: 1, Timestamp: timestamp}
	}
	if err := db.SyncFiles([]IndexedFile{{SourcePath: "/p/session.jsonl", Source: "session", Hash: "h", Project: "p", Messages: messages}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE tool_events SET command_head = NULL WHERE ordinal IN (0, 6)`); err != nil {
		t.Fatal(err)
	}
	wantFlat := []CommandPattern{{"Bash", "", 3}, {"Bash", "git", 2}, {"Bash", "go", 2}}
	wantTrend := []CommandPatternWeekly{{"2026-W01", "Bash", "", 2}, {"2026-W01", "Bash", "git", 2}, {"2026-W01", "Bash", "go", 2}, {"2025-W52", "Bash", "", 1}}
	t.Run("flat", func(t *testing.T) {
		got, err := db.AggregateCommands(AggregateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, wantFlat) {
			t.Fatalf("got %+v, want %+v", got, wantFlat)
		}
		for i := range wantFlat {
			page, err := db.AggregateCommands(AggregateOptions{Limit: 1, Offset: i})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(page, wantFlat[i:i+1]) {
				t.Fatalf("page %d: %+v", i, page)
			}
		}
	})
	t.Run("weekly", func(t *testing.T) {
		excluded := -1
		got, err := db.AggregateCommandsTrend(AggregateOptions{}, &excluded)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, wantTrend) || excluded != 0 {
			t.Fatalf("got %+v excluded=%d, want %+v", got, excluded, wantTrend)
		}
		for i := range wantTrend {
			page, err := db.AggregateCommandsTrend(AggregateOptions{Limit: 1, Offset: i}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(page, wantTrend[i:i+1]) {
				t.Fatalf("page %d: %+v", i, page)
			}
		}
	})
	var nulls int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM tool_events WHERE command_head IS NULL`).Scan(&nulls); err != nil {
		t.Fatal(err)
	}
	if nulls != 2 {
		t.Fatalf("aggregation changed persisted proof: %d NULLs", nulls)
	}
}
