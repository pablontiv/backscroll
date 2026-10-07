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

type emptyPiReplayReader struct {
	paths      []string
	hashes     map[string]string
	hashCalls  int
	parseCalls int
}

func (*emptyPiReplayReader) Name() string { return "pi" }

func (r *emptyPiReplayReader) Discover(context.Context, input_config.InputDefinition) ([]string, error) {
	return append([]string(nil), r.paths...), nil
}

func (r *emptyPiReplayReader) Hash(_ context.Context, path string) (string, error) {
	r.hashCalls++
	return r.hashes[path], nil
}

func (r *emptyPiReplayReader) Parse(_ context.Context, path string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	r.parseCalls++
	return models.ParsedFile{Path: path, Hash: r.hashes[path]}, nil
}

func TestMarkedEmptyPiRacyCleanComparesCanonicalContentHash(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.jsonl")
	initialContent := []byte("aaaa")
	if err := os.WriteFile(path, initialContent, 0o600); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	initialMtime := stat.ModTime()
	initialHash := "raw-initial"
	reader := &emptyPiReplayReader{
		paths:  []string{path},
		hashes: map[string]string{path: initialHash},
	}

	syncService := newStartupSyncService()
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{
			ID:     "marked-empty-pi",
			Source: "session",
			Active: true,
			Decode: input_config.DecodeConfig{Format: "pi"},
		}}, input_config.ModeDeclarative, nil
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
	fileMtime := initialMtime.Format(time.RFC3339)
	if !isRacyCleanFile(fileMtime, fileMtime) {
		t.Fatal("test setup error: matching file_mtime and last_indexed must force racy-clean hashing")
	}
	if _, err := db.DB().Exec(`
		INSERT INTO indexed_files (path, hash, last_indexed, file_size, file_mtime)
		VALUES (?, ?, ?, ?, ?)
	`, path, emptyPiHashPrefix+initialHash, fileMtime, stat.Size(), fileMtime); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("immutable racy-clean sync: %v", err)
	}
	if reader.hashCalls != 1 || reader.parseCalls != 0 {
		t.Fatalf("immutable racy-clean calls = hash %d parse %d, want 1/0", reader.hashCalls, reader.parseCalls)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	var persistedHash string
	if err := db.DB().QueryRow(`SELECT hash FROM indexed_files WHERE path = ?`, path).Scan(&persistedHash); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if want := emptyPiHashPrefix + initialHash; persistedHash != want {
		t.Fatalf("immutable skip changed persisted marker to %q, want %q", persistedHash, want)
	}

	changedContent := []byte("bbbb")
	if len(changedContent) != len(initialContent) {
		t.Fatal("test setup error: changed content must retain file size")
	}
	if err := os.WriteFile(path, changedContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, initialMtime, initialMtime); err != nil {
		t.Fatal(err)
	}
	reader.hashes[path] = "raw-changed"

	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("changed racy-clean sync: %v", err)
	}
	if reader.hashCalls != 2 || reader.parseCalls != 1 {
		t.Fatalf("changed racy-clean calls = hash %d parse %d, want 2/1", reader.hashCalls, reader.parseCalls)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.DB().QueryRow(`SELECT hash FROM indexed_files WHERE path = ?`, path).Scan(&persistedHash); err != nil {
		t.Fatal(err)
	}
	if want := emptyPiHashPrefix + reader.hashes[path]; persistedHash != want {
		t.Fatalf("changed parse persisted hash = %q, want %q", persistedHash, want)
	}
}

func TestEmptyPiReplayCapDrainsAndConverges(t *testing.T) {
	tmpDir := t.TempDir()
	reader := &emptyPiReplayReader{hashes: make(map[string]string)}
	oldTime := time.Now().Add(-time.Hour)

	syncService := newStartupSyncService()
	syncService.activeInputs = func([]string) ([]input_config.InputDefinition, input_config.InputMode, error) {
		return []input_config.InputDefinition{{
			ID:     "empty-pi-replay",
			Source: "session",
			Active: true,
			Decode: input_config.DecodeConfig{Format: "pi"},
		}}, input_config.ModeDeclarative, nil
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
	for i := 0; i < 201; i++ {
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
			INSERT INTO indexed_files (path, hash, last_indexed, file_size, file_mtime)
			VALUES (?, ?, '2099-01-01T00:00:00Z', ?, ?)
		`, path, hash, stat.Size(), stat.ModTime().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("first replay: %v", err)
	}
	if reader.parseCalls != 200 {
		t.Fatalf("first replay parsed %d files, want 200-file cap", reader.parseCalls)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	countHashes := func(pattern string) int {
		t.Helper()
		var count int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files WHERE hash LIKE ?`, pattern).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	if got := countHashes(emptyPiHashPrefix + "%"); got != 200 {
		t.Fatalf("marked hashes after first replay = %d, want 200", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if reader.parseCalls != 201 {
		t.Fatalf("second replay total parses = %d, want one remaining file", reader.parseCalls)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := countHashes(emptyPiHashPrefix + "%"); got != 201 {
		t.Fatalf("marked hashes after drain = %d, want 201", got)
	}
	var lastIndexedBefore string
	if err := db.DB().QueryRow(`SELECT last_indexed FROM indexed_files WHERE path = ?`, reader.paths[0]).Scan(&lastIndexedBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := syncService.sync(context.Background(), &cfg, &bytes.Buffer{}, startupPhaseTiming{}); err != nil {
		t.Fatalf("convergence replay: %v", err)
	}
	if reader.parseCalls != 201 {
		t.Fatalf("convergence replay parsed marked files: total parses = %d, want 201", reader.parseCalls)
	}

	db, err = storage.Open(cfg.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var hashAfter, lastIndexedAfter string
	if err := db.DB().QueryRow(`SELECT hash, last_indexed FROM indexed_files WHERE path = ?`, reader.paths[0]).Scan(&hashAfter, &lastIndexedAfter); err != nil {
		t.Fatal(err)
	}
	if hashAfter != emptyPiHashPrefix+reader.hashes[reader.paths[0]] {
		t.Fatalf("persisted marker = %q, want %q", hashAfter, emptyPiHashPrefix+reader.hashes[reader.paths[0]])
	}
	if lastIndexedAfter != lastIndexedBefore {
		t.Fatalf("last_indexed changed after converged replay: before %q after %q", lastIndexedBefore, lastIndexedAfter)
	}
}
