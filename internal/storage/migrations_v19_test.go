package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
)

func TestV19AddsNullableNativeCallIdentity(t *testing.T) {
	path := createFixtureDatabase(t, "v18.sql")
	db, err := openWithoutSetup(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(`INSERT INTO tool_events (source_path,ordinal,tool_name,extraction_version) VALUES ('/expired/source.jsonl',0,'exec_command',1)`); err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(context.Background(), db.DB())
	if err != nil || diag != nil {
		t.Fatalf("inspect v18: %v %+v", err, diag)
	}
	if len(plan.Steps) != 1 || plan.Steps[0] != (compat.MigrationStep{Version: 19, Name: "V19 native tool call identity"}) {
		t.Fatalf("v18 plan: %+v", plan.Steps)
	}
	if err := applyMigrationPlanForTest(context.Background(), db, plan); err != nil {
		t.Fatal(err)
	}
	var native sql.NullString
	var name string
	if err := db.DB().QueryRow(`SELECT tool_name,native_call_id FROM tool_events WHERE source_path='/expired/source.jsonl'`).Scan(&name, &native); err != nil {
		t.Fatal(err)
	}
	if name != "exec_command" || native.Valid {
		t.Fatalf("historical metadata changed: name=%s native=%+v", name, native)
	}
	assertCurrentShape(t, db.DB())
	assertMigrationVersionCount(t, db.DB(), 19, 1)
}

func TestNativeCallIdentityEnrichmentPreservesCanonicalOwnership(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := IndexedFile{SourcePath: "/native.jsonl", Source: "session", Hash: "first", Project: "p", Messages: []IndexedMessage{{UUID: "canonical-message", Ordinal: 0, Role: "assistant", Text: "call", ContentType: "tool", ToolName: "exec_command", CommandHead: "go", ExtractionVersion: 1}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	var id int64
	var native sql.NullString
	if err := db.DB().QueryRow(`SELECT id,native_call_id FROM tool_events`).Scan(&id, &native); err != nil {
		t.Fatal(err)
	}
	if native.Valid {
		t.Fatalf("unknown call ID=%+v", native)
	}
	file.Hash = "second"
	file.Messages[0].ToolUseID = "native-call"
	file.Messages[0].IsError = new(false)
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	var gotID int64
	var uuid, callID string
	var isError bool
	if err := db.DB().QueryRow(`SELECT id,message_uuid,native_call_id,is_error FROM tool_events`).Scan(&gotID, &uuid, &callID, &isError); err != nil {
		t.Fatal(err)
	}
	if gotID != id || uuid != "canonical-message" || callID != "native-call" || isError {
		t.Fatalf("enriched event id=%d uuid=%s call=%s error=%v", gotID, uuid, callID, isError)
	}
	patterns, err := db.AggregateCommands(AggregateOptions{Project: "p", Limit: 10})
	if err != nil || len(patterns) != 1 || patterns[0].Count != 1 {
		t.Fatalf("canonical event visibility: %+v err=%v", patterns, err)
	}
	file.Hash = "conflict"
	file.Messages[0].ToolUseID = "contradictory-call"
	if err := db.SyncFiles([]IndexedFile{file}); err == nil || !strings.Contains(err.Error(), "conflicting native call identity") {
		t.Fatalf("conflicting proven identity error=%v", err)
	}
	if err := db.DB().QueryRow(`SELECT native_call_id FROM tool_events`).Scan(&callID); err != nil || callID != "native-call" {
		t.Fatalf("conflict changed identity: %s %v", callID, err)
	}
}
