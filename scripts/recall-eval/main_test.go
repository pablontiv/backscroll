package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateSeparatesExecutionRankAndBoundedReachability(t *testing.T) {
	q := evalQuery{ID: "case", Cohort: "terminology-recovery", Text: "needle", ExpectedFile: "target.jsonl", ExpectedMatch: "answer-after-preview", MaxTokens: 7}
	run := func(_ string, _ []string, fields string, _ int) commandResult {
		if fields == "full" {
			return commandResult{Stdout: "result_0_filepath=/fixtures/target.jsonl\nresult_0_content=" + strings.Repeat("x", 240) + "answer-after-preview"}
		}
		return commandResult{Stdout: "result_0_truncated=true\nresult_0_omitted=1\n"}
	}
	got := evaluateQuery(q, "/fixtures", run)
	if got.Execution != "ok" || got.ReferenceRank != 1 || got.BoundedReachable {
		t.Fatalf("result = %+v", got)
	}
}

func TestEvaluateUsesExactFileIdentityAndCompleteGroundTruth(t *testing.T) {
	q := evalQuery{ID: "identity", Text: "needle", ExpectedFile: "target.jsonl", ExpectedMatch: "ground-truth", MaxTokens: 20}
	run := func(_ string, _ []string, fields string, _ int) commandResult {
		if fields == "full" {
			return commandResult{Stdout: "result_0_filepath=/fixtures/target.jsonl.decoy\nresult_0_content=ground-truth\nresult_1_filepath=/fixtures/target.jsonl\nresult_1_content=wrong"}
		}
		return commandResult{Stdout: "result_0_filepath=/fixtures/target.jsonl\n"}
	}
	got := evaluateQuery(q, "/fixtures", run)
	if got.ReferenceRank != 0 || !got.BoundedReachable {
		t.Fatalf("result = %+v", got)
	}
}

func TestUnescapeRobotPreservesBackslashesAndControlCharacters(t *testing.T) {
	got := unescapeRobot(`C:\\fixtures\\target\nnext\rline`)
	want := "C:\\fixtures\\target\nnext\rline"
	if got != want {
		t.Fatalf("unescapeRobot() = %q, want %q", got, want)
	}
}

func TestEvaluateKeepsStderrOutOfRobotOutput(t *testing.T) {
	q := evalQuery{ID: "failure", Text: "needle", ExpectedFile: "target.jsonl", ExpectError: true}
	run := func(_ string, _ []string, _ string, _ int) commandResult {
		return commandResult{Stderr: "result_0_filepath=/fixtures/target.jsonl", ExitCode: 23, Err: errors.New("exit status 23")}
	}
	got := evaluateQuery(q, "/fixtures", run)
	if got.Execution != "expected-error" || got.ReferenceRank != 0 || !strings.Contains(got.Stderr, "target.jsonl") {
		t.Fatalf("result = %+v", got)
	}
}

func TestSyntheticEnvironmentIsIsolatedAndReportIsObservational(t *testing.T) {
	fixtures := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixtures, "target.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareSyntheticEnvironment(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(prepared.Cleanup)
	if prepared.FixtureDigest == "" || !strings.HasPrefix(environmentMap(prepared.Env)["BACKSCROLL_DATABASE_PATH"], prepared.WorkDir) {
		t.Fatalf("prepared = %+v", prepared)
	}
	queries := []evalQuery{{ID: "miss", Cohort: "conversational-paraphrase", Text: "miss", ExpectedFile: "target.jsonl", MaxTokens: 10}}
	var out bytes.Buffer
	if code := runEvaluation(queries, fixtures, func(string, []string, string, int) commandResult { return commandResult{} }, true, false, 80, &out); code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	for _, want := range []string{"reference_rank=0", "conversational-paraphrase", "Quality gate: observational"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestLoadSelectValidateAndSearchArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.toml")
	data := `version = "2.0"
[[query]]
id = "legacy"
text = "literal"
expected_match = "target"
[[query]]
id = "synthetic"
dataset = "synthetic"
cohort = "query-echo"
text = "echo"
expected_file = "target.jsonl"
max_tokens = 37
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	set, err := loadEvalSet(path)
	if err != nil {
		t.Fatal(err)
	}
	selected := selectQueries(set.Queries, "synthetic", 1)
	if len(selected) != 1 || selected[0].ID != "synthetic" || set.Queries[0].Dataset != "legacy" || set.Queries[0].MaxTokens != 2000 {
		t.Fatalf("set=%+v selected=%+v", set, selected)
	}
	if err := validateQueries(selected, "synthetic"); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(searchArgs("echo", []string{"--content-type", "tool"}, "minimal", 37), " ")
	for _, want := range []string{"search --text echo", "--all-projects", "--fields minimal", "--max-tokens 37"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q missing %q", args, want)
		}
	}
}

func TestValidateRejectsDuplicateAndEscapingTargets(t *testing.T) {
	queries := []evalQuery{
		{ID: "same", Text: "one", ExpectedFile: "../outside.jsonl"},
		{ID: "same", Text: "two", ExpectedFile: "target.jsonl"},
	}
	if err := validateQueries(queries, "synthetic"); err == nil {
		t.Fatal("invalid queries were accepted")
	}
}

func TestRunEvaluationDistinguishesUnexpectedFailureAndLegacyGate(t *testing.T) {
	queries := []evalQuery{{ID: "broken", Cohort: "artifact-literal", Text: "broken", ExpectedMatch: "target"}}
	var out bytes.Buffer
	code := runEvaluation(queries, "", func(string, []string, string, int) commandResult {
		return commandResult{ExitCode: 17, Err: errors.New("exit 17"), Stderr: "synthetic failure"}
	}, true, true, 80, &out)
	if code != 2 || !strings.Contains(out.String(), "Unexpected execution failures: 1") {
		t.Fatalf("code=%d output=%s", code, out.String())
	}

	out.Reset()
	code = runEvaluation(queries, "", func(string, []string, string, int) commandResult { return commandResult{} }, false, true, 80, &out)
	if code != 1 || !strings.Contains(out.String(), "Quality gate: 80.0% required") {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
}

func TestRunMainExecutesLegacyEvaluationWithSeparateSubprocessStreams(t *testing.T) {
	dir := t.TempDir()
	evalPath := filepath.Join(dir, "queries.toml")
	if err := os.WriteFile(evalPath, []byte("version = \"2.0\"\n[[query]]\nid = \"legacy\"\ntext = \"needle\"\nexpected_match = \"target\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "fake-backscroll")
	script := `#!/bin/sh
if [ "$1" = status ]; then
  printf '%s\n' '{"index":{"total_files":1,"total_messages":2}}'
  exit 0
fi
printf '%s\n' 'result_0_filepath=/fixtures/target.jsonl'
printf '%s\n' 'result_0_content=target'
printf '%s\n' 'diagnostic remains on stderr' >&2
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runMain([]string{"--backscroll", bin, "--eval-set", evalPath, "--dataset", "legacy", "--source-sha", "abc"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "Recall@5: 1/1") || strings.Contains(stdout.String(), "diagnostic remains") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if code := runMain([]string{"--dataset", "unsupported"}, io.Discard, io.Discard); code != 2 {
		t.Fatalf("invalid options code=%d, want 2", code)
	}
}

func environmentMap(entries []string) map[string]string {
	result := make(map[string]string)
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}
