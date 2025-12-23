package readers

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func TestOpenCodeSnapshotMutations(t *testing.T) {
	mutations := map[string]string{
		"part payload":          `UPDATE part SET data='{"type":"text","text":"changed"}' WHERE id='p-1-1'`,
		"message without parts": `INSERT INTO message SELECT 'unattached', session_id,time_created,time_updated,data FROM message LIMIT 1`,
		"message timestamp":     `UPDATE message SET time_created=time_created+1 WHERE id='msg-1'`,
		"message payload":       `UPDATE message SET data='{"role":"assistant"}' WHERE id='msg-1'`,
		"equal max addition":    `INSERT INTO message SELECT 'added', session_id,time_created,time_updated,data FROM message WHERE id='msg-4'; INSERT INTO part SELECT 'added-part','added',session_id,time_created,time_updated,'{"type":"text","text":"added"}' FROM part LIMIT 1`,
		"message deletion":      `DELETE FROM part WHERE message_id='msg-1'; DELETE FROM message WHERE id='msg-1'`,
		"part deletion":         `DELETE FROM part WHERE id='p-1-1'`,
		"part addition":         `INSERT INTO part SELECT 'new-part',message_id,session_id,time_created,time_updated,'{"type":"text","text":"added part"}' FROM part WHERE id='p-1-1'`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			path := createOpenCodeDB(t)
			r := &OpenCodeReader{}
			before, err := r.Hash(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			unchanged, err := r.Hash(context.Background(), path)
			if err != nil || before != unchanged {
				t.Fatalf("unchanged: %s %s %v", before, unchanged, err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err = db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			after, err := r.Hash(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if before == after {
				t.Fatal("mutation did not change token")
			}
			parsed, err := r.Parse(context.Background(), path, input_config.InputDefinition{})
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Hash != after {
				t.Fatalf("parse token %s != hash %s", parsed.Hash, after)
			}
		})
	}
}

func TestOpenCodeWALSnapshotConsistency(t *testing.T) {
	path := createOpenCodeDB(t)
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err = writer.Exec(`CREATE TABLE session(id TEXT PRIMARY KEY,directory TEXT); INSERT INTO session VALUES('sess-1','/projects/original')`); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}
	reader, err := openReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	tx, err := reader.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	before, err := openCodeSnapshotHash(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	mainBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Commit while the reader holds an older snapshot; no checkpoint is possible.
	if _, err = writer.Exec(`UPDATE part SET data='{"type":"text","text":"committed WAL payload"}' WHERE id='p-1-1'; UPDATE session SET directory='/projects/committed'`); err != nil {
		t.Fatal(err)
	}
	mainAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mainBefore, mainAfter) {
		t.Fatal("fixture commit unexpectedly changed the main database file")
	}
	if info, err := os.Stat(path + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("missing committed WAL: %v", err)
	}
	old, err := parseOpenCodeSnapshot(context.Background(), tx, path)
	if err != nil {
		t.Fatal(err)
	}
	if old.Hash != before || old.Records[0].Content != "hello from user" || old.Records[0].Cwd != "/projects/original" {
		t.Fatalf("mixed snapshot: %+v", old)
	}
	fresh, err := (&OpenCodeReader{}).Parse(context.Background(), path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Hash == before || fresh.Records[0].Content != "committed WAL payload" || fresh.Records[0].Cwd != "/projects/committed" {
		t.Fatalf("missing WAL commit: %+v", fresh)
	}
	token, err := (&OpenCodeReader{}).Hash(context.Background(), path)
	if err != nil || token != fresh.Hash {
		t.Fatalf("fresh hash: %s %v", token, err)
	}
	// An uncommitted write is not part of the token.
	writeTx, err := writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer writeTx.Rollback()
	if _, err = writeTx.Exec("DELETE FROM part"); err != nil {
		t.Fatal(err)
	}
	unchanged, err := (&OpenCodeReader{}).Hash(context.Background(), path)
	if err != nil || unchanged != token {
		t.Fatalf("uncommitted write visible: %s %v", unchanged, err)
	}
}

func TestOpenCodeSnapshotCancellation(t *testing.T) {
	path := createOpenCodeDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &OpenCodeReader{}
	if _, err := r.Hash(ctx, path); err == nil {
		t.Fatal("Hash ignored cancellation")
	}
	if _, err := r.Parse(ctx, path, input_config.InputDefinition{}); err == nil {
		t.Fatal("Parse ignored cancellation")
	}
}

func TestOpenCodeSnapshotSkipsMissingSessionGroups(t *testing.T) {
	path := createOpenCodeDB(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE session(id TEXT PRIMARY KEY,directory TEXT); INSERT INTO session VALUES('sess-1','/projects/native')`); err != nil {
		t.Fatal(err)
	}
	reader := &OpenCodeReader{}
	before, err := reader.Parse(context.Background(), path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO message VALUES('orphan-a','missing',0,0,'{"role":"user"}'),('orphan-b','missing',0,0,'{"role":"assistant"}'),('orphan-empty','missing',0,0,'{"role":"user"}'); INSERT INTO part VALUES('orphan-p1','orphan-a','missing',0,0,'{"type":"text","text":"orphan"}'),('orphan-p2','orphan-a','missing',0,0,'{"type":"text","text":"also orphan"}'),('orphan-p3','orphan-b','missing',0,0,'{"type":"tool","tool":"Bash","state":{"input":{"command":"echo orphan"},"output":"orphan"}}')`); err != nil {
		t.Fatal(err)
	}
	after, err := reader.Parse(context.Background(), path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if after.SkippedMessages != 3 || len(after.Records) != len(before.Records) {
		t.Fatalf("skipped groups: %+v", after)
	}
	for i, message := range after.Records {
		want := before.Records[i]
		if message != want || message.UUID == "" || message.SessionID != "sess-1" || message.Cwd != "/projects/native" || message.Origin == "" {
			t.Fatalf("native record changed: %+v want %+v", message, want)
		}
	}
	if _, err := db.Exec(`INSERT INTO part VALUES('broken','absent-message','missing',0,0,'{"type":"text","text":"broken"}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Parse(context.Background(), path, input_config.InputDefinition{}); err == nil || err.Error() != "invalid OpenCode message/part relationship" {
		t.Fatalf("relationship error: %v", err)
	}
}
