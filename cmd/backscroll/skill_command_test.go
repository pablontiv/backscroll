package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/skillinstall"
)

func TestSkillCommandMatchesAliasAndBypassesStartup(t *testing.T) {
	rootDir := t.TempDir()
	badConfig := filepath.Join(rootDir, "config-is-a-file")
	if err := os.WriteFile(badConfig, []byte("invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(rootDir, "missing-home"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", badConfig)
	t.Setenv("BACKSCROLL_DATABASE_PATH", filepath.Join(rootDir, "missing-db", "index.db"))

	var stdout, stderr bytes.Buffer
	startupCalls := 0
	root := buildRootCmdWithDependencies(&stdout, &stderr, func(context.Context, io.Writer, startupCommandClass) startupResult {
		startupCalls++
		return startupResult{Failure: &startupFailure{Cause: errors.New("startup must not run")}}
	}, nil, nil)
	applyCalls := 0
	fetchCalls := 0
	err := runContextWithDependencies(context.Background(), &stdout, &stderr, []string{"skill"}, runDependencies{
		root: root,
		applyStaged: func() error {
			applyCalls++
			return nil
		},
		fetchAndStage: func(context.Context, string) error {
			fetchCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("skill: %v", err)
	}
	var aliasStdout, aliasStderr bytes.Buffer
	if err := run(&aliasStdout, &aliasStderr, []string{"--skill"}); err != nil {
		t.Fatalf("--skill: %v", err)
	}
	if !bytes.Equal(stdout.Bytes(), aliasStdout.Bytes()) {
		t.Fatal("skill stdout differs from the compatibility alias")
	}
	if stderr.Len() != 0 || aliasStderr.Len() != 0 {
		t.Fatalf("skill stderr = %q and alias stderr = %q, want both empty", stderr.String(), aliasStderr.String())
	}
	if startupCalls != 0 || applyCalls != 0 || fetchCalls != 0 {
		t.Fatalf("skill calls: startup=%d apply=%d fetch=%d", startupCalls, applyCalls, fetchCalls)
	}
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(badConfig) {
		t.Fatalf("skill created side effects: %v", entryNames(entries))
	}
}

func TestSkillInvalidArgumentsHaveNoEffects(t *testing.T) {
	rootDir := t.TempDir()
	home := filepath.Join(rootDir, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(rootDir, "invalid-config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", filepath.Join(rootDir, "database", "index.db"))

	tests := [][]string{
		{"skill", "extra"},
		{"skill", "--home", home},
		{"skill", "--approve", strings.Repeat("a", 64)},
		{"skill", "--install", "--restore", filepath.Join(rootDir, "receipt.json"), "--approve", strings.Repeat("a", 64)},
		{"skill", "--restore", filepath.Join(rootDir, "receipt.json")},
		{"skill", "--restore", filepath.Join(rootDir, "receipt.json"), "--approve", strings.Repeat("a", 64), "--config-home", filepath.Join(rootDir, "config")},
		{"skill", "--install", "--home", "relative"},
		{"skill", "--install", "--home", home, "--config-home", "relative"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := run(&stdout, &stderr, args); err == nil {
				t.Fatalf("run(%v) succeeded", args)
			}
			if stdout.Len() != 0 {
				t.Fatalf("run(%v) stdout = %q", args, stdout.String())
			}
			entries, err := os.ReadDir(rootDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "home" {
				t.Fatalf("run(%v) created side effects: %v", args, entryNames(entries))
			}
		})
	}
}

func TestSkillRestoreOutsideStateHasNoEffects(t *testing.T) {
	rootDir := t.TempDir()
	home := filepath.Join(rootDir, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(rootDir, "config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", filepath.Join(rootDir, "database", "index.db"))

	before, err := directoryTree(t, rootDir)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := realMain(&stdout, &stderr, []string{
		"skill", "--restore", filepath.Join(rootDir, "outside", "receipt.json"),
		"--approve", strings.Repeat("a", 64), "--home", home,
	})
	if code != 1 {
		t.Fatalf("skill --restore exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("skill --restore stdout = %q, want empty", stdout.String())
	}
	const wantError = "receipt must belong to this home's installation state\n"
	if stderr.String() != wantError {
		t.Fatalf("skill --restore stderr = %q, want %q", stderr.String(), wantError)
	}
	after, err := directoryTree(t, rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("skill --restore created filesystem entries: before=%v after=%v", before, after)
	}
}

func TestSkillInstallIdempotenceAndRestore(t *testing.T) {
	rootDir := t.TempDir()
	t.Cleanup(func() { makeTreeRemovable(rootDir) })
	home := filepath.Join(rootDir, "home")
	configHome := filepath.Join(rootDir, "config")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(rootDir, "ambient-config-must-not-be-used"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(rootDir, "invalid-product-config"))
	database := filepath.Join(rootDir, "database", "index.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", database)

	stdout, stderr, err := runCmd("skill", "--install", "--home", home, "--config-home", configHome)
	if err != nil {
		t.Fatalf("skill --install: %v; stderr=%q", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("skill --install stderr = %q", stderr)
	}
	var installed struct {
		Changed         int     `json:"changed"`
		Receipt         *string `json:"receipt"`
		RestoreApproval *string `json:"restore_approval"`
	}
	if err := json.Unmarshal([]byte(stdout), &installed); err != nil {
		t.Fatalf("decode install result: %v; stdout=%q", err, stdout)
	}
	if installed.Changed != 3 || installed.Receipt == nil || installed.RestoreApproval == nil {
		t.Fatalf("install result = %+v", installed)
	}

	targets := []string{
		filepath.Join(home, ".claude/skills/backscroll"),
		filepath.Join(home, ".agents/skills/backscroll"),
		filepath.Join(configHome, "opencode/skills/backscroll"),
	}
	var managed string
	for _, target := range targets {
		link, err := os.Readlink(target)
		if err != nil {
			t.Fatalf("read installed link %s: %v", target, err)
		}
		if !filepath.IsAbs(link) {
			t.Fatalf("installed link is not absolute: %s", link)
		}
		if managed == "" {
			managed = link
		} else if link != managed {
			t.Fatalf("installed links differ: %s and %s", managed, link)
		}
	}
	shim, err := skillinstall.OfficialShim()
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range shim {
		got, err := os.ReadFile(filepath.Join(managed, name))
		if err != nil {
			t.Fatalf("read managed shim %s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("managed shim %s differs from embedded content", name)
		}
	}
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		t.Fatalf("skill install touched database: %v", err)
	}

	stdout, stderr, err = runCmd("skill", "--install", "--home", home, "--config-home", configHome)
	if err != nil {
		t.Fatalf("second skill --install: %v; stderr=%q", err, stderr)
	}
	var unchanged struct {
		Changed         int `json:"changed"`
		Receipt         any `json:"receipt"`
		RestoreApproval any `json:"restore_approval"`
	}
	if err := json.Unmarshal([]byte(stdout), &unchanged); err != nil {
		t.Fatal(err)
	}
	if unchanged.Changed != 0 || unchanged.Receipt != nil || unchanged.RestoreApproval != nil {
		t.Fatalf("idempotent result = %+v", unchanged)
	}

	stdout, stderr, err = runCmd("skill", "--restore", *installed.Receipt, "--approve", *installed.RestoreApproval, "--home", home)
	if err != nil {
		t.Fatalf("skill --restore: %v; stderr=%q", err, stderr)
	}
	var restored skillinstall.RestoreResult
	if err := json.Unmarshal([]byte(stdout), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Restored != 3 {
		t.Fatalf("restore result = %+v", restored)
	}
	for _, target := range targets {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("restore left target %s: %v", target, err)
		}
	}

	stdout, stderr, err = runCmd("skill", "--restore", *installed.Receipt, "--approve", *installed.RestoreApproval, "--home", home)
	if err != nil {
		t.Fatalf("second skill --restore: %v; stderr=%q", err, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Restored != 0 {
		t.Fatalf("second restore result = %+v", restored)
	}
}

func TestSkillInstallApprovalRequestDoesNotMutate(t *testing.T) {
	rootDir := t.TempDir()
	home := filepath.Join(rootDir, "home")
	configHome := filepath.Join(rootDir, "config")
	target := filepath.Join(home, ".claude/skills/backscroll")
	if err := os.MkdirAll(target, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "operator-note"), []byte("keep\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	before, err := directoryTree(t, rootDir)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runCmd("skill", "--install", "--home", home, "--config-home", configHome)
	if err != nil {
		t.Fatalf("approval inventory: %v; stderr=%q", err, stderr)
	}
	var response struct {
		ApprovalRequired bool              `json:"approval_required"`
		Plan             skillinstall.Plan `json:"plan"`
	}
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("decode approval inventory: %v", err)
	}
	if !response.ApprovalRequired || response.Plan.Approval == "" {
		t.Fatalf("approval response = %+v", response)
	}
	after, err := directoryTree(t, rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("approval inventory changed files: before=%v after=%v", before, after)
	}

	_, _, err = runCmd("skill", "--install", "--home", home, "--config-home", configHome, "--approve", strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "approval mismatch") {
		t.Fatalf("incorrect approval error = %v", err)
	}
	after, err = directoryTree(t, rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("incorrect approval changed files: before=%v after=%v", before, after)
	}
}

func TestPresetsInstallUsesExplicitConfigHome(t *testing.T) {
	rootDir := t.TempDir()
	home := filepath.Join(rootDir, "missing-home")
	configHome := filepath.Join(rootDir, "config")
	ambient := filepath.Join(rootDir, "ambient")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", ambient)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(rootDir, "product-config"))

	stdout, stderr, err := runCmd("presets", "install", "--config-home", configHome)
	if err != nil {
		t.Fatalf("presets install: %v; stderr=%q", err, stderr)
	}
	var result struct {
		Installed []string `json:"installed"`
		Skipped   []string `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Installed) != 5 || len(result.Skipped) != 0 {
		t.Fatalf("presets result = %+v", result)
	}
	inputs := filepath.Join(configHome, "backscroll", "inputs")
	customPath := filepath.Join(inputs, result.Installed[0])
	if err := os.WriteFile(customPath, []byte("operator content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmd("presets", "install", "--config-home", configHome); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "operator content\n" {
		t.Fatal("presets install replaced an existing file without --overwrite")
	}
	if _, _, err := runCmd("presets", "install", "--config-home", configHome, "--overwrite"); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(customPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) == "operator content\n" {
		t.Fatal("presets install --overwrite preserved an existing managed preset")
	}
	if _, err := os.Stat(ambient); !os.IsNotExist(err) {
		t.Fatalf("presets install touched ambient config home: %v", err)
	}
}

func TestInstalledSkillDelegatesToPayloadAndRetrievesFixture(t *testing.T) {
	buildDir := t.TempDir()
	binary := filepath.Join(buildDir, "backscroll")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build dev binary: %v\n%s", err, output)
	}

	root := t.TempDir()
	t.Cleanup(func() { makeTreeRemovable(root) })
	home := filepath.Join(root, "home")
	configHome := filepath.Join(root, "config")
	work := filepath.Join(root, "application")
	sessions := filepath.Join(root, "sessions")
	for _, path := range []string{home, work, sessions} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	environment := environmentWithOverrides(os.Environ(), map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": configHome,
		"XDG_CACHE_HOME":           filepath.Join(root, "cache"),
		"XDG_DATA_HOME":            filepath.Join(root, "data"),
		"XDG_STATE_HOME":           filepath.Join(root, "state"),
		"BACKSCROLL_CONFIG_DIR":    configHome,
		"BACKSCROLL_DATABASE_PATH": filepath.Join(root, "index.db"),
		"BACKSCROLL_SESSION_DIRS":  sessions,
	})
	runBinary := func(directory string, args ...string) (string, string, error) {
		command := exec.Command(binary, args...)
		command.Dir = directory
		command.Env = environment
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		err := command.Run()
		return stdout.String(), stderr.String(), err
	}

	stdout, stderr, err := runBinary(work, "skill", "--install", "--home", home, "--config-home", configHome)
	if err != nil || stderr != "" {
		t.Fatalf("skill install error = %v, stderr = %q, stdout = %q", err, stderr, stdout)
	}
	var install struct {
		Changed int `json:"changed"`
	}
	if err := json.Unmarshal([]byte(stdout), &install); err != nil || install.Changed != 3 {
		t.Fatalf("skill install result = %q, error = %v", stdout, err)
	}
	targets := []string{
		filepath.Join(home, ".claude/skills/backscroll"),
		filepath.Join(home, ".agents/skills/backscroll"),
		filepath.Join(configHome, "opencode/skills/backscroll"),
	}
	managed := ""
	for _, target := range targets {
		link, err := os.Readlink(target)
		if err != nil {
			t.Fatal(err)
		}
		if managed == "" {
			managed = link
		} else if link != managed {
			t.Fatalf("installed links differ: %s and %s", managed, link)
		}
		shim, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(shim, []byte("`backscroll --skill`")) || bytes.Contains(shim, []byte("result_N_filepath")) {
			t.Fatalf("installed discovery shim is not minimal: %s", shim)
		}
	}

	payload, payloadStderr, err := runBinary(work, "--skill")
	if err != nil || payloadStderr != "" || payload != embeddedBackscrollSkill {
		t.Fatalf("--skill error = %v, stderr = %q, exact payload = %v", err, payloadStderr, payload == embeddedBackscrollSkill)
	}
	if !strings.Contains(payload, "result_N_filepath") || strings.Contains(payload, "result_N_source_path") {
		t.Fatal("authoritative payload does not contain the observed robot keys")
	}

	document := filepath.Join(sessions, "fixture.jsonl")
	record := map[string]any{
		"type": "user", "uuid": "skill-install-fixture", "cwd": work,
		"timestamp": "2026-01-01T00:00:00Z",
		"message":   map[string]any{"role": "user", "content": "installationoraclecobalt"},
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(document, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	projectConfig := filepath.Join(configHome, "backscroll")
	if err := os.MkdirAll(projectConfig, 0o700); err != nil {
		t.Fatal(err)
	}
	projects := "[[projects]]\nid = \"skill-fixture\"\nroots = [" + string(mustJSON(t, work)) + "]\n"
	if err := os.WriteFile(filepath.Join(projectConfig, "projects.toml"), []byte(projects), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err = runBinary(work, "search", "--text", "installationoraclecobalt", "--robot", "--fields", "minimal", "--max-tokens", "2000")
	if err != nil || stderr != "" {
		t.Fatalf("installed recipe search error = %v, stderr = %q, stdout = %q", err, stderr, stdout)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			fields[key] = value
		}
	}
	if fields["result_0_filepath"] != document || fields["result_0_source_path"] != "" {
		t.Fatalf("installed recipe robot output = %q", stdout)
	}
}

func makeTreeRemovable(root string) {
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
}

func mustJSON(t *testing.T, value string) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func directoryTree(t *testing.T, root string) ([]string, error) {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, relative+":"+entry.Type().String())
		return nil
	})
	return paths, err
}
