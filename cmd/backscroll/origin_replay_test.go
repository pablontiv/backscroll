package main

import (
	"bytes"
	"context"
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
	delegate           readers.ClaudeReader
	parseCalls         int
	parsePaths         []string
	reverseDiscovery   bool
	replayUUIDLessRows bool
	zeroRecords        bool
	parseErr           error
}

func (*originReplayClaudeReader) Name() string { return "claude" }

func (r *originReplayClaudeReader) Discover(ctx context.Context, def input_config.InputDefinition) ([]string, error) {
	refs, err := r.delegate.Discover(ctx, def)
	if err != nil || !r.reverseDiscovery {
		return refs, err
	}
	for left, right := 0, len(refs)-1; left < right; left, right = left+1, right-1 {
		refs[left], refs[right] = refs[right], refs[left]
	}
	return refs, nil
}

func (r *originReplayClaudeReader) Hash(ctx context.Context, path string) (string, error) {
	return r.delegate.Hash(ctx, path)
}

func (r *originReplayClaudeReader) Parse(ctx context.Context, path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	r.parseCalls++
	r.parsePaths = append(r.parsePaths, path)
	if r.parseErr != nil {
		return models.ParsedFile{}, r.parseErr
	}
	parsed, err := r.delegate.Parse(ctx, path, def)
	if err != nil {
		return models.ParsedFile{}, err
	}
	if r.zeroRecords {
		parsed.Records = nil
		return parsed, nil
	}
	if r.replayUUIDLessRows {
		var index int
		if _, err := fmt.Sscanf(filepath.Base(path), "session-%d.jsonl", &index); err == nil && index < 200 {
			uuidLess := models.Message{Role: "user", Origin: models.OriginHuman, Content: "current parser row without identity", ContentType: "text"}
			if index%2 == 0 {
				parsed.Records = append(parsed.Records, uuidLess)
			} else {
				parsed.Records = []models.Message{uuidLess}
			}
		}
	}
	return parsed, nil
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
	syncService := newStartupSyncService()
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
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
	syncService.newRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(reader)
		return registry
	}

	cfg := config.Config{DatabasePath: filepath.Join(tmp, "index.db")}
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
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

	// The first 200 perennial paths also retain a historical UUID which the
	// current parser no longer emits. They consume the first replay cap, then
	// must leave the queue so the final live path can advance on the next run.
	const retainedFiles = 200
	tx, err := db.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < retainedFiles; i++ {
		path := filepath.Join(liveRoot, fmt.Sprintf("session-%03d.jsonl", i))
		if _, err := tx.Exec(`
			INSERT INTO search_items(source, source_path, ordinal, role, origin, text, uuid, content_type, extraction_version, search_echo, origin_version)
			VALUES ('session', ?, 1, 'assistant', 'unknown', 'retained historical payload', ?, 'text', ?, 0, NULL)
		`, path, fmt.Sprintf("origin-retained-%03d", i), storage.CurrentExtractionVersion); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}

	// More recent, expired sources sort ahead of every live source. They must
	// remain unknown without preventing the live backlog from using the cap.
	const expiredFiles = 201
	expiredRoot := filepath.Join(tmp, "expired")
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
	if _, err := db.DB().Exec(`UPDATE indexed_files SET last_indexed = '2098-01-01T00:00:00Z' WHERE path LIKE ?`, liveRoot+"%"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Discovery intentionally runs in the opposite order from the durable queue.
	// The first 200 paths also keep emitting UUID-less or mixed output; a
	// successful replay must close them so the final live path can advance.
	reader.reverseDiscovery = true
	reader.replayUUIDLessRows = true
	reader.parsePaths = nil
	beforeReplay := reader.parseCalls
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("first origin replay: %v", err)
	}
	if got := reader.parseCalls - beforeReplay; got != 200 {
		t.Fatalf("first origin replay parsed %d files, want total cap 200", got)
	}
	selected := make(map[string]bool, len(reader.parsePaths))
	for _, path := range reader.parsePaths {
		selected[path] = true
	}
	if !selected[filepath.Join(liveRoot, "session-000.jsonl")] || selected[filepath.Join(liveRoot, "session-200.jsonl")] {
		t.Fatalf("origin selection followed discovery instead of queue order: first=%v final=%v", selected[filepath.Join(liveRoot, "session-000.jsonl")], selected[filepath.Join(liveRoot, "session-200.jsonl")])
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
	if got := count(`SELECT COUNT(*) FROM search_items WHERE source_path LIKE ? AND (origin_version IS NULL OR origin_version < ?)`, liveRoot+"%", storage.CurrentOriginVersion); got != 1 {
		t.Fatalf("live origin backlog after first replay = %d, want 1", got)
	}
	var pendingPath string
	if err := db.DB().QueryRow(`SELECT DISTINCT source_path FROM search_items WHERE source_path LIKE ? AND (origin_version IS NULL OR origin_version < ?)`, liveRoot+"%", storage.CurrentOriginVersion).Scan(&pendingPath); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(liveRoot, "session-200.jsonl"); pendingPath != want {
		t.Fatalf("pending path after reverse discovery = %q, want queue tail %q", pendingPath, want)
	}
	if got := count(`SELECT COUNT(*) FROM search_items WHERE uuid LIKE 'origin-retained-%' AND (origin != 'unknown' OR origin_version != ?)`, storage.CurrentOriginVersion); got != 0 {
		t.Fatalf("retained rows not closed as current unknown after first replay = %d", got)
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

	reader.parsePaths = nil
	beforeReplay = reader.parseCalls
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("second origin replay: %v", err)
	}
	if got := reader.parseCalls - beforeReplay; got != 1 {
		t.Fatalf("second origin replay parsed %d files, want final 1", got)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := count(`SELECT COUNT(*) FROM search_items WHERE uuid LIKE 'origin-live-%' AND origin_version != ?`, storage.CurrentOriginVersion); got != 0 {
		t.Fatalf("live parser rows without current origin version after convergence = %d", got)
	}
	if got := count(`SELECT COUNT(*) FROM search_items WHERE uuid LIKE 'origin-retained-%' AND (origin != 'unknown' OR origin_version != ?)`, storage.CurrentOriginVersion); got != 0 {
		t.Fatalf("retained rows without closed unknown origin after convergence = %d", got)
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
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("converged origin replay: %v", err)
	}
	if got := reader.parseCalls - beforeReplay; got != 0 {
		t.Fatalf("converged origin replay parsed %d files, want 0", got)
	}
}

func TestOriginPerennialZeroMessageReplayPreservesHistoryAndConverges(t *testing.T) {
	tmp := t.TempDir()
	liveRoot := filepath.Join(tmp, "live")
	if err := os.MkdirAll(liveRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(liveRoot, "zero.jsonl")
	record := `{"type":"user","uuid":"zero-origin","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"retained zero replay payload"}}`
	if err := os.WriteFile(path, []byte(record+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	reader := &originReplayClaudeReader{}
	syncService := newStartupSyncService()
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{
			ID:     "origin-zero-claude",
			Source: "session",
			Active: true,
			Discover: input_config.DiscoverConfig{
				Roots:   []string{liveRoot},
				Include: []string{"*.jsonl"},
			},
			Decode: input_config.DecodeConfig{Format: "claude"},
		}}, input_config.ModeDeclarative, nil
	}
	syncService.newRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(reader)
		return registry
	}

	cfg := config.Config{DatabasePath: filepath.Join(tmp, "index.db")}
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	var originalID int64
	if err := db.DB().QueryRow(`SELECT id FROM search_items WHERE uuid = 'zero-origin'`).Scan(&originalID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO tool_events(message_uuid, source_path, ordinal, tool_name, command_head, is_error, extraction_version)
		VALUES ('zero-origin', ?, 0, 'Bash', 'go test', 0, ?)
	`, path, storage.CurrentExtractionVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE search_items SET origin = 'unknown', origin_version = 0 WHERE uuid = 'zero-origin'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reader.parseErr = fmt.Errorf("injected parse failure")
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err == nil {
		t.Fatal("parse failure unexpectedly succeeded")
	}
	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	var failedVersion int
	var failedItems, failedEvents int
	if err := db.DB().QueryRow(`SELECT origin_version FROM search_items WHERE uuid = 'zero-origin'`).Scan(&failedVersion); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE uuid = 'zero-origin'`).Scan(&failedItems); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM tool_events WHERE message_uuid = 'zero-origin'`).Scan(&failedEvents); err != nil {
		t.Fatal(err)
	}
	if failedVersion != 0 || failedItems != 1 || failedEvents != 1 {
		t.Fatalf("parse failure mutated history: version=%d items=%d events=%d", failedVersion, failedItems, failedEvents)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reader.parseErr = nil
	reader.zeroRecords = true
	reader.parsePaths = nil
	beforeReplay := reader.parseCalls
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("zero-message replay: %v", err)
	}
	if got := reader.parseCalls - beforeReplay; got != 1 {
		t.Fatalf("zero-message replay parser calls = %d, want 1", got)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	var gotID int64
	var gotText, gotOrigin string
	var gotVersion int
	if err := db.DB().QueryRow(`SELECT id, text, origin, origin_version FROM search_items WHERE uuid = 'zero-origin'`).Scan(&gotID, &gotText, &gotOrigin, &gotVersion); err != nil {
		t.Fatal(err)
	}
	if gotID != originalID || gotText != "retained zero replay payload" || gotOrigin != "unknown" || gotVersion != storage.CurrentOriginVersion {
		t.Fatalf("retained row = (%d, %q, %q, %d), want (%d, %q, unknown, %d)", gotID, gotText, gotOrigin, gotVersion, originalID, "retained zero replay payload", storage.CurrentOriginVersion)
	}
	var toolName, commandHead string
	if err := db.DB().QueryRow(`SELECT tool_name, command_head FROM tool_events WHERE message_uuid = 'zero-origin'`).Scan(&toolName, &commandHead); err != nil {
		t.Fatal(err)
	}
	if toolName != "Bash" || commandHead != "go test" {
		t.Fatalf("retained tool event = (%q, %q), want (Bash, go test)", toolName, commandHead)
	}
	if pending, err := db.PendingOriginPaths(storage.CurrentOriginVersion, 10); err != nil || len(pending) != 0 {
		t.Fatalf("origin queue after zero-message replay = %v, err=%v", pending, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	beforeReplay = reader.parseCalls
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("converged zero-message replay: %v", err)
	}
	if got := reader.parseCalls - beforeReplay; got != 0 {
		t.Fatalf("converged zero-message path reparsed %d times, want 0", got)
	}
}
