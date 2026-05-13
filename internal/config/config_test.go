package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Save current env vars
	oldDbPath := os.Getenv("BACKSCROLL_DATABASE_PATH")
	oldSessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	defer func() {
		if oldDbPath != "" {
			os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	// Clear env vars
	os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Change to temp dir with no backscroll.toml
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

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
			os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	// Set env var
	os.Setenv("BACKSCROLL_DATABASE_PATH", "/tmp/test.db")
	os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Change to temp dir with no backscroll.toml
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

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
			os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	// Set env var (colon-separated)
	os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	os.Setenv("BACKSCROLL_SESSION_DIRS", "/path/one:/path/two:/path/three")

	// Change to temp dir with no backscroll.toml
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

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
			os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Create temp dir
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

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
			os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Create temp dir
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

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
			os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	os.Unsetenv("BACKSCROLL_DATABASE_PATH")
	os.Unsetenv("BACKSCROLL_SESSION_DIRS")

	// Create temp dir
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)

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

func TestEnvVarSessionDirsMultiple(t *testing.T) {
	oldDbPath := os.Getenv("BACKSCROLL_DATABASE_PATH")
	oldSessionDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	defer func() {
		if oldDbPath != "" {
			os.Setenv("BACKSCROLL_DATABASE_PATH", oldDbPath)
		} else {
			os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		}
		if oldSessionDirs != "" {
			os.Setenv("BACKSCROLL_SESSION_DIRS", oldSessionDirs)
		} else {
			os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		}
	}()

	os.Setenv("BACKSCROLL_DATABASE_PATH", "/tmp/test.db")
	os.Setenv("BACKSCROLL_SESSION_DIRS", "/dir/a:/dir/b:/dir/c")

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
