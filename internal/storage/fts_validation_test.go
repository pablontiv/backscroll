package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the production API on a real temporary database, including read-only validation.
func TestValidateFTSMembershipReadOnly(t *testing.T) {
	for _, corruption := range []string{"healthy", "missing prose", "missing tool", "tool in prose", "prose in tool", "generic rebuild", "orphan posting"} {
		t.Run(corruption, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.db")
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.SyncFiles([]IndexedFile{{SourcePath: "fixture.jsonl", Source: "session", Hash: "fixture", Messages: []IndexedMessage{
				{Ordinal: 0, Text: "healthy prose words", ContentType: "text"},
				{Ordinal: 1, Text: "command tool output", ContentType: "tool"},
				{Ordinal: 2, Text: "", ContentType: "text"},
				{Ordinal: 3, Text: "ab", ContentType: "tool"},
				{Ordinal: 4, Text: "!!!", ContentType: "code"},
				{Ordinal: 5, Text: "reasoning proof", ContentType: "reasoning"},
			}}}); err != nil {
				t.Fatal(err)
			}
			statements := map[string]string{
				"missing prose":   "INSERT INTO messages_fts(messages_fts,rowid,text) SELECT 'delete',id,text FROM search_items WHERE ordinal=0",
				"missing tool":    "INSERT INTO tool_fts(tool_fts,rowid,text) SELECT 'delete',id,text FROM search_items WHERE ordinal=1",
				"tool in prose":   "INSERT INTO messages_fts(rowid,text) SELECT id,text FROM search_items WHERE ordinal=1",
				"prose in tool":   "INSERT INTO tool_fts(rowid,text) SELECT id,text FROM search_items WHERE ordinal=0",
				"generic rebuild": "INSERT INTO messages_fts(messages_fts) VALUES('rebuild')",
				"orphan posting":  "INSERT INTO tool_fts(rowid,text) VALUES(99999,'orphan posting')",
			}
			if stmt := statements[corruption]; stmt != "" {
				if _, err := db.DB().Exec(stmt); err != nil {
					t.Fatal(err)
				}
			}
			ro, err := OpenReadOnly(path)
			if err != nil {
				t.Fatal(err)
			}
			defer ro.Close()
			for i := 0; i < 2; i++ { // Validation must not repair corruption or leave temporary objects behind.
				err = ro.Validate()
				if corruption == "healthy" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "FTS membership") || !strings.Contains(err.Error(), "rebuild") {
					t.Fatalf("expected actionable corruption diagnostic, got %v", err)
				}
			}
		})
	}
}
