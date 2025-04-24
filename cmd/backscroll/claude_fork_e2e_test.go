package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestClaudeForkCopiesCanonicalOwnerE2E(t *testing.T) {
	e := newQueryEchoE2E(t)
	const sharedUUID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const ownerTailUUID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	const forkTailUUID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	const sharedText = "canonical orchard shared history"
	const ownerTail = "original juniper unique tail"
	const forkTail = "fork sapphire unique tail"

	// Only the envelope changes between copies of the native message. A Bash
	// invocation also proves copied history is one execution, not one per file.
	write := func(name, session, tailUUID, tail string) string {
		t.Helper()
		path := filepath.Join(e.fixtures, name)
		shared := map[string]any{
			"uuid": sharedUUID, "type": "assistant", "sessionId": session,
			"promptId": "prompt-" + session, "parentUuid": nil,
			"timestamp": "2026-01-01T00:00:00Z", "cwd": "/synthetic/claude-fork",
			"message": map[string]any{"role": "assistant", "content": []map[string]any{
				{"type": "text", "text": sharedText},
				{"type": "tool_use", "id": "synthetic-bash-call", "name": "Bash", "input": map[string]any{"command": "printf synthetic-fork-proof"}},
			}},
		}
		data, err := json.Marshal(shared)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, '\n')
		if tailUUID != "" {
			record, err := json.Marshal(map[string]any{
				"uuid": tailUUID, "type": "user", "sessionId": session,
				"promptId": "tail-" + session, "parentUuid": sharedUUID,
				"timestamp": "2026-01-01T00:01:00Z", "cwd": "/synthetic/claude-fork",
				"message": map[string]any{"role": "user", "content": tail},
			})
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, append(record, '\n')...)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	owner := write("m-owner.jsonl", "owner-session", ownerTailUUID, ownerTail)
	fork := write("z-fork.jsonl", "fork-session", forkTailUUID, forkTail)
	e.run("status", "--json")

	type anchor struct {
		ID      int64
		UUID    string
		Path    string
		Ordinal int64
	}
	readAnchor := func() anchor {
		t.Helper()
		db, err := sql.Open("sqlite", "file:"+e.database+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		var got anchor
		if err := db.QueryRow("SELECT id, uuid, source_path, ordinal FROM search_items WHERE uuid = ?", sharedUUID).Scan(&got.ID, &got.UUID, &got.Path, &got.Ordinal); err != nil {
			t.Fatal(err)
		}
		return got
	}
	baseline := readAnchor()
	if baseline.Path != owner || baseline.UUID != sharedUUID || baseline.Ordinal != 0 || baseline.ID <= 0 {
		t.Fatalf("canonical anchor = %+v, want first owner at ordinal 0", baseline)
	}

	assertConsumers := func() {
		t.Helper()
		for _, want := range []struct{ query, path, uuid string }{
			{sharedText, owner, sharedUUID}, {ownerTail, owner, ownerTailUUID}, {forkTail, fork, forkTailUUID},
		} {
			results := e.searchJSON(want.query, "", 20)
			if len(results) != 1 || results[0].FilePath != want.path || (want.uuid != sharedUUID && results[0].Content != want.query) {
				t.Fatalf("search %q = %+v, want one result at %s", want.query, results, want.path)
			}
			var context contextEnvelope
			if err := json.Unmarshal([]byte(e.run("context", "--uuid", want.uuid, "--before", "10", "--after", "10", "--max-tokens", "4096", "--json")), &context); err != nil {
				t.Fatal(err)
			}
			if context.Anchor.UUID == nil || *context.Anchor.UUID != want.uuid || context.Anchor.SourcePath != want.path || context.Truncated || context.Omitted != 0 {
				t.Fatalf("context anchor for %s = %+v", want.uuid, context)
			}
			wantUUIDs := []string{sharedUUID, sharedUUID + "#t1", ownerTailUUID}
			if want.uuid == forkTailUUID {
				wantUUIDs = []string{forkTailUUID}
			}
			var gotUUIDs []string
			anchors := 0
			for _, record := range context.Records {
				if record.UUID == nil || record.SourcePath != want.path {
					t.Fatalf("context record escaped canonical source: %+v", record)
				}
				gotUUIDs = append(gotUUIDs, *record.UUID)
				if record.IsAnchor {
					anchors++
					if *record.UUID != want.uuid || record.Ordinal != context.Anchor.Ordinal {
						t.Fatalf("context record anchor = %+v, envelope = %+v", record, context.Anchor)
					}
				}
			}
			if !reflect.DeepEqual(gotUUIDs, wantUUIDs) || anchors != 1 {
				t.Fatalf("context %s UUIDs=%v anchors=%d, want %v and one anchor", want.uuid, gotUUIDs, anchors, wantUUIDs)
			}
		}
	}
	assertConsumers()

	// A later lexically earlier, fully shared source cannot steal ownership,
	// even when the original transcript is no longer available for ingestion.
	if err := os.Remove(owner); err != nil {
		t.Fatal(err)
	}
	copyPath := write("a-copy.jsonl", "copy-session", "", "")
	e.run("status", "--json")
	for startup := range 2 {
		e.run("status", "--json")
		if got := readAnchor(); got != baseline {
			t.Fatalf("startup %d changed anchor: got %+v want %+v", startup, got, baseline)
		}
		assertConsumers()
		db, err := sql.Open("sqlite", "file:"+e.database+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		var rows, events, progress int
		err = db.QueryRow("SELECT (SELECT count(*) FROM search_items), (SELECT count(*) FROM tool_events), (SELECT count(*) FROM indexed_files WHERE path IN (?, ?) AND hash <> '')", fork, copyPath).Scan(&rows, &events, &progress)
		_ = db.Close()
		if err != nil {
			t.Fatal(err)
		}
		if rows != 4 || events != 1 || progress != 2 {
			t.Fatalf("startup %d census: rows=%d events=%d progressed live files=%d, want 4/1/2", startup, rows, events, progress)
		}
	}
	e.assertIntegrity()
}
