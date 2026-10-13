package config

import (
	"errors"
	"testing"
)

func TestLoadRejectsEmptyLegacySourcesFromLoadedBytes(t *testing.T) {
	setupLegacySourcesTest(t)
	writeFile(t, "backscroll.toml", "[sources]\n")
	_, err := Load()
	var legacy *LegacySourcesError
	if !errors.As(err, &legacy) {
		t.Fatalf("Load error = %v, want legacy sources rejection", err)
	}
}

func TestLoadSnapshotPrecedenceCollision(t *testing.T) {
	setupLegacySourcesTest(t)
	t.Setenv("BACKSCROLL_DATABASE_PATH", "env.db")
	t.Setenv("BACKSCROLL_SESSION_DIRS", "env-a:env-b")
	writeFile(t, globalConfigPath(), "database_path = \"global.db\"\nsession_dirs = [\"global\"]\n")
	writeFile(t, "backscroll.toml", "database_path = \"local.db\"\nsession_dir = \"local\"\n")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabasePath != "env.db" || len(cfg.SessionDirs) != 2 || cfg.SessionDirs[0] != "env-a" {
		t.Fatalf("env precedence: %+v", cfg)
	}
	t.Setenv("BACKSCROLL_DATABASE_PATH", "")
	t.Setenv("BACKSCROLL_SESSION_DIRS", "")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabasePath != "local.db" || len(cfg.SessionDirs) != 1 || cfg.SessionDirs[0] != "local" {
		t.Fatalf("local precedence: %+v", cfg)
	}
}
