package presets

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInstallUsesExplicitConfigHome(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "explicit")
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(root, "ambient"))

	result, err := Install(Options{ConfigHome: configHome})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !reflect.DeepEqual(result.Installed, presetNames) {
		t.Fatalf("Installed = %v, want %v", result.Installed, presetNames)
	}
	for _, name := range presetNames {
		assertInstalledPreset(t, filepath.Join(configHome, "backscroll", "inputs"), name)
	}
	if _, err := os.Stat(filepath.Join(root, "ambient")); !os.IsNotExist(err) {
		t.Fatalf("Install touched ambient config directory: %v", err)
	}
}

func TestInstallRejectsRelativeConfigHome(t *testing.T) {
	if _, err := Install(Options{ConfigHome: "relative"}); err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("Install relative config home error = %v", err)
	}
}

func TestInstallInitial(t *testing.T) {
	inputsDir := isolatedInputsDir(t)
	if err := os.MkdirAll(inputsDir, 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := Install(Options{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !reflect.DeepEqual(result.Installed, presetNames) {
		t.Fatalf("Installed = %v, want %v", result.Installed, presetNames)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("Skipped = %v, want none", result.Skipped)
	}

	for _, name := range presetNames {
		assertInstalledPreset(t, inputsDir, name)
		info, err := os.Stat(filepath.Join(inputsDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != fileMode {
			t.Errorf("mode for %s = %04o, want %04o", name, got, fileMode)
		}
	}
}

func TestInstallPreservesExistingPreset(t *testing.T) {
	inputsDir := isolatedInputsDir(t)
	if err := os.MkdirAll(inputsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(inputsDir, presetNames[0])
	const custom = "user content\n"
	if err := os.WriteFile(path, []byte(custom), 0o640); err != nil {
		t.Fatal(err)
	}

	result, err := Install(Options{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !reflect.DeepEqual(result.Skipped, []string{presetNames[0]}) {
		t.Fatalf("Skipped = %v, want %v", result.Skipped, []string{presetNames[0]})
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != custom {
		t.Fatalf("existing preset = %q, want %q", got, custom)
	}
}

func TestInstallOverwritesWithExplicitOption(t *testing.T) {
	inputsDir := isolatedInputsDir(t)
	if err := os.MkdirAll(inputsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(inputsDir, presetNames[0])
	if err := os.WriteFile(path, []byte("user content\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	result, err := Install(Options{Overwrite: true})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !reflect.DeepEqual(result.Installed, presetNames) {
		t.Fatalf("Installed = %v, want %v", result.Installed, presetNames)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("Skipped = %v, want none", result.Skipped)
	}
	assertInstalledPreset(t, inputsDir, presetNames[0])
}

func TestInstallCreatesMissingDirectories(t *testing.T) {
	inputsDir := isolatedInputsDir(t)
	if _, err := os.Stat(inputsDir); !os.IsNotExist(err) {
		t.Fatalf("inputs directory exists before install or stat failed: %v", err)
	}

	if _, err := Install(Options{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	info, err := os.Stat(inputsDir)
	if err != nil {
		t.Fatalf("stat inputs directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("inputs path is not a directory: %s", inputsDir)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("inputs directory mode = %04o, want 0700", got)
	}
}

func TestInstallReportsWriteFailure(t *testing.T) {
	inputsDir := isolatedInputsDir(t)
	if err := os.MkdirAll(filepath.Join(inputsDir, presetNames[0]), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := Install(Options{Overwrite: true})
	if err == nil {
		t.Fatal("Install returned nil error")
	}
	if !strings.Contains(err.Error(), "replace preset") {
		t.Fatalf("Install error = %q, want replacement context", err)
	}
	matches, globErr := filepath.Glob(filepath.Join(inputsDir, ".*.inputs.toml-*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain after failure: %v", matches)
	}
}

func TestEmbeddedPresetsMatchShippedInputs(t *testing.T) {
	for _, name := range presetNames {
		embedded, err := presetFiles.ReadFile(filepath.ToSlash(filepath.Join("assets", name)))
		if err != nil {
			t.Fatalf("read embedded %s: %v", name, err)
		}
		shipped, err := os.ReadFile(filepath.Join("..", "..", "inputs", name))
		if err != nil {
			t.Fatalf("read shipped %s: %v", name, err)
		}
		if !reflect.DeepEqual(embedded, shipped) {
			t.Errorf("embedded %s differs from inputs/%s", name, name)
		}
	}
}

func isolatedInputsDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	config := filepath.Join(root, "config")
	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", config)
	return filepath.Join(config, "backscroll", "inputs")
}

func assertInstalledPreset(t *testing.T, inputsDir, name string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(inputsDir, name))
	if err != nil {
		t.Fatalf("read installed %s: %v", name, err)
	}
	want, err := presetFiles.ReadFile(filepath.ToSlash(filepath.Join("assets", name)))
	if err != nil {
		t.Fatalf("read embedded %s: %v", name, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("installed %s differs from embedded preset", name)
	}
}
