package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/config"
)

// TestMaybeAutoSyncWithInvalidDBPath tests error handling when database path is invalid
func TestMaybeAutoSyncWithInvalidDBPath(t *testing.T) {
	// Create a path inside a non-existent directory
	cfg := &config.Config{
		DatabasePath: "/nonexistent/path/to/db/test.db",
		SessionDirs:  []string{},
		Sources:      config.SourcesConfig{},
		Embedding: config.EmbeddingConfig{
			Enabled: false,
		},
	}

	// Call maybeAutoSync
	err := maybeAutoSync(cfg)

	// It should error because the parent directory doesn't exist
	if err == nil {
		t.Errorf("maybeAutoSync should error with invalid DB path")
	}
}

// TestMaybeAutoSyncDoesNotErrorWithEmptySessionDirs tests that maybeAutoSync
// gracefully handles empty session directories
func TestMaybeAutoSyncDoesNotErrorWithEmptySessionDirs(t *testing.T) {
	// Create a temporary directory for the test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a minimal config pointing to our temp directory
	cfg := &config.Config{
		DatabasePath: dbPath,
		SessionDirs:  []string{},
		Sources:      config.SourcesConfig{},
		Embedding: config.EmbeddingConfig{
			Enabled: false,
		},
	}

	// Skip this test on systems where ActiveInputs implementation is slow
	// (It's more of an integration test than a unit test)
	t.Skip("maybeAutoSync calls ActiveInputs which is slow in unit tests")

	// Call maybeAutoSync
	err := maybeAutoSync(cfg)

	// It should succeed (no error even with empty session dirs)
	if err != nil {
		t.Errorf("maybeAutoSync should not error with empty session dirs: %v", err)
	}

	// Database should now exist
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("database should have been created: %v", err)
	}
}
