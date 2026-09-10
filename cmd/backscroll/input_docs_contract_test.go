package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestLivingInputManifestExamplesIngestThroughCommandBoundary(t *testing.T) {
	const (
		claudeFixture = `{"type":"user","uuid":"docs-claude-u1","timestamp":"2026-09-09T20:00:00Z","message":{"role":"user","content":[{"type":"text","text":"docclaudecobalt manifest example"}]}}` + "\n"
		codexFixture  = `{"type":"response_item","timestamp":"2026-09-01T12:00:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"doccodexquartz manifest example"}]}}` + "\n"
		piFixture     = `{"type":"session","id":"docs-pi-session","timestamp":"2026-09-09T20:00:00Z","cwd":"/workspace/docs"}` + "\n" +
			`{"type":"message","id":"docs-pi-u1","parentId":"docs-pi-session","timestamp":"2026-09-09T20:00:01Z","message":{"role":"user","content":[{"type":"text","text":"docpisaffron manifest example"}]}}` + "\n"
	)

	cases := []struct {
		name      string
		path      string
		anchor    string
		fixture   string
		query     string
		wholeFile bool
	}{
		{
			name:      "supported shipped Claude preset control",
			path:      "inputs/claude.inputs.toml",
			fixture:   claudeFixture,
			query:     "docclaudecobalt",
			wholeFile: true,
		},
		{
			name:    "input contract file shape",
			path:    "docs/input-contract.md",
			anchor:  "## File shape",
			fixture: claudeFixture,
			query:   "docclaudecobalt",
		},
		{
			name:    "input contract complete Claude example",
			path:    "docs/input-contract.md",
			anchor:  "## Complete Claude example",
			fixture: claudeFixture,
			query:   "docclaudecobalt",
		},
		{
			name:    "input contract complete Pi example",
			path:    "docs/input-contract.md",
			anchor:  "## Complete Pi example",
			fixture: piFixture,
			query:   "docpisaffron",
		},
		{
			name:      "supported shipped Codex preset",
			path:      "inputs/codex.inputs.toml",
			fixture:   codexFixture,
			query:     "doccodexquartz",
			wholeFile: true,
		},
		{
			name:    "input contract complete Codex example",
			path:    "docs/input-contract.md",
			anchor:  "## Complete Codex example",
			fixture: codexFixture,
			query:   "doccodexquartz",
		},
		{
			name:    "configuration guide session example",
			path:    "docs/configuration.md",
			anchor:  "A minimal installed manifest looks like:",
			fixture: claudeFixture,
			query:   "docclaudecobalt",
		},
		{
			name:    "sync guide session example",
			path:    "docs/sync.md",
			anchor:  "A session input example:",
			fixture: claudeFixture,
			query:   "docclaudecobalt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, content := readTrackedSkillMarkdown(t, tc.path)
			manifest := content
			if !tc.wholeFile {
				manifest = tomlFenceAfterAnchor(t, tc.path, content, tc.anchor)
			}

			root := t.TempDir()
			sessionsRoot := filepath.Join(root, "sessions")
			sourcePath := filepath.Join(sessionsRoot, "project", "session.jsonl")
			writeFile(t, sourcePath, tc.fixture)
			manifest = replaceManifestRoots(t, tc.path, manifest, sessionsRoot)

			cfgDir := filepath.Join(root, "config")
			setIndexPolicyEnv(t, filepath.Join(root, "index.db"), cfgDir)
			writeFile(t, filepath.Join(cfgDir, "backscroll", "inputs", "documented.inputs.toml"), manifest)

			var stdout, stderr bytes.Buffer
			argv := []string{"search", "--text", tc.query, "--all-projects", "--content-type", "text", "--source-path", sourcePath, "--json"}
			if err := run(&stdout, &stderr, argv); err != nil {
				t.Fatalf("living manifest %s failed command boundary: %v\nargv=%q\nstdout=%s\nstderr=%s", tc.path, err, argv, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.query) {
				t.Fatalf("living manifest %s did not ingest searchable fixture %q\nargv=%q\nstdout=%s\nstderr=%s", tc.path, tc.query, argv, stdout.String(), stderr.String())
			}
		})
	}
}

func TestLivingInputManifestDocsDoNotAdvertiseRetiredGenericFields(t *testing.T) {
	for _, path := range []string{"docs/configuration.md", "docs/input-contract.md", "docs/sync.md"} {
		_, content := readTrackedSkillMarkdown(t, path)
		for _, retired := range []string{
			`format = "jsonl"`,
			"[inputs.record]",
			"[inputs.map]",
			"[inputs.content]",
			"[inputs.text]",
		} {
			if strings.Contains(content, retired) {
				t.Errorf("%s advertises retired generic input field %q", path, retired)
			}
		}
	}
}

func TestUnsupportedInputDecoderFailsBeforeSearch(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	setIndexPolicyEnv(t, filepath.Join(root, "index.db"), cfgDir)
	writeFile(t, filepath.Join(cfgDir, "backscroll", "inputs", "unsupported.inputs.toml"), fmt.Sprintf(`version = 1

[[inputs]]
id = "unsupported"
source = "session"
active = true

[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]

[inputs.decode]
format = "jsonl"
`, filepath.Join(root, "sessions")))

	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, []string{"search", "--text", "sentinel", "--all-projects", "--json"})
	if err == nil {
		t.Fatalf("unsupported decoder search succeeded; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	const want = `no reader registered for format "jsonl"`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("unsupported decoder error missing %q; err=%v stdout=%s stderr=%s", want, err, stdout.String(), stderr.String())
	}
	const wantJSON = `no reader registered for format \"jsonl\"`
	if !strings.Contains(stdout.String(), wantJSON) {
		t.Fatalf("unsupported decoder JSON missing %q; stdout=%s stderr=%s", wantJSON, stdout.String(), stderr.String())
	}
}

func tomlFenceAfterAnchor(t *testing.T, path, content, anchor string) string {
	t.Helper()
	anchorAt := strings.Index(content, anchor)
	if anchorAt < 0 {
		t.Fatalf("%s missing manifest example anchor %q", path, anchor)
	}
	remainder := content[anchorAt+len(anchor):]
	const open = "```toml\n"
	openAt := strings.Index(remainder, open)
	if openAt < 0 {
		t.Fatalf("%s anchor %q has no TOML fence", path, anchor)
	}
	remainder = remainder[openAt+len(open):]
	closeAt := strings.Index(remainder, "```")
	if closeAt < 0 {
		t.Fatalf("%s anchor %q has unterminated TOML fence", path, anchor)
	}
	return remainder[:closeAt]
}

func replaceManifestRoots(t *testing.T, path, manifest, root string) string {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(manifest, "\r\n", "\n"), "\n")
	replaced := 0
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "roots = ") {
			lines[i] = fmt.Sprintf("roots = [%q]", root)
			replaced++
		}
	}
	if replaced != 1 {
		t.Fatalf("%s manifest example has %d roots declarations, want 1", path, replaced)
	}
	return strings.Join(lines, "\n")
}
