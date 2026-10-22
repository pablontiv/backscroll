package readers

import (
	"context"
	"database/sql"
	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/input_config"
	"strings"
	"testing"
)

func TestOpenCodeNativeRecordOwnership(t *testing.T) {
	path := createOpenCodeDB(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE session(id TEXT PRIMARY KEY,directory TEXT); INSERT INTO session VALUES('sess-1','/projects/alpha'),('sess-2','/projects/beta'); UPDATE message SET session_id='sess-2' WHERE id='msg-4'; UPDATE part SET session_id='sess-2' WHERE message_id='msg-4'`); err != nil {
		t.Fatal(err)
	}
	reader := &OpenCodeReader{}
	first, err := reader.Parse(context.Background(), path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]string)
	for _, m := range first.Records {
		if m.UUID == "" {
			t.Fatal("native identity was discarded")
		}
		if ids[m.UUID] != "" {
			t.Fatal("duplicate identity")
		}
		ids[m.UUID] = m.Content
		if m.SessionID == "" {
			t.Fatal("native session was discarded")
		}
		if m.Cwd == "" {
			t.Fatal("native directory was discarded")
		}
	}
	if _, err := db.Exec(`UPDATE message SET time_created=-1 WHERE id='msg-4'; UPDATE part SET data='{"type":"text","text":"mutated"}' WHERE id='p-1-1'`); err != nil {
		t.Fatal(err)
	}
	second, err := reader.Parse(context.Background(), path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range second.Records {
		if _, ok := ids[m.UUID]; !ok {
			t.Fatalf("identity changed: %s", m.UUID)
		}
	}
	if first.Hash == second.Hash {
		t.Fatal("snapshot mutation not hashed")
	}
	if _, err := db.Exec(`UPDATE session SET directory='/projects/renamed' WHERE id='sess-1'`); err != nil {
		t.Fatal(err)
	}
	hash, err := reader.Hash(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if hash == second.Hash {
		t.Fatal("session metadata not hashed")
	}
	if _, err := db.Exec(`UPDATE part SET session_id='wrong' WHERE id='p-1-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Parse(context.Background(), path, input_config.InputDefinition{}); err == nil {
		t.Fatal("contradictory native session accepted")
	}
}

func TestOpenCodeNativeToolIdentityAndMalformedMetadata(t *testing.T) {
	path := createOpenCodeDB(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE session(id TEXT PRIMARY KEY,directory TEXT);INSERT INTO session VALUES('sess-1','/projects/native');INSERT INTO part VALUES('tool-part','msg-1','sess-1',0,0,'{"type":"tool","tool":"Bash","state":{"input":{"command":"go test"},"output":"done"}}')`); err != nil {
		t.Fatal(err)
	}
	reader := &OpenCodeReader{}
	parsed, err := reader.Parse(context.Background(), path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	var input, output string
	for _, m := range parsed.Records {
		if m.ToolName == "Bash" {
			input = m.UUID
		}
		if m.Content == "done" {
			output = m.UUID
		}
	}
	if !strings.HasSuffix(input, "#t0") || !strings.HasSuffix(output, "#r0") || strings.TrimSuffix(input, "#t0") != strings.TrimSuffix(output, "#r0") {
		t.Fatalf("tool identities %s/%s", input, output)
	}
	if err := compat.ValidateRecoveryUUID(input); err != nil {
		t.Fatal(err)
	}
	if err := compat.ValidateRecoveryUUID(output); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE session SET directory='relative'`); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Parse(context.Background(), path, input_config.InputDefinition{}); err == nil {
		t.Fatal("malformed cwd silently fell back to capability path")
	}
	if _, err := db.Exec(`DELETE FROM session`); err != nil {
		t.Fatal(err)
	}
	orphaned, err := reader.Parse(context.Background(), path, input_config.InputDefinition{})
	if err != nil || len(orphaned.Records) != 0 || orphaned.SkippedMessages != 4 {
		t.Fatalf("orphan groups: %+v err=%v", orphaned, err)
	}
}

func TestOpenCodeNativeSessionlessExportAndOrphanRelationships(t *testing.T) {
	path := createOpenCodeDB(t)
	reader := &OpenCodeReader{}
	parsed, err := reader.Parse(context.Background(), path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Records) == 0 {
		t.Fatal("expected native records")
	}
	for _, message := range parsed.Records {
		if message.UUID == "" || message.SessionID != "sess-1" || message.Cwd != "" {
			t.Fatalf("sessionless ownership: %+v", message)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE part SET message_id='missing' WHERE id='p-1-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Parse(context.Background(), path, input_config.InputDefinition{}); err == nil {
		t.Fatal("orphan part accepted")
	}
	if _, err := db.Exec(`UPDATE part SET message_id='msg-1' WHERE id='p-1-1'; UPDATE message SET session_id=' sess-1' WHERE id='msg-1'; UPDATE part SET session_id=' sess-1' WHERE message_id='msg-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Parse(context.Background(), path, input_config.InputDefinition{}); err == nil {
		t.Fatal("malformed native session accepted")
	}
}
