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
	args := []string{"search", "--text", "violet handshake adaptation", "--all-projects", "--robot", "--fields", "minimal", "--max-tokens", "200"}
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
	positive := []string{"search", "--text", "violet handshake", "--all-projects", "--robot", "--fields", "minimal"}
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
		base := append([]string{"search", "--text", "violet handshake", "--all-projects"}, format...)
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

func TestSearchCandidatePaginationOutput(t *testing.T) {
	testEnv(t)
	sessions := t.TempDir()
	var records strings.Builder
	add := func(id string, content any) {
		record := map[string]any{
			"uuid": id, "type": "assistant", "cwd": "/synthetic/candidate",
			"timestamp": "2026-01-01T00:00:00Z",
			"message":   map[string]any{"role": "assistant", "content": content},
		}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		records.Write(data)
		records.WriteByte('\n')
	}
	for i := range 201 {
		query := "orchard handshake exactboundary"
		if i == 200 {
			query = "orchard handshake overflowboundary"
		}
		add(fmt.Sprintf("text-%d", i), query)
		add(fmt.Sprintf("tool-%d", i), queryEchoToolBlock(fmt.Sprintf("tool-%d", i), "rg "+query))
	}
	for i := range 450 {
		add(fmt.Sprintf("noise-%d", i), "adaptation unrelated distractor")
	}
	if err := os.WriteFile(filepath.Join(sessions, "pool.jsonl"), []byte(records.String()), 0600); err != nil {
		t.Fatal(err)
	}
	inputs := filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs")
	if err := os.MkdirAll(inputs, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("version = 1\n[[inputs]]\nid = \"candidates\"\nsource = \"session\"\nactive = true\n[inputs.discover]\nroots = [%q]\ninclude = [\"**/*.jsonl\"]\n[inputs.decode]\nformat = \"claude\"\n", sessions)
	if err := os.WriteFile(filepath.Join(inputs, "candidates.inputs.toml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(query string, flags ...string) (string, string) {
		t.Helper()
		out, stderr, err := runCmd(append([]string{"search", "--text", query, "--all-projects"}, flags...)...)
		if err != nil {
			t.Fatalf("search %v: %v %s", flags, err, stderr)
		}
		return out, stderr
	}
	for _, fields := range []string{"minimal", "full"} {
		for _, offset := range []string{"0", "400"} {
			out, stderr := run("orchard handshake", "--json", "--fields", fields, "--offset", offset)
			if !strings.HasPrefix(strings.TrimSpace(out), "[") {
				t.Fatalf("JSON output must be an array: %s", out)
			}
			var rows []map[string]any
			if err := json.Unmarshal([]byte(out), &rows); err != nil {
				t.Fatalf("bare JSON array contract: %v %s", err, out)
			}
			want := 20
			if offset == "400" {
				want = 0
			}
			if len(rows) != want || !strings.Contains(stderr, "candidate limit reached (200") || strings.Contains(stderr, "no results") {
				t.Fatalf("JSON page %s: rows=%d stderr=%s", offset, len(rows), stderr)
			}
		}
	}
	for _, flags := range [][]string{{"--robot", "--offset", "400"}, {"--robot", "--relax", "--offset", "400"}, {"--robot", "--limit", "1"}} {
		out, stderr := run("orchard handshake", flags...)
		if !strings.Contains(out, "candidate_limit=200\ncandidate_limit_reached=true\n") || strings.Contains(stderr, "candidate limit") {
			t.Fatalf("robot cap: stdout=%s stderr=%s", out, stderr)
		}
	}
	out, stderr := run("orchard handshake", "--robot", "--max-tokens", "1")
	if out != "" || !strings.Contains(stderr, "candidate limit reached (200") {
		t.Fatalf("tight-budget fallback: stdout=%q stderr=%s", out, stderr)
	}
	out, stderr = run("orchard handshake", "--offset", "400")
	if out != "" || !strings.Contains(stderr, "candidate limit reached (200") {
		t.Fatalf("text late page: stdout=%q stderr=%s", out, stderr)
	}
	for _, kind := range []string{"text", "tool"} {
		out, stderr = run("orchard handshake", "--json", "--content-type", kind, "--offset", "200")
		var rows []minimalSearchResult
		if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 || strings.Contains(stderr, "candidate limit") {
			t.Fatalf("typed late %s page: stdout=%s stderr=%s err=%v", kind, out, stderr, err)
		}
	}
	out, stderr = run("exactboundary", "--robot", "--limit", "1")
	if strings.Contains(out, "candidate_limit") || strings.Contains(stderr, "candidate limit") {
		t.Fatalf("exact boundary marked capped: stdout=%s stderr=%s", out, stderr)
	}
	out, stderr = run("orchard handshake adaptation", "--relax", "--robot", "--limit", "1")
	if !strings.Contains(out, "candidate_limit_reached=true") || !strings.Contains(out, "match_stage=drop-terms") {
		t.Fatalf("relaxed cap: stdout=%s stderr=%s", out, stderr)
	}
}
