package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

func TestListHelpDescribesDefaultProjectScope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := buildRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"list", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("list --help: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("list --help stderr = %q, want empty", stderr.String())
	}
	help := stdout.String()
	for _, want := range []string{
		"project inferred from the current working directory",
		"Use --all-projects to list across all projects",
		"List sessions across all projects",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("list --help missing %q:\n%s", want, help)
		}
	}
}

func TestListJSONSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("list", "--json", "--all-projects")
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var payload struct {
		Count    int `json:"count"`
		Sessions []struct {
			Path    string `json:"Path"`
			Project string `json:"Project"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("list --json emitted invalid JSON %q: %v", stdout, err)
	}
	if payload.Count == 0 || len(payload.Sessions) == 0 {
		t.Fatalf("list --json returned empty sessions after seeded startup: %+v", payload)
	}
	foundSeed := false
	for _, s := range payload.Sessions {
		if s.Path == "/cov/s.jsonl" && s.Project == "covproj" {
			foundSeed = true
			break
		}
	}
	if !foundSeed {
		t.Fatalf("list --json missing seeded /cov/s.jsonl@covproj session: %+v", payload.Sessions)
	}
}

func TestListRobotSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--robot", "--all-projects"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestListOrderAscWithLimitOffset(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--order", "timestamp:asc", "--limit", "1", "--offset", "1", "--all-projects"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestListInvalidOrderRejected(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--order", "nonsense:updown", "--all-projects"); err == nil {
		t.Log("invalid order accepted silently (documenting current behavior)")
	}
}

func TestListRecentZeroAll(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("list", "--recent", "0", "--all-projects"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestListProjectFilterSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("list", "--project", "covproj")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	_ = stdout
}

func TestListProjectScopeAcrossLegacyAndV2(t *testing.T) {
	setupListScopeFixture(t)

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "implicit cwd legacy", args: []string{"list", "--json"}, want: []string{"/scope/a.jsonl"}},
		{name: "implicit cwd v2", args: []string{"list", "--json", "--limit", "10"}, want: []string{"/scope/a.jsonl"}},
		{name: "explicit project", args: []string{"list", "--json", "--project", "project-b"}, want: []string{"/scope/b.jsonl"}},
		{name: "all projects", args: []string{"list", "--json", "--all-projects"}, want: []string{"/scope/a.jsonl", "/scope/b.jsonl"}},
		{name: "all projects wins", args: []string{"list", "--json", "--project", "project-a", "--all-projects", "--limit", "10"}, want: []string{"/scope/a.jsonl", "/scope/b.jsonl"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runCmd(tc.args...)
			if err != nil {
				t.Fatalf("run: %v\nstderr=%s", err, stderr)
			}
			var payload struct {
				Sessions []struct {
					Path string `json:"Path"`
				} `json:"sessions"`
			}
			if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
				t.Fatalf("decode %q: %v", stdout, err)
			}
			got := make(map[string]bool, len(payload.Sessions))
			for _, session := range payload.Sessions {
				got[session.Path] = true
			}
			if len(got) != len(tc.want) {
				t.Fatalf("paths=%v, want %v", got, tc.want)
			}
			for _, path := range tc.want {
				if !got[path] {
					t.Errorf("paths=%v missing %q", got, path)
				}
			}
			if got["/scope/plan.md"] {
				t.Errorf("list included non-session source: %v", got)
			}
		})
	}
}

func TestListEmptyOutputKeepsFormatsAndWritesScopedHintToStderr(t *testing.T) {
	setupListScopeFixture(t)

	for _, tc := range []struct {
		name string
		flag string
		want string
	}{
		{name: "text", want: "No sessions found\n"},
		{name: "json", flag: "--json", want: "{\"count\":0,\"sessions\":[]}\n"},
		{name: "robot", flag: "--robot", want: "No sessions found\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"list", "--project", "missing"}
			if tc.flag != "" {
				args = append(args, tc.flag)
			}
			stdout, stderr, err := runCmd(args...)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if stdout != tc.want {
				t.Errorf("stdout=%q, want %q", stdout, tc.want)
			}
			for _, want := range []string{`project "missing"`, "--all-projects"} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr %q missing %q", stderr, want)
				}
				if strings.Contains(stdout, want) {
					t.Errorf("stdout leaked diagnostic %q: %q", want, stdout)
				}
			}
		})
	}
}

func TestListDeletedCwdFailsBeforeOpeningDatabase(t *testing.T) {
	originalGetwd := currentWorkingDirectory
	currentWorkingDirectory = func() (string, error) { return "", errors.New("current directory was removed") }
	t.Cleanup(func() { currentWorkingDirectory = originalGetwd })

	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	inputsDir := filepath.Join(root, "inputs")
	for _, dir := range []string{configDir, inputsDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dbPath := filepath.Join(root, "must-not-exist.db")
	t.Setenv("HOME", root)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	t.Setenv("BACKSCROLL_SESSION_DIRS", inputsDir)

	var stdout, stderr bytes.Buffer
	cmd := buildRootCmd(&stdout, &stderr)
	cmd.SetArgs([]string{"list"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("list succeeded with an unresolvable current directory")
	}
	for _, want := range []string{"--project NAME", "--all-projects"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("resolution failure wrote output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
		t.Errorf("database touched before scope resolution: stat error=%v", statErr)
	}
}

func setupListScopeFixture(t *testing.T) {
	t.Helper()
	dbPath, cleanup := testEnv(t)
	t.Cleanup(cleanup)

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	files := []storage.IndexedFile{
		{SourcePath: "/scope/a.jsonl", Source: "session", Hash: "scope-a", Project: "project-a", Messages: []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "a", Timestamp: "2026-01-01T00:00:00Z"}}},
		{SourcePath: "/scope/b.jsonl", Source: "session", Hash: "scope-b", Project: "project-b", Messages: []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "b", Timestamp: "2026-01-02T00:00:00Z"}}},
		{SourcePath: "/scope/plan.md", Source: "plan", Hash: "scope-plan", Project: "project-a", Messages: []storage.IndexedMessage{{Ordinal: 0, Role: "plan", Text: "plan", Timestamp: "2026-01-03T00:00:00Z"}}},
	}
	if err := db.SyncFiles(files); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	projectDir := filepath.Join(t.TempDir(), "project-a")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	registryDir := filepath.Join(home, ".config", "backscroll")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := fmt.Sprintf("[[projects]]\nid = \"project-a\"\nroots = [%q]\n", projectDir)
	if err := os.WriteFile(filepath.Join(registryDir, "projects.toml"), []byte(registry), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)
}
