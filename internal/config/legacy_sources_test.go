package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateNoLegacySourcesReportsAllConfigFiles(t *testing.T) {
	home := setupLegacySourcesTest(t)
	globalPath := globalConfigPath()
	writeFile(t, globalPath, `[sources]
specs = ["docs/specs"]
ke = ["docs/ke"]
backlog = ["docs/backlog"]
rules = ["docs/rules"]
memories = ["docs/memories"]
decisions = ["docs/decisions"]
`)
	writeFile(t, "backscroll.toml", `[sources]
ke = ["local/ke"]
`)
	inputsDir := filepath.Join(home, ".config", "backscroll", "inputs")

	err := ValidateNoLegacySources(inputsDir)
	if err == nil {
		t.Fatal("ValidateNoLegacySources returned nil error")
	}

	var legacyErr *LegacySourcesError
	if !errors.As(err, &legacyErr) {
		t.Fatalf("error type = %T, want *LegacySourcesError", err)
	}
	if legacyErr.InputsDir != inputsDir {
		t.Fatalf("InputsDir = %q, want %q", legacyErr.InputsDir, inputsDir)
	}

	wantFiles := []LegacySourcesFile{
		{Path: globalPath, Keys: []string{"backlog", "decisions", "ke", "memories", "rules", "specs"}},
		{Path: "./backscroll.toml", Keys: []string{"ke"}},
	}
	if !reflect.DeepEqual(legacyErr.Files, wantFiles) {
		t.Fatalf("Files = %#v, want %#v", legacyErr.Files, wantFiles)
	}

	msg := err.Error()
	for _, want := range []string{inputsDir, "markdown_document", "markdown_sections"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Error() = %q, want it to contain %q", msg, want)
		}
	}
}

func TestValidateNoLegacySourcesRejectsEmptyTable(t *testing.T) {
	setupLegacySourcesTest(t)
	writeFile(t, "backscroll.toml", `[sources]
`)

	err := ValidateNoLegacySources("/tmp/backscroll-inputs")
	if err == nil {
		t.Fatal("ValidateNoLegacySources returned nil error")
	}

	var legacyErr *LegacySourcesError
	if !errors.As(err, &legacyErr) {
		t.Fatalf("error type = %T, want *LegacySourcesError", err)
	}
	wantFiles := []LegacySourcesFile{{Path: "./backscroll.toml", Keys: nil}}
	if !reflect.DeepEqual(legacyErr.Files, wantFiles) {
		t.Fatalf("Files = %#v, want %#v", legacyErr.Files, wantFiles)
	}
	if !strings.Contains(err.Error(), "keys: none") {
		t.Fatalf("Error() = %q, want it to contain %q", err.Error(), "keys: none")
	}
}

func TestValidateNoLegacySourcesIgnoresMissingFiles(t *testing.T) {
	setupLegacySourcesTest(t)

	if err := ValidateNoLegacySources("/tmp/backscroll-inputs"); err != nil {
		t.Fatalf("ValidateNoLegacySources returned error for missing files: %v", err)
	}
}

func TestValidateNoLegacySourcesReportsMalformedTOML(t *testing.T) {
	setupLegacySourcesTest(t)
	writeFile(t, "backscroll.toml", `[sources]
ke = ["unterminated"
`)

	err := ValidateNoLegacySources("/tmp/backscroll-inputs")
	if err == nil {
		t.Fatal("ValidateNoLegacySources returned nil error")
	}
	if !strings.Contains(err.Error(), "./backscroll.toml") {
		t.Fatalf("Error() = %q, want it to contain malformed TOML path", err.Error())
	}
}

func setupLegacySourcesTest(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	wd := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(home, ".config", "backscroll"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", filepath.Join(home, "backscroll.db"))
	t.Chdir(wd)
	return home
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
