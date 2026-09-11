package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Direct search-echo tool rows are excluded from unfiltered result pages
// (#70/#80) and must not invert --relax IDF. Without that, the native
// recoverable overload (target = violet handshake; extra term = adaptation)
// drops "violet" instead of "adaptation" and returns zero rows.
func TestSearchRelaxationQueryEchoesDoNotInvertIDF(t *testing.T) {
	testEnv(t)
	sessions := t.TempDir()
	target := filepath.Join(sessions, "progressive-target.jsonl")
	fixture, err := os.ReadFile(filepath.Join("..", "..", "docs", "eval", "fixtures", "recall", "progressive-target.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		record := fmt.Sprintf(`{"uuid":"noise-%d","type":"assistant","cwd":"/synthetic/eval","timestamp":"2026-01-01T00:08:00Z","message":{"role":"assistant","content":"The adaptation discussion concerns unrelated component %d."}}`+"\n", i, i)
		if err := os.WriteFile(filepath.Join(sessions, fmt.Sprintf("noise-%d.jsonl", i)), []byte(record), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 8; i++ {
		record := fmt.Sprintf(`{"uuid":"echo-%d","type":"assistant","cwd":"/synthetic/eval","timestamp":"2026-01-01T00:09:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"echo-%d","name":"Bash","input":{"command":"backscroll search --text 'violet handshake'"}}]}}`+"\n", i, i)
		if err := os.WriteFile(filepath.Join(sessions, fmt.Sprintf("echo-%d.jsonl", i)), []byte(record), 0600); err != nil {
			t.Fatal(err)
		}
	}
	inputs := filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs")
	if err := os.MkdirAll(inputs, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("version = 1\n[[inputs]]\nid = \"relaxation-echo-idf\"\nsource = \"session\"\nactive = true\n[inputs.discover]\nroots = [%q]\ninclude = [\"**/*.jsonl\"]\n[inputs.decode]\nformat = \"claude\"\n", sessions)
	if err := os.WriteFile(filepath.Join(inputs, "relaxation-echo-idf.inputs.toml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}

	control := []string{"search", "--text", "violet handshake adaptation", "--all-projects", "--lexical-only", "--robot", "--fields", "minimal", "--max-tokens", "200"}
	strict, stderr, err := runCmd(control...)
	if err != nil || strict != "" {
		t.Fatalf("strict must miss: err=%v stdout=%s stderr=%s", err, strict, stderr)
	}
	out, stderr, err := runCmd(append(control, "--relax")...)
	if err != nil {
		t.Fatalf("relaxed search: %v stderr=%s", err, stderr)
	}
	if strings.Contains(out, "command=backscroll") || strings.Contains(out, "echo-") {
		t.Fatalf("echo leaked into unfiltered --relax output: %s", out)
	}
	if !strings.Contains(out, "result_0_filepath="+target+"\n") || !strings.Contains(out, `result_0_dropped_terms=["adaptation"]`) {
		t.Fatalf("query-echo rows inverted --relax IDF\nstdout=%s\nstderr=%s", out, stderr)
	}
}
