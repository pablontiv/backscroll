package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type evalSet struct {
	Version string      `toml:"version"`
	Queries []evalQuery `toml:"query"`
}

type evalQuery struct {
	ID            string   `toml:"id"`
	Dataset       string   `toml:"dataset"`
	Cohort        string   `toml:"cohort"`
	Text          string   `toml:"text"`
	RefinedText   string   `toml:"refined_text"`
	Flags         []string `toml:"flags"`
	ExpectedMatch string   `toml:"expected_match"`
	ExpectedFile  string   `toml:"expected_file"`
	MaxTokens     int      `toml:"max_tokens"`
	ExpectError   bool     `toml:"expect_error"`
	ExpectAbsent  bool     `toml:"expect_absent"`
}

type commandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

type commandRunner func(query string, flags []string, fields string, maxTokens int) commandResult

type evalResult struct {
	ID                      string
	Cohort                  string
	Execution               string
	ExitCode                int
	Stderr                  string
	ReferenceRank           int
	BoundedReachable        bool
	RefinedRank             int
	RefinedBoundedReachable bool
	Diagnostic              bool
}

type cohortSummary struct {
	Cases, Eligible, Recalled, Bounded int
}

func loadEvalSet(path string) (evalSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return evalSet{}, fmt.Errorf("read eval set: %w", err)
	}
	var set evalSet
	if err := toml.Unmarshal(data, &set); err != nil {
		return evalSet{}, fmt.Errorf("parse eval set: %w", err)
	}
	for i := range set.Queries {
		if set.Queries[i].Dataset == "" {
			set.Queries[i].Dataset = "legacy"
		}
		if set.Queries[i].Cohort == "" {
			set.Queries[i].Cohort = "artifact-literal"
		}
		if set.Queries[i].MaxTokens == 0 {
			set.Queries[i].MaxTokens = 2000
		}
	}
	return set, nil
}

func evaluateQuery(q evalQuery, fixtureRoot string, run commandRunner) evalResult {
	r := evalResult{ID: q.ID, Cohort: q.Cohort, Execution: "ok", Diagnostic: q.ExpectAbsent}
	if r.Cohort == "" {
		r.Cohort = "artifact-literal"
	}
	full := run(q.Text, q.Flags, "full", 0)
	r.ExitCode, r.Stderr = full.ExitCode, full.Stderr
	if full.Err != nil || full.ExitCode != 0 {
		if q.ExpectError {
			r.Execution, r.Diagnostic = "expected-error", true
		} else {
			r.Execution = "error"
		}
		return r
	}
	if q.ExpectError {
		r.Execution, r.Diagnostic = "unexpected-success", true
		return r
	}
	r.ReferenceRank = findTargetRank(full.Stdout, q, fixtureRoot, true)
	bounded := run(q.Text, q.Flags, "minimal", q.MaxTokens)
	if bounded.Err != nil || bounded.ExitCode != 0 {
		r.Execution, r.ExitCode, r.Stderr = "error", bounded.ExitCode, bounded.Stderr
		return r
	}
	r.BoundedReachable = findTargetRank(bounded.Stdout, q, fixtureRoot, false) > 0
	if q.RefinedText != "" {
		refined := run(q.RefinedText, q.Flags, "full", 0)
		if refined.Err != nil || refined.ExitCode != 0 {
			r.Execution, r.ExitCode, r.Stderr = "error", refined.ExitCode, refined.Stderr
			return r
		}
		r.RefinedRank = findTargetRank(refined.Stdout, q, fixtureRoot, true)
		refinedBounded := run(q.RefinedText, q.Flags, "minimal", q.MaxTokens)
		if refinedBounded.Err != nil || refinedBounded.ExitCode != 0 {
			r.Execution, r.ExitCode, r.Stderr = "error", refinedBounded.ExitCode, refinedBounded.Stderr
			return r
		}
		r.RefinedBoundedReachable = findTargetRank(refinedBounded.Stdout, q, fixtureRoot, false) > 0
	}
	if q.ExpectAbsent && (r.ReferenceRank > 0 || r.BoundedReachable || r.RefinedRank > 0 || r.RefinedBoundedReachable) {
		r.Execution = "unexpected-success"
	}
	return r
}

var robotField = regexp.MustCompile(`^result_([0-9]+)_([a-z_]+)=(.*)$`)

func findTargetRank(output string, q evalQuery, fixtureRoot string, requireContent bool) int {
	results := parseRobotResults(output)
	expectedPath := ""
	if q.ExpectedFile != "" {
		expectedPath = filepath.Clean(filepath.Join(fixtureRoot, q.ExpectedFile))
	}
	indexes := make([]int, 0, len(results))
	for index := range results {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		fields := results[index]
		if expectedPath != "" {
			if filepath.Clean(fields["filepath"]) != expectedPath {
				continue
			}
			if requireContent && q.ExpectedMatch != "" && !strings.Contains(fields["content"], q.ExpectedMatch) {
				continue
			}
			return index + 1
		}
		if q.ExpectedMatch != "" && (strings.Contains(fields["filepath"], q.ExpectedMatch) || strings.Contains(fields["content"], q.ExpectedMatch)) {
			return index + 1
		}
	}
	return 0
}

func parseRobotResults(output string) map[int]map[string]string {
	results := make(map[int]map[string]string)
	for _, line := range strings.Split(output, "\n") {
		match := robotField.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		index, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		if results[index] == nil {
			results[index] = make(map[string]string)
		}
		results[index][match[2]] = unescapeRobot(match[3])
	}
	return results
}

func unescapeRobot(value string) string {
	var decoded strings.Builder
	decoded.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 == len(value) {
			decoded.WriteByte(value[i])
			continue
		}
		switch value[i+1] {
		case '\\':
			decoded.WriteByte('\\')
		case 'n':
			decoded.WriteByte('\n')
		case 'r':
			decoded.WriteByte('\r')
		default:
			decoded.WriteByte(value[i])
			continue
		}
		i++
	}
	return decoded.String()
}

func summarize(results []evalResult) map[string]cohortSummary {
	out := make(map[string]cohortSummary)
	for _, r := range results {
		s := out[r.Cohort]
		s.Cases++
		if !r.Diagnostic && r.Execution == "ok" {
			s.Eligible++
			if r.ReferenceRank > 0 && r.ReferenceRank <= 5 {
				s.Recalled++
			}
			if r.BoundedReachable {
				s.Bounded++
			}
		}
		out[r.Cohort] = s
	}
	return out
}

func runEvaluation(queries []evalQuery, fixtureRoot string, run commandRunner, verbose, qualityGate bool, threshold float64, out io.Writer) int {
	results := make([]evalResult, 0, len(queries))
	for _, q := range queries {
		r := evaluateQuery(q, fixtureRoot, run)
		results = append(results, r)
		if verbose {
			_, _ = fmt.Fprintf(out, "%s: execution=%s exit=%d reference_rank=%d bounded=%t", r.ID, r.Execution, r.ExitCode, r.ReferenceRank, r.BoundedReachable)
			if q.RefinedText != "" {
				_, _ = fmt.Fprintf(out, " refined_rank=%d refined_bounded=%t", r.RefinedRank, r.RefinedBoundedReachable)
			}
			if r.Stderr != "" {
				_, _ = fmt.Fprintf(out, " stderr=%q", strings.TrimSpace(r.Stderr))
			}
			_, _ = fmt.Fprintln(out)
		}
	}
	byCohort := summarize(results)
	cohorts := make([]string, 0, len(byCohort))
	for cohort := range byCohort {
		cohorts = append(cohorts, cohort)
	}
	sort.Strings(cohorts)
	eligible, recalled, bounded, failures := 0, 0, 0, 0
	for _, cohort := range cohorts {
		s := byCohort[cohort]
		eligible, recalled, bounded = eligible+s.Eligible, recalled+s.Recalled, bounded+s.Bounded
		_, _ = fmt.Fprintf(out, "Cohort %s: recall@5=%d/%d bounded=%d/%d cases=%d\n", cohort, s.Recalled, s.Eligible, s.Bounded, s.Eligible, s.Cases)
	}
	for _, r := range results {
		if r.Execution == "error" || r.Execution == "unexpected-success" {
			failures++
		}
	}
	percent := 0.0
	if eligible > 0 {
		percent = 100 * float64(recalled) / float64(eligible)
	}
	_, _ = fmt.Fprintf(out, "Recall@5: %d/%d (%.1f%%)\nBounded target reachability: %d/%d\nUnexpected execution failures: %d\n", recalled, eligible, percent, bounded, eligible, failures)
	if qualityGate {
		_, _ = fmt.Fprintf(out, "Quality gate: %.1f%% required\n", threshold)
		if failures > 0 {
			return 2
		}
		if percent < threshold {
			return 1
		}
		return 0
	}
	_, _ = fmt.Fprintln(out, "Quality gate: observational")
	if failures > 0 {
		return 2
	}
	return 0
}
