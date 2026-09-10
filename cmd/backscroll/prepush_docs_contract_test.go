package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrePushDocumentationGateUsesSubstantiveAGENTS(t *testing.T) {
	repoRoot := prePushRepositoryRoot(t)
	hook, err := os.ReadFile(filepath.Join(repoRoot, ".githooks", "pre-push"))
	if err != nil {
		t.Fatal(err)
	}
	safeGate := prePushDocumentationGateOnly(t, string(hook))

	cases := []struct {
		name       string
		mutate     func(*testing.T, string)
		wantExit   int
		wantOutput string
	}{
		{"production Go with AGENTS", mutateProductionGoAndAGENTS, 0, "DOC_GATES_OK"},
		{"package addition with AGENTS", mutatePackageAndAGENTS, 0, "DOC_GATES_OK"},
		{"CI config with AGENTS", mutateCIConfigAndAGENTS, 0, "DOC_GATES_OK"},
		{"package addition without docs", mutatePackageOnly, 1, "Go source changed but docs were not updated."},
		{"package addition with pointer only", mutatePackageAndPointer, 1, "Go source changed but docs were not updated."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			repo := filepath.Join(root, "repo")
			for _, path := range []string{filepath.Join(repo, "internal", "core"), filepath.Join(root, "home"), filepath.Join(root, "tmp")} {
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writePrePushFixture(t, filepath.Join(repo, "internal", "core", "core.go"), "package core\n")
			writePrePushFixture(t, filepath.Join(repo, "AGENTS.md"), "# Agent memory\n\n## Package Layout\n\n- internal/core\n")
			writePrePushFixture(t, filepath.Join(repo, "CLAUDE.md"), "<!-- Points Claude at AGENTS.md via import; edit AGENTS.md, not this file. -->\n@AGENTS.md\n")

			env := prePushFixtureEnv(root)
			runPrePushGit(t, repo, env, "init", "-q")
			runPrePushGit(t, repo, env, "add", ".")
			runPrePushGit(t, repo, env, "-c", "user.name=Contract", "-c", "user.email=contract@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "base")
			base := strings.TrimSpace(runPrePushGit(t, repo, env, "rev-parse", "HEAD"))

			tc.mutate(t, repo)
			runPrePushGit(t, repo, env, "add", ".")
			runPrePushGit(t, repo, env, "-c", "user.name=Contract", "-c", "user.email=contract@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "scenario")
			head := strings.TrimSpace(runPrePushGit(t, repo, env, "rev-parse", "HEAD"))

			script := filepath.Join(root, "pre-push-doc-gates.sh")
			writePrePushFixture(t, script, safeGate)
			if err := os.Chmod(script, 0o700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", script, "origin", "unused")
			cmd.Dir = repo
			cmd.Env = env
			cmd.Stdin = strings.NewReader(fmt.Sprintf("refs/heads/scenario %s refs/heads/main %s\n", head, base))
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			gotExit := 0
			if err != nil {
				exitErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("run safe documentation gate: %v", err)
				}
				gotExit = exitErr.ExitCode()
			}
			if gotExit != tc.wantExit || !strings.Contains(stdout.String(), tc.wantOutput) {
				t.Fatalf("documentation gate exit/output = %d/%q, want %d containing %q; stderr=%q", gotExit, stdout.String(), tc.wantExit, tc.wantOutput, stderr.String())
			}
		})
	}
}

func prePushDocumentationGateOnly(t *testing.T, hook string) string {
	t.Helper()
	const boundary = "  # CI parity gate: run full CI if Go files changed"
	at := strings.Index(hook, boundary)
	if at < 0 {
		t.Fatalf("pre-push hook missing documentation/CI boundary")
	}
	prefix := hook[:at]
	for _, forbidden := range []string{"/tmp/backscroll-ci.log", "just ci", "install_input_presets", "SKILLS_DEST", "BACKSCROLL_BIN", "sudo", "go build"} {
		if strings.Contains(prefix, forbidden) {
			t.Fatalf("documentation-only hook prefix contains post-boundary side effect %q", forbidden)
		}
	}
	return prefix + "  echo DOC_GATES_OK\n  exit 0\ndone\nexit 0\n"
}

func prePushRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func prePushFixtureEnv(root string) []string {
	return []string{
		"HOME=" + filepath.Join(root, "home"),
		"TMPDIR=" + filepath.Join(root, "tmp"),
		"PATH=" + os.Getenv("PATH"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_EXTERNAL_DIFF=",
		"GIT_OPTIONAL_LOCKS=0",
	}
}

func runPrePushGit(t *testing.T, repo string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writePrePushFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mutateProductionGoAndAGENTS(t *testing.T, repo string) {
	writePrePushFixture(t, filepath.Join(repo, "internal", "core", "core.go"), "package core\n\nconst Changed = true\n")
	appendPrePushFixture(t, filepath.Join(repo, "AGENTS.md"), "\nProduction behavior updated.\n")
}

func mutatePackageAndAGENTS(t *testing.T, repo string) {
	writePrePushFixture(t, filepath.Join(repo, "internal", "newpkg", "new.go"), "package newpkg\n")
	appendPrePushFixture(t, filepath.Join(repo, "AGENTS.md"), "\n- internal/newpkg\n")
}

func mutateCIConfigAndAGENTS(t *testing.T, repo string) {
	writePrePushFixture(t, filepath.Join(repo, ".github", "workflows", "contract.yml"), "name: contract\n")
	appendPrePushFixture(t, filepath.Join(repo, "AGENTS.md"), "\nCI contract updated.\n")
}

func mutatePackageOnly(t *testing.T, repo string) {
	writePrePushFixture(t, filepath.Join(repo, "internal", "newpkg", "new.go"), "package newpkg\n")
}

func mutatePackageAndPointer(t *testing.T, repo string) {
	mutatePackageOnly(t, repo)
	appendPrePushFixture(t, filepath.Join(repo, "CLAUDE.md"), "<!-- pointer refreshed -->\n")
}

func appendPrePushFixture(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
