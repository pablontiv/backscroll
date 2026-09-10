package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestPairedEchoRefillAfterFTSRebuild(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	messages := make([]IndexedMessage, 0, 206)
	for i := 0; i < 205; i++ {
		messages = append(messages, IndexedMessage{Ordinal: i, Role: "user", UUID: fmt.Sprint("echo-", i), Text: "orchard", ContentType: "tool", SearchEcho: true})
	}
	messages = append(messages, IndexedMessage{Ordinal: 205, Role: "user", UUID: "legitimate", Text: "orchard", ContentType: "tool"})
	if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "active", Hash: "h", Messages: messages}}); err != nil {
		t.Fatal(err)
	}
	if err := db.RebuildFTS(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		got, err := db.Search("orchard", models.SearchOptions{AllProjects: true, Limit: 300})
		if err != nil || len(got) != 1 || got[0].UUID != "legitimate" {
			t.Fatalf("refill lost legitimate output: %v %v", got, err)
		}
	}
	tools, err := db.Search("orchard", models.SearchOptions{AllProjects: true, ContentType: "tool", Limit: 300})
	if err != nil || len(tools) != 206 {
		t.Fatalf("tool-only data lost: count=%d err=%v", len(tools), err)
	}
}
