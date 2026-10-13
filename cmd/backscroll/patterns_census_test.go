package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

func TestFailureCensusAndTrendKeys(t *testing.T) {
	path, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("TMPDIR", t.TempDir())
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, a ...any) {
		t.Helper()
		if _, err := db.DB().Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	// Two failure weeks with different denominators, a zero-failure week,
	// and an undated successful event.
	for i, stamp := range []string{"2026-01-05", "2026-01-05", "2026-01-05", "2026-01-12", "2026-01-12", "2026-01-19", ""} {
		signal := []any{1, 0, nil, 1, nil, 0, 0}[i]
		uuid := fmt.Sprint("census-", i)
		source := fmt.Sprint("/census-", i)
		exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,project,content_type,timestamp) VALUES ('session',?,0,'assistant','fixture',?,'p','tool',NULLIF(?,''))`, source, uuid, stamp)
		exec(`INSERT INTO tool_events(message_uuid,source_path,ordinal,tool_name,command_head,is_error,extraction_version) VALUES (?,?,0,'Bash','go',?,1)`, uuid, source, signal)
		exec(`INSERT INTO session_tags VALUES (?,'keep'),(?,'extra')`, source, source)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"commands", "failures"} {
		out, _, err := runCmd("patterns", "--kind", kind, "--trend", "--all-projects", "--robot")
		if err != nil {
			t.Fatal(err)
		}
		if kind == "failures" {
			for _, want := range []string{"week_1_total_events=2", "week_1_signalled_events=1", "week_1_unsignalled_events=1", "week_1_result_0_coverage=1/1", "week_2_result_0_coverage=1/2"} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q in %s", want, out)
				}
			}
			text, _, err := runCmd("patterns", "--kind", kind, "--trend", "--all-projects")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"Total events: 6", "Unsignalled events: 2", "2026-W03", "2026-W02", "2026-W01"} {
				if !strings.Contains(text, want) {
					t.Errorf("text missing %q: %s", want, text)
				}
			}
		}
		keys := map[string]bool{}
		for _, line := range strings.Split(out, "\n") {
			k, _, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			if keys[k] {
				t.Errorf("%s duplicate key %s", kind, k)
			}
			keys[k] = true
		}
	}
	for _, tc := range []struct {
		trend                     bool
		tag                       string
		total, signalled, unknown int
	}{
		{false, "keep", 7, 5, 2}, {false, "missing", 0, 0, 0}, {true, "keep", 6, 4, 2},
	} {
		args := []string{"patterns", "--kind", "failures", "--all-projects", "--tag", tc.tag, "--json"}
		if tc.trend {
			args = append(args, "--trend")
		}
		out, stderr, err := runCmd(args...)
		if err != nil {
			t.Fatal(err)
		}
		var p struct {
			Total     int `json:"total_events"`
			Signalled int `json:"signalled_events"`
			Unknown   int `json:"unsignalled_events"`
			Trends    []struct {
				Week      string
				Total     int `json:"total_events"`
				Signalled int `json:"signalled_events"`
				Unknown   int `json:"unsignalled_events"`
				Patterns  []storage.FailurePatternWeekly
			}
		}
		if err := json.Unmarshal([]byte(out), &p); err != nil {
			t.Fatal(err)
		}
		if p.Total != tc.total || p.Signalled != tc.signalled || p.Unknown != tc.unknown {
			t.Errorf("census got %s want %d/%d/%d", out, tc.total, tc.signalled, tc.unknown)
		}
		if tc.trend {
			if !strings.Contains(stderr, "1 events excluded") {
				t.Errorf("undated census diagnostic: %s", stderr)
			}
			if len(p.Trends) != 3 {
				t.Fatalf("want 3 census weeks: %s", out)
			}
			for i, w := range p.Trends {
				totals := []int{1, 2, 3}
				signals := []int{1, 1, 2}
				unknowns := []int{0, 1, 1}
				if w.Total != totals[i] || w.Signalled != signals[i] || w.Unknown != unknowns[i] {
					t.Errorf("week census: %+v", w)
				}
				for _, r := range w.Patterns {
					if r.SignalledEvents != signals[i] {
						t.Errorf("weekly denominator=%d want %d", r.SignalledEvents, signals[i])
					}
				}
			}
		}
	}
}

func TestFailureCensusEmptyFormats(t *testing.T) {
	path, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("TMPDIR", t.TempDir())
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SyncFiles([]storage.IndexedFile{{SourcePath: "/success", Source: "session", Hash: "fixture", Project: "p", Messages: []storage.IndexedMessage{
		{UUID: "success", Ordinal: 0, Role: "assistant", Text: "ok", ContentType: "tool", Timestamp: "2026-01-05T00:00:00Z", ToolName: "Bash", IsError: boolPtrCensus(false), ExtractionVersion: 1},
	}}}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	for _, trend := range []bool{false, true} {
		for _, format := range []string{"", "--json", "--robot"} {
			args := []string{"patterns", "--kind", "failures", "--project", "p", "--limit", "1", "--offset", "9"}
			if trend {
				args = append(args, "--trend")
			}
			if format != "" {
				args = append(args, format)
			}
			out, _, err := runCmd(args...)
			if err != nil {
				t.Fatal(err)
			}
			again, _, err := runCmd(args...)
			if err != nil || out != again {
				t.Fatalf("nondeterministic: %q / %q (%v)", out, again, err)
			}
			want := "Total events: 1"
			if format == "--robot" {
				want = "total_events=1"
			}
			if format == "--json" {
				want = `"total_events":1`
			}
			if !strings.Contains(out, want) {
				t.Errorf("zero failures missing census: %s", out)
			}
			if format == "--json" && !strings.Contains(out, `"patterns":[]`) {
				t.Errorf("zero failures invalid array: %s", out)
			}
		}
	}
}
func boolPtrCensus(v bool) *bool { return &v }
