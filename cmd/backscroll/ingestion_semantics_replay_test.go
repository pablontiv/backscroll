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

type semanticReplayReader struct {
	name       string
	paths      []string
	hashes     map[string]string
	epoch      uint64
	hashCalls  int
	parseCalls int
}

func (r *semanticReplayReader) Name() string {
	if r.name == "" {
		return "pi"
	}
	return r.name
}
func (r *semanticReplayReader) IngestionEpoch() uint64 {
	return r.epoch
}
func (r *semanticReplayReader) Discover(context.Context, input_config.InputDefinition) ([]string, error) {
	return append([]string(nil), r.paths...), nil
}
func (r *semanticReplayReader) Hash(_ context.Context, path string) (string, error) {
	r.hashCalls++
	return r.hashes[path], nil
}
func (r *semanticReplayReader) Parse(_ context.Context, path string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	r.parseCalls++
	return models.ParsedFile{
		Path: path,
		Hash: r.hashes[path],
		Records: []models.Message{{
			Role:        "assistant",
			Origin:      models.OriginAssistant,
			Content:     "record recovered by decoder epoch",
			UUID:        path + "#recovered",
			ContentType: "text",
		}},
	}, nil
}

func TestIngestionFingerprintReplayCapDrainsConvergesAndLeavesUnavailableRows(t *testing.T) {
	tmpDir := t.TempDir()
	reader := &semanticReplayReader{hashes: make(map[string]string), epoch: 2}
	def := input_config.InputDefinition{
		ID:     "semantic-replay",
		Source: "session",
		Active: true,
		Decode: input_config.DecodeConfig{Format: "pi"},
	}
	oldFingerprint := readers.IngestionFingerprint(&semanticReplayReader{epoch: 1}, def)
	currentFingerprint := readers.IngestionFingerprint(reader, def)

	syncService := newStartupSyncService()
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{def}, input_config.ModeDeclarative, nil
	}
	syncService.newRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(reader)
		return registry
	}

	cfg := config.Config{DatabasePath: filepath.Join(tmpDir, "index.db")}
	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Hour)
	for i := range 201 {
		path := filepath.Join(tmpDir, fmt.Sprintf("empty-%03d.jsonl", i))
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
		stat, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		hash := fmt.Sprintf("raw-%03d", i)
		reader.paths = append(reader.paths, path)
		reader.hashes[path] = hash
		if _, err := tx.Exec(`
			INSERT INTO indexed_files
				(path, hash, last_indexed, file_size, file_mtime, ingestion_fingerprint)
			VALUES (?, ?, '2099-01-01T00:00:00Z', ?, ?, ?)
		`, path, hash, stat.Size(), stat.ModTime().Format(time.RFC3339), oldFingerprint); err != nil {
			t.Fatal(err)
		}
	}
	unavailablePath := filepath.Join(tmpDir, "expired.jsonl")
	if _, err := tx.Exec(`
		INSERT INTO indexed_files (path, hash, last_indexed, ingestion_fingerprint)
		VALUES (?, 'expired-hash', '2020-01-01T00:00:00Z', ?);
		INSERT INTO search_items
			(source, source_path, ordinal, role, text, uuid, content_type)
		VALUES ('session', ?, 0, 'user', 'expired perennial history', 'expired-uuid', 'text')
	`, unavailablePath, oldFingerprint, unavailablePath); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var firstProgress bytes.Buffer
	if err := syncService.sync(context.Background(), &cfg, &firstProgress, startupPhaseTiming{}); err != nil {
		t.Fatalf("first replay: %v", err)
	}
	if reader.parseCalls != 200 {
		t.Fatalf("first replay parsed %d files, want shared 200-file cap", reader.parseCalls)
	}
	if got := bytes.Count(firstProgress.Bytes(), []byte("Re-parsing changed ingestion semantics")); got != 200 {
		t.Fatalf("first replay diagnostics = %d, want 200", got)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	var current, old int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files WHERE ingestion_fingerprint = ?`, currentFingerprint).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files WHERE ingestion_fingerprint = ?`, oldFingerprint).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if current != 200 || old != 2 {
		t.Fatalf("fingerprint counts current/old=%d/%d, want 200/2 including unavailable source", current, old)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if reader.parseCalls != 201 {
		t.Fatalf("second replay total parses = %d, want one remaining available source", reader.parseCalls)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	var lastIndexedBefore string
	if err := db.DB().QueryRow(`SELECT last_indexed FROM indexed_files WHERE path = ?`, reader.paths[0]).Scan(&lastIndexedBefore); err != nil {
		t.Fatal(err)
	}
	var unavailableRows int
	var unavailableFingerprint string
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ?`, unavailablePath).Scan(&unavailableRows); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT ingestion_fingerprint FROM indexed_files WHERE path = ?`, unavailablePath).Scan(&unavailableFingerprint); err != nil {
		t.Fatal(err)
	}
	if unavailableRows != 1 || unavailableFingerprint != oldFingerprint {
		t.Fatalf("unavailable source changed: rows=%d fingerprint=%q", unavailableRows, unavailableFingerprint)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("converged replay: %v", err)
	}
	if reader.parseCalls != 201 {
		t.Fatalf("unchanged fingerprint reparsed sources: total parses=%d want 201", reader.parseCalls)
	}
	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var lastIndexedAfter string
	if err := db.DB().QueryRow(`SELECT last_indexed FROM indexed_files WHERE path = ?`, reader.paths[0]).Scan(&lastIndexedAfter); err != nil {
		t.Fatal(err)
	}
	if lastIndexedAfter != lastIndexedBefore {
		t.Fatalf("unchanged fingerprint advanced last_indexed: before=%q after=%q", lastIndexedBefore, lastIndexedAfter)
	}
}

func TestSourceAndFormatChangesReplayStableBytes(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "stable.jsonl")
	if err := os.WriteFile(sourcePath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	piReader := &semanticReplayReader{
		name: "pi", epoch: 1, paths: []string{sourcePath},
		hashes: map[string]string{sourcePath: "stable"},
	}
	ompReader := &semanticReplayReader{
		name: "omp", epoch: 1, paths: []string{sourcePath},
		hashes: map[string]string{sourcePath: "stable"},
	}
	def := input_config.InputDefinition{ID: "transitions", Source: "session", Active: true, Decode: input_config.DecodeConfig{Format: "pi"}}
	syncService := newStartupSyncService()
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{def}, input_config.ModeDeclarative, nil
	}
	syncService.newRegistry = func() *readers.Registry {
		registry := readers.NewRegistry()
		registry.Register(piReader)
		registry.Register(ompReader)
		return registry
	}
	cfg := config.Config{DatabasePath: filepath.Join(tmpDir, "index.db")}
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatal(err)
	}

	def.Source = "decision"
	var sourceProgress bytes.Buffer
	if err := syncService.sync(context.Background(), &cfg, &sourceProgress, startupPhaseTiming{}); err != nil {
		t.Fatal(err)
	}
	if piReader.parseCalls != 2 || !bytes.Contains(sourceProgress.Bytes(), []byte("Re-parsing changed ingestion semantics")) {
		t.Fatalf("source transition parse count=%d progress=%q", piReader.parseCalls, sourceProgress.String())
	}
	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	var reclassified int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ? AND source = 'decision'`, sourcePath).Scan(&reclassified); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if reclassified != 1 {
		t.Fatalf("source transition reclassified %d rows, want 1", reclassified)
	}

	def.Decode.Format = "omp"
	var formatProgress bytes.Buffer
	if err := syncService.sync(context.Background(), &cfg, &formatProgress, startupPhaseTiming{}); err != nil {
		t.Fatal(err)
	}
	if ompReader.parseCalls != 1 || !bytes.Contains(formatProgress.Bytes(), []byte("Re-parsing changed ingestion semantics")) {
		t.Fatalf("format transition parse count=%d progress=%q", ompReader.parseCalls, formatProgress.String())
	}
}

func TestReasoningToggleReplaysStablePiBytes(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "reasoning.jsonl")
	const fixture = `{"recordType":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"thinking","text":"stable reasoning block"},{"type":"text","text":"stable visible block"}]}}` + "\n"
	if err := os.WriteFile(sourcePath, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(sourcePath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	def := input_config.InputDefinition{
		ID:     "pi-reasoning-toggle",
		Source: "session",
		Active: true,
		Discover: input_config.DiscoverConfig{
			Roots:   []string{tmpDir},
			Include: []string{"*.jsonl"},
		},
		Decode: input_config.DecodeConfig{Format: "pi", IndexReasoning: false},
	}
	syncService := newStartupSyncService()
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{def}, input_config.ModeDeclarative, nil
	}
	cfg := config.Config{DatabasePath: filepath.Join(tmpDir, "index.db")}
	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("initial private sync: %v", err)
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	var reasoningRows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ? AND content_type = 'reasoning'`, sourcePath).Scan(&reasoningRows); err != nil {
		t.Fatal(err)
	}
	if reasoningRows != 0 {
		t.Fatalf("private initial sync stored %d reasoning rows", reasoningRows)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	def.Decode.IndexReasoning = true
	var replayProgress bytes.Buffer
	if err := syncService.sync(context.Background(), &cfg, &replayProgress, startupPhaseTiming{}); err != nil {
		t.Fatalf("reasoning opt-in replay: %v", err)
	}
	if !bytes.Contains(replayProgress.Bytes(), []byte("Re-parsing changed ingestion semantics")) {
		t.Fatalf("reasoning opt-in emitted no semantic replay diagnostic: %q", replayProgress.String())
	}
	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path = ? AND content_type = 'reasoning'`, sourcePath).Scan(&reasoningRows); err != nil {
		t.Fatal(err)
	}
	if reasoningRows != 1 {
		t.Fatalf("reasoning opt-in stored %d rows, want 1", reasoningRows)
	}
}
