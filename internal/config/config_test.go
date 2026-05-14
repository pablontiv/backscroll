package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddingConfig_Defaults(t *testing.T) {
	_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Embedding.Enabled {
		t.Error("Embedding.Enabled default should be false")
	}
	if cfg.Embedding.ModelName != "all-MiniLM-L6-v2" {
		t.Errorf("ModelName = %q, want all-MiniLM-L6-v2", cfg.Embedding.ModelName)
	}
	if cfg.Embedding.SimilarityThreshold != 0.7 {
		t.Errorf("SimilarityThreshold = %v, want 0.7", cfg.Embedding.SimilarityThreshold)
	}
	if cfg.Embedding.TopK != 10 {
		t.Errorf("TopK = %d, want 10", cfg.Embedding.TopK)
	}
}

func TestEmbeddingConfig_LoadFromTOML(t *testing.T) {
	dir := t.TempDir()
	toml := `[embedding]
enabled = true
model_name = "custom-model"
model_path = "/path/to/model.onnx"
similarity_threshold = 0.85
top_k = 20
`
	tomlPath := filepath.Join(dir, "backscroll.toml")
	if err := os.WriteFile(tomlPath, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(orig) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Embedding.Enabled {
		t.Error("Embedding.Enabled should be true")
	}
	if cfg.Embedding.ModelName != "custom-model" {
		t.Errorf("ModelName = %q", cfg.Embedding.ModelName)
	}
	if cfg.Embedding.ModelPath != "/path/to/model.onnx" {
		t.Errorf("ModelPath = %q", cfg.Embedding.ModelPath)
	}
	if cfg.Embedding.SimilarityThreshold != 0.85 {
		t.Errorf("SimilarityThreshold = %v", cfg.Embedding.SimilarityThreshold)
	}
	if cfg.Embedding.TopK != 20 {
		t.Errorf("TopK = %d", cfg.Embedding.TopK)
	}
}

func TestEmbeddingConfig_LegacyTOMLUnaffected(t *testing.T) {
	dir := t.TempDir()
	// No [embedding] section
	toml := `database_path = "/tmp/test.db"
`
	tomlPath := filepath.Join(dir, "backscroll.toml")
	if err := os.WriteFile(tomlPath, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(orig) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Embedding.Enabled {
		t.Error("Embedding.Enabled should default to false when not in TOML")
	}
	if cfg.Embedding.TopK != 10 {
		t.Errorf("TopK should default to 10, got %d", cfg.Embedding.TopK)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Save current env vars
	oldDbPath := os.Getenv("BACKSCROLL_DATABASE_PATH")
	oldSessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	defer func() {
		if oldDbPath != "" {
			_ = os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			_ = os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	// Clear env vars
	_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Change to temp dir with no backscroll.toml
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check DatabasePath defaults to ~/.backscroll.db
	expectedDbPath := filepath.Join(homeDir(), ".backscroll.db")
	if cfg.DatabasePath != expectedDbPath {
		t.Errorf("expected DatabasePath %s, got %s", expectedDbPath, cfg.DatabasePath)
	}

	// Check SessionDirs defaults to ["."]
	if len(cfg.SessionDirs) != 1 || cfg.SessionDirs[0] != "." {
		t.Errorf("expected SessionDirs ['.'], got %v", cfg.SessionDirs)
	}
}

func TestEnvVarDatabasePath(t *testing.T) {
	// Save current env vars
	oldDbPath := os.Getenv("BACKSCROLL_DATABASE_PATH")
	oldSessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	defer func() {
		if oldDbPath != "" {
			_ = os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			_ = os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	// Set env var
	_ = os.Setenv("BACKSCROLL_DATABASE_PATH", "/tmp/test.db")
	_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Change to temp dir with no backscroll.toml
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabasePath != "/tmp/test.db" {
		t.Errorf("expected DatabasePath /tmp/test.db, got %s", cfg.DatabasePath)
	}
}

func TestEnvVarSessionDirs(t *testing.T) {
	// Save current env vars
	oldDbPath := os.Getenv("BACKSCROLL_DATABASE_PATH")
	oldSessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	defer func() {
		if oldDbPath != "" {
			_ = os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			_ = os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	// Set env var (colon-separated)
	_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	_ = os.Setenv("BACKSCROLL_SESSION_DIRS", "/path/one:/path/two:/path/three")

	// Change to temp dir with no backscroll.toml
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedDirs := []string{"/path/one", "/path/two", "/path/three"}
	if len(cfg.SessionDirs) != len(expectedDirs) {
		t.Errorf("expected %d SessionDirs, got %d", len(expectedDirs), len(cfg.SessionDirs))
	}
	for i, dir := range cfg.SessionDirs {
		if dir != expectedDirs[i] {
			t.Errorf("expected SessionDirs[%d] %s, got %s", i, expectedDirs[i], dir)
		}
	}
}

func TestLocalConfigOverridesGlobal(t *testing.T) {
	// Save current env vars
	oldDbPath := os.Getenv("BACKSCROLL_DATABASE_PATH")
	oldSessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	defer func() {
		if oldDbPath != "" {
			_ = os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			_ = os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Create temp dir
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	// Create local backscroll.toml
	localConfig := `database_path = "/local/test.db"
session_dirs = ["/local/dir1", "/local/dir2"]
`
	if err := os.WriteFile("backscroll.toml", []byte(localConfig), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabasePath != "/local/test.db" {
		t.Errorf("expected DatabasePath /local/test.db, got %s", cfg.DatabasePath)
	}

	expectedDirs := []string{"/local/dir1", "/local/dir2"}
	if len(cfg.SessionDirs) != len(expectedDirs) {
		t.Errorf("expected %d SessionDirs, got %d", len(expectedDirs), len(cfg.SessionDirs))
	}
	for i, dir := range cfg.SessionDirs {
		if dir != expectedDirs[i] {
			t.Errorf("expected SessionDirs[%d] %s, got %s", i, expectedDirs[i], dir)
		}
	}
}

func TestSessionDirSingularAlias(t *testing.T) {
	// Save current env vars
	oldDbPath := os.Getenv("BACKSCROLL_DATABASE_PATH")
	oldSessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	defer func() {
		if oldDbPath != "" {
			_ = os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			_ = os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Create temp dir
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	// Create local backscroll.toml with session_dir (singular)
	localConfig := `database_path = "/local/test.db"
session_dir = "/single/dir"
`
	if err := os.WriteFile("backscroll.toml", []byte(localConfig), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.SessionDirs) != 1 || cfg.SessionDirs[0] != "/single/dir" {
		t.Errorf("expected SessionDirs ['/single/dir'], got %v", cfg.SessionDirs)
	}
}

func TestSessionDirsArray(t *testing.T) {
	// Save current env vars
	oldDbPath := os.Getenv("BACKSCROLL_DATABASE_PATH")
	oldSessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	defer func() {
		if oldDbPath != "" {
			_ = os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			_ = os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Create temp dir
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	// Create local backscroll.toml with session_dirs (array)
	localConfig := `database_path = "/local/test.db"
session_dirs = ["/dir1", "/dir2", "/dir3"]
`
	if err := os.WriteFile("backscroll.toml", []byte(localConfig), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedDirs := []string{"/dir1", "/dir2", "/dir3"}
	if len(cfg.SessionDirs) != len(expectedDirs) {
		t.Errorf("expected %d SessionDirs, got %d", len(expectedDirs), len(cfg.SessionDirs))
	}
	for i, dir := range cfg.SessionDirs {
		if dir != expectedDirs[i] {
			t.Errorf("expected SessionDirs[%d] %s, got %s", i, expectedDirs[i], dir)
		}
	}
}

func TestDiscoverSessionDirs(t *testing.T) {
	// Test discovery doesn't error even if directory doesn't exist
	_, err := DiscoverSessionDirs()
	if err != nil {
		t.Errorf("DiscoverSessionDirs unexpectedly errored: %v", err)
	}
}

func TestDiscoverSessionDirsWithEntries(t *testing.T) {
	// Set up a fake HOME with .claude/projects/ subdirectories
	tmpHome := t.TempDir()
	projectsDir := filepath.Join(tmpHome, ".claude", "projects")
	if err := os.MkdirAll(filepath.Join(projectsDir, "proj-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectsDir, "proj-b"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Also create a file (should be skipped)
	if err := os.WriteFile(filepath.Join(projectsDir, "not-a-dir.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	dirs, err := DiscoverSessionDirs()
	if err != nil {
		t.Fatalf("DiscoverSessionDirs: %v", err)
	}
	if len(dirs) != 2 {
		t.Errorf("expected 2 dirs, got %d: %v", len(dirs), dirs)
	}
}

func TestEnvVarSessionDirsMultiple(t *testing.T) {
	oldDbPath := os.Getenv("BACKSCROLL_DATABASE_PATH")
	oldSessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	defer func() {
		if oldDbPath != "" {
			_ = os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			_ = os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	_ = os.Setenv("BACKSCROLL_DATABASE_PATH", "/tmp/test.db")
	_ = os.Setenv("BACKSCROLL_SESSION_DIRS", "/dir/a:/dir/b:/dir/c")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.SessionDirs) != 3 {
		t.Errorf("expected 3 session dirs, got %d: %v", len(cfg.SessionDirs), cfg.SessionDirs)
	}
	if !cfg.SessionDirsExplicit {
		t.Error("expected SessionDirsExplicit to be true")
	}
}

func TestLoadConfigFileWithSources(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.toml")

	content := `
database_path = "/tmp/mydb.db"
session_dirs = ["/sessions"]

[sources]
ke = ["/sources/ke.md"]
decisions = ["/sources/decisions.md"]
memories = ["/sources/memories.md"]
rules = ["/sources/rules.md"]
specs = ["/sources/specs.md"]
backlog = ["/sources/backlog.md"]
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := &Config{DatabasePath: "/default.db", SessionDirs: []string{"."}}
	if err := loadConfigFile(cfgPath, cfg); err != nil {
		t.Fatalf("loadConfigFile: %v", err)
	}

	if cfg.DatabasePath != "/tmp/mydb.db" {
		t.Errorf("DatabasePath = %q, want /tmp/mydb.db", cfg.DatabasePath)
	}
	if len(cfg.Sources.KE) != 1 || cfg.Sources.KE[0] != "/sources/ke.md" {
		t.Errorf("Sources.KE = %v", cfg.Sources.KE)
	}
	if len(cfg.Sources.Decisions) != 1 {
		t.Errorf("Sources.Decisions = %v", cfg.Sources.Decisions)
	}
	if len(cfg.Sources.Memories) != 1 {
		t.Errorf("Sources.Memories = %v", cfg.Sources.Memories)
	}
	if len(cfg.Sources.Rules) != 1 {
		t.Errorf("Sources.Rules = %v", cfg.Sources.Rules)
	}
	if len(cfg.Sources.Specs) != 1 {
		t.Errorf("Sources.Specs = %v", cfg.Sources.Specs)
	}
	if len(cfg.Sources.Backlog) != 1 {
		t.Errorf("Sources.Backlog = %v", cfg.Sources.Backlog)
	}
}

func TestLoadConfigFileInvalidTOML(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "bad.toml")
	if err := os.WriteFile(cfgPath, []byte("not valid toml ==="), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{DatabasePath: "/default.db"}
	err := loadConfigFile(cfgPath, cfg)
	if err == nil {
		t.Error("expected error for invalid TOML, got nil")
	}
}

func TestLoadConfigFileSessionDirSingular(t *testing.T) {
	tmpdir := t.TempDir()
	cfgPath := filepath.Join(tmpdir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(`session_dir = "/single/dir"`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{DatabasePath: "/default.db", SessionDirs: []string{"."}}
	if err := loadConfigFile(cfgPath, cfg); err != nil {
		t.Fatalf("loadConfigFile: %v", err)
	}
	if len(cfg.SessionDirs) != 1 || cfg.SessionDirs[0] != "/single/dir" {
		t.Errorf("SessionDirs = %v", cfg.SessionDirs)
	}
	if !cfg.SessionDirsExplicit {
		t.Error("expected SessionDirsExplicit to be true")
	}
}
