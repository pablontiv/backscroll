package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoveryShimDelegatesToSkillFlag(t *testing.T) {
	shim, err := os.ReadFile(filepath.Join("..", "..", ".claude", "skills", "backscroll", "SKILL.md"))
	if err != nil {
		t.Fatalf("read discovery shim: %v", err)
	}
	content := string(shim)
	for _, want := range []string{"---\nname: backscroll\n", "description:", "`backscroll --skill`", "follow the complete instructions printed to stdout"} {
		if !strings.Contains(content, want) {
			t.Errorf("discovery shim missing %q", want)
		}
	}
	if len(shim) > 1000 {
		t.Fatalf("discovery shim is %d bytes; want a minimal shim", len(shim))
	}
	for _, coupledProduct := range []string{"Claude", "Codex", "OpenCode"} {
		if strings.Contains(content, coupledProduct) {
			t.Errorf("discovery shim is coupled to external product %q", coupledProduct)
		}
	}
}

func TestSkillFlagPrintsExactPayloadAndBypassesUserState(t *testing.T) {
	root := t.TempDir()
	badConfig := filepath.Join(root, "config-is-a-file")
	badConfigContent := []byte("not a config directory\n")
	if err := os.WriteFile(badConfig, badConfigContent, 0o600); err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(root, "missing-home")
	database := filepath.Join(root, "missing-db-parent", "index.db")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "missing-cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "missing-xdg-config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "missing-state"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", badConfig)
	t.Setenv("BACKSCROLL_DATABASE_PATH", database)
	t.Setenv("BACKSCROLL_SESSION_DIRS", filepath.Join(root, "missing-inputs"))

	var stdout, stderr bytes.Buffer
	if err := run(&stdout, &stderr, []string{"--skill"}); err != nil {
		t.Fatalf("run --skill: %v", err)
	}
	if !bytes.Equal(stdout.Bytes(), []byte(embeddedBackscrollSkill)) {
		t.Fatal("--skill stdout does not exactly match the authoritative payload")
	}
	if stderr.Len() != 0 {
		t.Fatalf("--skill stderr = %q, want empty", stderr.String())
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("--skill touched HOME: stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(database)); !os.IsNotExist(err) {
		t.Fatalf("--skill touched database directory: stat error = %v", err)
	}
	gotConfig, err := os.ReadFile(badConfig)
	if err != nil {
		t.Fatalf("read invalid config sentinel: %v", err)
	}
	if !bytes.Equal(gotConfig, badConfigContent) {
		t.Fatal("--skill modified invalid config sentinel")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(badConfig) {
		t.Fatalf("--skill created side effects: entries = %v", entryNames(entries))
	}
}

func TestHelpDocumentsSkillFlag(t *testing.T) {
	stdout, stderr, err := runCmd("--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if stderr != "" {
		t.Fatalf("--help stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "--skill") || !strings.Contains(stdout, "Print the authoritative Backscroll skill payload") {
		t.Fatalf("--help does not document --skill:\n%s", stdout)
	}
}

func TestValidateSkillInvocationHonorsPOSIXSeparator(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantPrint  bool
		wantReject bool
	}{
		{name: "exact", args: []string{"--skill"}, wantPrint: true},
		{name: "literal search query", args: []string{"search", "--", "--skill"}},
		{name: "literal root argument", args: []string{"--", "--skill"}},
		{name: "inline literal after separator", args: []string{"search", "--", "--skill=true"}},
		{name: "combined before separator", args: []string{"search", "--skill", "--"}, wantReject: true},
		{name: "exact plus separator", args: []string{"--skill", "--", "--skill"}, wantReject: true},
		{name: "inline value before separator", args: []string{"--skill=true", "--"}, wantReject: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPrint, err := validateSkillInvocation(tc.args)
			if gotPrint != tc.wantPrint {
				t.Errorf("validateSkillInvocation(%v) print = %v, want %v", tc.args, gotPrint, tc.wantPrint)
			}
			if tc.wantReject {
				if err != errSkillMustBeUsedAlone {
					t.Fatalf("validateSkillInvocation(%v) error = %v, want %v", tc.args, err, errSkillMustBeUsedAlone)
				}
			} else if err != nil {
				t.Fatalf("validateSkillInvocation(%v) error = %v, want nil", tc.args, err)
			}
		})
	}
}

func TestDevBinarySkillEntryPoint(t *testing.T) {
	buildDir := t.TempDir()
	binary := filepath.Join(buildDir, "backscroll")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	buildOutput, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build dev binary: %v\n%s", err, buildOutput)
	}

	runtimeRoot := t.TempDir()
	badConfig := filepath.Join(runtimeRoot, "config-is-a-file")
	if err := os.WriteFile(badConfig, []byte("invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeEnv := environmentWithOverrides(os.Environ(), map[string]string{
		"HOME":                     filepath.Join(runtimeRoot, "missing-home"),
		"XDG_CACHE_HOME":           filepath.Join(runtimeRoot, "missing-cache"),
		"XDG_CONFIG_HOME":          filepath.Join(runtimeRoot, "missing-xdg-config"),
		"XDG_STATE_HOME":           filepath.Join(runtimeRoot, "missing-state"),
		"BACKSCROLL_CONFIG_DIR":    badConfig,
		"BACKSCROLL_DATABASE_PATH": filepath.Join(runtimeRoot, "missing-db", "index.db"),
		"BACKSCROLL_SESSION_DIRS":  filepath.Join(runtimeRoot, "missing-inputs"),
	})

	cmd := exec.Command(binary, "--skill")
	cmd.Env = runtimeEnv
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("dev binary --skill: %v; stderr=%q", err, stderr.String())
	}
	if !bytes.Equal(stdout, []byte(embeddedBackscrollSkill)) {
		t.Fatal("dev binary --skill stdout does not exactly match authoritative payload")
	}
	if stderr.Len() != 0 {
		t.Fatalf("dev binary --skill stderr = %q, want empty", stderr.String())
	}
	assertSkillRuntimeUntouched(t, runtimeRoot, badConfig)

	invalidInvocations := [][]string{
		{"--skill", "--help"},
		{"--version", "--skill"},
		{"--skill", "--json"},
		{"list", "--skill"},
		{"--skill", "list"},
		{"--skill", "extra"},
		{"--skill", "--skill"},
		{"--skill=true"},
	}
	for _, args := range invalidInvocations {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			cmd := exec.Command(binary, args...)
			cmd.Env = runtimeEnv
			cmd.Stderr = &stderr
			stdout, err := cmd.Output()
			if err == nil {
				t.Fatalf("%v succeeded, want usage error", args)
			}
			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != 1 {
				t.Fatalf("%v error = %v, want exit status 1", args, err)
			}
			if len(stdout) != 0 {
				t.Fatalf("%v stdout = %q, want empty", args, stdout)
			}
			wantStderr := errSkillMustBeUsedAlone.Error() + "\n"
			if stderr.String() != wantStderr {
				t.Fatalf("%v stderr = %q, want %q", args, stderr.String(), wantStderr)
			}
			assertSkillRuntimeUntouched(t, runtimeRoot, badConfig)
		})
	}

	t.Run("literal skill query after separator", func(t *testing.T) {
		searchRoot := t.TempDir()
		home := filepath.Join(searchRoot, "home")
		configDir := filepath.Join(searchRoot, "config")
		emptyInputs := filepath.Join(searchRoot, "empty-inputs")
		for _, path := range []string{home, configDir, emptyInputs} {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("create isolated search path: %v", err)
			}
		}
		database := filepath.Join(searchRoot, "index.db")
		searchEnv := environmentWithOverrides(os.Environ(), map[string]string{
			"HOME":                     home,
			"XDG_CACHE_HOME":           filepath.Join(searchRoot, "cache"),
			"XDG_CONFIG_HOME":          filepath.Join(searchRoot, "xdg-config"),
			"XDG_STATE_HOME":           filepath.Join(searchRoot, "state"),
			"BACKSCROLL_CONFIG_DIR":    configDir,
			"BACKSCROLL_DATABASE_PATH": database,
			"BACKSCROLL_SESSION_DIRS":  emptyInputs,
		})

		var stderr bytes.Buffer
		cmd := exec.Command(binary, "search", "--", "--skill")
		cmd.Env = searchEnv
		cmd.Stderr = &stderr
		stdout, err := cmd.Output()
		if err != nil {
			t.Fatalf("search literal --skill: %v; stdout=%q stderr=%q", err, stdout, stderr.String())
		}
		if strings.Contains(stderr.String(), errSkillMustBeUsedAlone.Error()) {
			t.Fatalf("literal --skill was intercepted as global option: %q", stderr.String())
		}
		if !strings.Contains(stderr.String(), "no results") {
			t.Fatalf("literal --skill did not reach search result flow; stderr=%q", stderr.String())
		}
		if _, err := os.Stat(database); err != nil {
			t.Fatalf("search did not create isolated database: %v", err)
		}
	})
}

func assertSkillRuntimeUntouched(t *testing.T, runtimeRoot, badConfig string) {
	t.Helper()
	content, err := os.ReadFile(badConfig)
	if err != nil {
		t.Fatalf("read config sentinel: %v", err)
	}
	if string(content) != "invalid\n" {
		t.Fatalf("config sentinel content = %q, want unchanged", content)
	}
	entries, err := os.ReadDir(runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(badConfig) {
		t.Fatalf("skill invocation created side effects: entries = %v", entryNames(entries))
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func environmentWithOverrides(base []string, overrides map[string]string) []string {
	env := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		key, _, _ := strings.Cut(item, "=")
		if _, replaced := overrides[key]; !replaced {
			env = append(env, item)
		}
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}
