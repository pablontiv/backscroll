package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The target is the scout's tracked native recall fixture. The added adaptation
// term is absent from it but frequent in independent distractor records: a
// recoverable AND-overload case, unlike the zero-overlap s2/s5 paraphrases.
// Removing opt-in fallback or dropping rare anchors instead of common terms
// must lose this target; changing the default must fail the strict control.
func TestSearchRelaxationRecoverableE2E(t *testing.T) {
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
	inputs := filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs")
	if err := os.MkdirAll(inputs, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("version = 1\n[[inputs]]\nid = \"relaxation\"\nsource = \"session\"\nactive = true\n[inputs.discover]\nroots = [%q]\ninclude = [\"**/*.jsonl\"]\n[inputs.decode]\nformat = \"claude\"\n", sessions)
	if err := os.WriteFile(filepath.Join(inputs, "relaxation.inputs.toml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"search", "--text", "violet handshake adaptation", "--all-projects", "--lexical-only", "--robot", "--fields", "minimal", "--max-tokens", "200"}
	strict, stderr, err := runCmd(args...)
	if err != nil || strict != "" {
		t.Fatalf("strict must miss target: err=%v stdout=%s stderr=%s", err, strict, stderr)
	}
	out, stderr, err := runCmd(append(args, "--relax")...)
	if err != nil {
		t.Fatalf("relaxed search: %v stderr=%s", err, stderr)
	}
	if !strings.Contains(out, "result_0_filepath="+target+"\n") || strings.Count(out, "_filepath=") != 1 {
		t.Fatalf("want exact target alone in bounded output, got %s stderr=%s", out, stderr)
	}
	if !strings.Contains(out, "result_0_match_stage=drop-terms\n") || !strings.Contains(out, "result_0_dropped_terms=[\"adaptation\"]\n") {
		t.Fatalf("relaxed target must carry provenance: %s", out)
	}
	positive := []string{"search", "--text", "violet handshake", "--all-projects", "--lexical-only", "--robot", "--fields", "minimal"}
	before, _, err := runCmd(positive...)
	if err != nil {
		t.Fatal(err)
	}
	after, _, err := runCmd(append(positive, "--relax")...)
	if err != nil || after != before {
		t.Fatalf("an existing strict match must be unchanged, without relaxed provenance: %v\nbefore=%s\nafter=%s", err, before, after)
	}
	for _, fields := range []string{"minimal", "full"} {
		jsonArgs := []string{"search", "--text", "violet handshake adaptation", "--all-projects", "--relax", "--json", "--fields", fields}
		payload, stderr, err := runCmd(jsonArgs...)
		if err != nil {
			t.Fatalf("JSON search: %v %s", err, stderr)
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(payload), &rows); err != nil || len(rows) != 1 {
			t.Fatalf("bad JSON: %v %s", err, payload)
		}
		if rows[0]["match_stage"] != "drop-terms" {
			t.Fatalf("JSON provenance missing: %s", payload)
		}
	}
	human, stderr, err := runCmd("search", "--text", "violet handshake adaptation", "--all-projects", "--relax")
	if err != nil || !strings.Contains(human, "Match stage: drop-terms | Dropped terms: [\"adaptation\"]") {
		t.Fatalf("human provenance missing: %v %s %s", err, human, stderr)
	}
	for _, filter := range [][]string{{"--project", "nonexistent-project"}, {"--source-path", "/absent-input.jsonl"}, {"--content-type", "tool"}, {"--text", "+adaptation violet handshake"}} {
		// These invocations deliberately omit --all-projects; explicit project
		// and input filters must never be broadened to recover the known target.
		filtered := []string{"search", "--text", "violet handshake adaptation", "--relax", "--robot", "--project", "eval"}
		payload, stderr, err := runCmd(append(filtered, filter...)...)
		if err != nil || payload != "" || !strings.Contains(stderr, "relaxation stages tried: strict") {
			t.Fatalf("protected query/filter escaped: %v stdout=%s stderr=%s", err, payload, stderr)
		}
	}
	for _, format := range [][]string{{}, {"--robot"}, {"--json"}, {"--json", "--fields", "full"}} {
		base := append([]string{"search", "--text", "violet handshake", "--all-projects", "--lexical-only"}, format...)
		before, beforeErr, err := runCmd(base...)
		if err != nil {
			t.Fatal(err)
		}
		after, afterErr, err := runCmd(append(base, "--relax=false")...)
		if err != nil || after != before || afterErr != beforeErr {
			t.Fatalf("disabled relaxation changed bytes: %v before=%q after=%q", err, before, after)
		}
	}
	strictAgain, _, err := runCmd(args...)
	if err != nil || strictAgain != strict {
		t.Fatalf("opt-in changed subsequent strict output: %v %q", err, strictAgain)
	}
}
