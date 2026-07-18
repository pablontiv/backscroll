package storage

import (
	"path/filepath"
	"testing"
)

func sessionFile(hash string, msgs []IndexedMessage) []IndexedFile {
	return []IndexedFile{{SourcePath: "/p/grow.jsonl", Source: "session", Hash: hash, Project: "proj", Messages: msgs}}
}

func TestGrowingSessionKeepsStableIDs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	m0 := IndexedMessage{Ordinal: 0, Role: "user", Text: "hola", UUID: "u1",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m0})); err != nil {
		t.Fatal(err)
	}

	var idBefore int64
	if err := db.db.QueryRow("SELECT id FROM search_items WHERE uuid='u1'").Scan(&idBefore); err != nil {
		t.Fatal(err)
	}

	// session grows: same first message, one new message
	m1 := IndexedMessage{Ordinal: 1, Role: "assistant", Text: "hola!", UUID: "u2",
		Timestamp: "2026-01-01T00:00:05Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h2", []IndexedMessage{m0, m1})); err != nil {
		t.Fatal(err)
	}

	var idAfter int64
	if err := db.db.QueryRow("SELECT id FROM search_items WHERE uuid='u1'").Scan(&idAfter); err != nil {
		t.Fatal(err)
	}
	if idBefore != idAfter {
		t.Errorf("perennity violated: id changed %d -> %d on re-sync", idBefore, idAfter)
	}

	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path='/p/grow.jsonl'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("want 2 rows after growth, got %d", n)
	}
}

func TestUUIDLessSessionKeepsLegacyReload(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	m := IndexedMessage{Ordinal: 0, Role: "user", Text: "sin uuid",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m})); err != nil {
		t.Fatal(err)
	}
	if err := db.SyncFiles(sessionFile("h2", []IndexedMessage{m})); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path='/p/grow.jsonl'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("legacy reload must not duplicate: got %d rows", n)
	}
}

func TestRebuildFTSRestoresIndexWithoutDataLoss(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	m := IndexedMessage{Ordinal: 0, Role: "user", Text: "perennial evidence", UUID: "u1",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m})); err != nil {
		t.Fatal(err)
	}

	if err := db.RebuildFTS(); err != nil {
		t.Fatalf("rebuild fts: %v", err)
	}

	// row survived and is still searchable after re-derivation
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM search_items").Scan(&n); err != nil || n != 1 {
		t.Fatalf("data loss: n=%d err=%v", n, err)
	}
	var hits int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'perennial'").Scan(&hits); err != nil || hits != 1 {
		t.Fatalf("fts not re-derived: hits=%d err=%v", hits, err)
	}
}

func TestPurgeDeletesToolEventsExplicitly(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	old := IndexedMessage{Ordinal: 0, Role: "assistant", Text: "Bash command=rm", UUID: "u1",
		Timestamp: "2020-01-01T00:00:00Z", ContentType: "tool", ToolName: "Bash", CommandHead: "rm", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{old})); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Purge("2021-01-01"); err != nil {
		t.Fatalf("purge: %v", err)
	}

	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM tool_events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("purge must delete satellite tool_events rows, %d remain", n)
	}
}

func TestSessionSurvivesSourceFileExpiry(t *testing.T) {
	// Simulates: session indexed -> JSONL expires from disk -> rebuild runs.
	// The perennity contract: rows survive both events.
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	m := IndexedMessage{Ordinal: 0, Role: "user", Text: "irreplaceable history", UUID: "u1",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m})); err != nil {
		t.Fatal(err)
	}

	// "File expired": no further syncs mention /p/grow.jsonl. Rebuild runs.
	if err := db.RebuildFTS(); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE uuid='u1'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("perennity violated: row lost after source expiry + rebuild (n=%d err=%v)", n, err)
	}
}

func TestPerennialFileNeverWipedOnFlap(t *testing.T) {
	// Flap guard: a file synced as perennial (all messages have UUID) should
	// stay perennial even if a later sync has some uuid-less messages (parsing
	// drift). Existing rows must not be wiped.
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// First sync: all messages have UUIDs (perennial)
	m0 := IndexedMessage{Ordinal: 0, Role: "user", Text: "first", UUID: "u1",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m0})); err != nil {
		t.Fatal(err)
	}

	var idBefore int64
	if err := db.db.QueryRow("SELECT id FROM search_items WHERE uuid='u1'").Scan(&idBefore); err != nil {
		t.Fatal(err)
	}

	// Re-sync: one message still has UUID, but add a second without UUID (parsing drift).
	// Flap guard should preserve the first message's row and NOT wipe the file.
	m1 := IndexedMessage{Ordinal: 1, Role: "assistant", Text: "second (no uuid)",
		Timestamp: "2026-01-01T00:00:05Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h2", []IndexedMessage{m0, m1})); err != nil {
		t.Fatal(err)
	}

	// Check m0's ID is unchanged
	var idAfter int64
	if err := db.db.QueryRow("SELECT id FROM search_items WHERE uuid='u1'").Scan(&idAfter); err != nil {
		t.Fatal(err)
	}
	if idBefore != idAfter {
		t.Errorf("flap guard failed: id changed %d -> %d when uuid-less message added", idBefore, idAfter)
	}

	// Check only m0 was inserted (m1 has no UUID, so INSERT OR IGNORE skips it)
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path='/p/grow.jsonl'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 row (m0 with uuid), got %d", n)
	}
}

func TestToolEventsUUIDDedupesAcrossOrdinals(t *testing.T) {
	// UNIQUE INDEX on tool_events(message_uuid) WHERE message_uuid IS NOT NULL
	// prevents the same message_uuid from being inserted at different ordinals.
	// This guards against ordinal drift creating duplicate tool_events.
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// First sync: tool_events with message_uuid u1 at ordinal 0
	m := IndexedMessage{Ordinal: 0, Role: "assistant", Text: "Bash", UUID: "u1",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool", ToolName: "Bash", CommandHead: "ls", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m})); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM tool_events WHERE message_uuid='u1'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 tool_event row, got %d", n)
	}

	// Re-sync: same message_uuid u1 but at a different ordinal (drift).
	// INSERT OR IGNORE should skip this due to UNIQUE(message_uuid).
	m2 := IndexedMessage{Ordinal: 1, Role: "assistant", Text: "Bash", UUID: "u1",
		Timestamp: "2026-01-01T00:00:05Z", ContentType: "tool", ToolName: "Bash", CommandHead: "ls", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h2", []IndexedMessage{m2})); err != nil {
		t.Fatal(err)
	}

	// tool_events count should still be 1 (deduped)
	if err := db.db.QueryRow("SELECT COUNT(*) FROM tool_events WHERE message_uuid='u1'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 tool_event row after re-sync (deduped), got %d", n)
	}
}
