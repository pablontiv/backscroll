package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
)

type originReplayClaudeReader struct {
	delegate   readers.ClaudeReader
	parseCalls int
}

func (*originReplayClaudeReader) Name() string { return "claude" }

func (r *originReplayClaudeReader) Discover(def input_config.InputDefinition) ([]string, error) {
	return r.delegate.Discover(def)
}

func (r *originReplayClaudeReader) Hash(path string) (string, error) {
	return r.delegate.Hash(path)
}

func (r *originReplayClaudeReader) Parse(path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	r.parseCalls++
	return r.delegate.Parse(path, def)
}

func TestOriginParserSyncReplayIsBoundedAndConverges(t *testing.T) {
	tmp := t.TempDir()
	liveRoot := filepath.Join(tmp, "live")
	if err := os.MkdirAll(liveRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	const liveFiles = 201
	oldTime := time.Now().Add(-time.Hour)
	for i := 0; i < liveFiles; i++ {
		path := filepath.Join(liveRoot, fmt.Sprintf("session-%03d.jsonl", i))
		record := fmt.Sprintf(`{"type":"user","uuid":"origin-live-%03d","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"parser-backed origin %03d"}}`, i, i)
		if err := os.WriteFile(path, []byte(record+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}

	reader := &originReplayClaudeReader{}
	oldActiveInputs, oldNewRegistry := maybeAutoSyncActiveInputs, maybeAutoSyncNewRegistry
	t.Cleanup(func() {
		maybeAutoSyncActiveInputs = oldActiveInputs
		maybeAutoSyncNewRegistry = oldNewRegistry
	})
	maybeAutoSyncActiveInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{
			ID:     "origin-replay-claude",
			Source: "session",
			Active: true,
			Discover: input_config.DiscoverConfig{
				Roots:   []string{liveRoot},
				Include: []string{"*.jsonl"},
			},
			Decode: input_config.DecodeConfig{Format: "claude"},
		}}, input_config.ModeDeclarative, nil
	}
	maybeAutoSyncNewRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(reader)
		return registry
	}

	cfg := config.Config{DatabasePath: filepath.Join(tmp, "index.db")}
	if err := maybeAutoSync(&cfg, &bytes.Buffer{}); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if reader.parseCalls != liveFiles {
		t.Fatalf("initial parser calls = %d, want %d", reader.parseCalls, liveFiles)
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	var initialOrigin models.MessageOrigin
	var initialOriginVersion int
	if err := db.DB().QueryRow(`SELECT origin, origin_version FROM search_items WHERE uuid = 'origin-live-000'`).Scan(&initialOrigin, &initialOriginVersion); err != nil {
		t.Fatal(err)
	}
	if initialOrigin != models.OriginHuman || initialOriginVersion != storage.CurrentOriginVersion {
		t.Fatalf("fresh parser origin = (%q, %d), want (%q, %d)", initialOrigin, initialOriginVersion, models.OriginHuman, storage.CurrentOriginVersion)
	}

	initialHashes := make(map[string]string, liveFiles)
	rows, err := db.DB().Query(`SELECT path, hash FROM indexed_files WHERE path LIKE ?`, liveRoot+"%")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		initialHashes[path] = hash
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(initialHashes) != liveFiles {
		t.Fatalf("initial indexed hashes = %d, want %d", len(initialHashes), liveFiles)
	}

	// Reproduce the v16 backlog. One row also enters the search_echo backlog;
	// the two provenance queues must still spend only one parse for that path.
	if _, err := db.DB().Exec(`UPDATE search_items SET origin='unknown', origin_version=NULL, search_echo=CASE WHEN uuid='origin-live-000' THEN NULL ELSE 0 END WHERE source_path LIKE ?`, liveRoot+"%"); err != nil {
		t.Fatal(err)
	}

	// More recent, expired sources sort ahead of every live source. They must
	// remain unknown without preventing the live backlog from using the cap.
	const expiredFiles = 201
	expiredRoot := filepath.Join(tmp, "expired")
	tx, err := db.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < expiredFiles; i++ {
		path := filepath.Join(expiredRoot, fmt.Sprintf("expired-%03d.jsonl", i))
		if _, err := tx.Exec(`INSERT INTO indexed_files(path, hash, last_indexed) VALUES (?, ?, '2099-01-01T00:00:00Z')`, path, fmt.Sprintf("expired-hash-%03d", i)); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if _, err := tx.Exec(`
			INSERT INTO search_items(source, source_path, ordinal, role, origin, text, uuid, content_type, extraction_version, search_echo, origin_version)
			VALUES ('session', ?, 0, 'assistant', 'unknown', 'stored role and text are not provenance', ?, 'text', ?, 0, NULL)
		`, path, fmt.Sprintf("origin-expired-%03d", i), storage.CurrentExtractionVersion); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	beforeReplay := reader.parseCalls
	if err := maybeAutoSync(&cfg, &bytes.Buffer{}); err != nil {
		t.Fatalf("first origin replay: %v", err)
	}
	if got := reader.parseCalls - beforeReplay; got != 200 {
		t.Fatalf("first origin replay parsed %d files, want total cap 200", got)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var got int
		if err := db.DB().QueryRow(query, args...).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := count(`SELECT COUNT(*) FROM search_items WHERE source_path LIKE ? AND origin_version IS NULL`, liveRoot+"%"); got != 1 {
		t.Fatalf("live origin backlog after first replay = %d, want 1", got)
	}
	if got := count(`SELECT COUNT(*) FROM search_items WHERE source_path LIKE ? AND search_echo IS NULL`, liveRoot+"%"); got != 0 {
		t.Fatalf("overlapping search_echo backlog after first replay = %d, want 0", got)
	}
	if got := count(`SELECT COUNT(*) FROM search_items WHERE source_path LIKE ? AND origin='unknown' AND origin_version IS NULL`, expiredRoot+"%"); got != expiredFiles {
		t.Fatalf("expired unknown origins after replay = %d, want %d", got, expiredFiles)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	beforeReplay = reader.parseCalls
	if err := maybeAutoSync(&cfg, &bytes.Buffer{}); err != nil {
		t.Fatalf("second origin replay: %v", err)
	}
	if got := reader.parseCalls - beforeReplay; got != 1 {
		t.Fatalf("second origin replay parsed %d files, want final 1", got)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := count(`SELECT COUNT(*) FROM search_items WHERE source_path LIKE ? AND (origin != 'human' OR origin_version != ?)`, liveRoot+"%", storage.CurrentOriginVersion); got != 0 {
		t.Fatalf("live rows without persisted parser origin after convergence = %d", got)
	}
	for path, wantHash := range initialHashes {
		var gotHash string
		if err := db.DB().QueryRow(`SELECT hash FROM indexed_files WHERE path = ?`, path).Scan(&gotHash); err != nil {
			t.Fatal(err)
		}
		if gotHash != wantHash {
			t.Fatalf("replay changed hash for %s: got %q want %q", path, gotHash, wantHash)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	beforeReplay = reader.parseCalls
	if err := maybeAutoSync(&cfg, &bytes.Buffer{}); err != nil {
		t.Fatalf("converged origin replay: %v", err)
	}
	if got := reader.parseCalls - beforeReplay; got != 0 {
		t.Fatalf("converged origin replay parsed %d files, want 0", got)
	}
}
