package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

func TestPatternsTagEligibilityCLI(t *testing.T) {
	path, cleanup := testEnv(t)
	defer cleanup()
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.DB().Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 6; i++ {
		source := fmt.Sprint("/tag-session-", i)
		for ordinal, tool := range []string{"Read", "Write"} {
			uuid := fmt.Sprintf("tag-%d-%d", i, ordinal)
			exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,project,content_type,timestamp) VALUES ('session',?,?,'user','No, use another approach',?,'p','text','2026-01-05T00:00:00Z')`, source, ordinal, uuid)
			exec(`INSERT INTO tool_events(message_uuid,source_path,ordinal,tool_name,extraction_version) VALUES (?,?,?,?,1)`, uuid, source, ordinal, tool)
			if ordinal == 0 {
				exec(`INSERT INTO correction_signals(item_uuid,source_path,ordinal,detector,confidence,extraction_version) VALUES (?,?,0,'test',?,?)`, uuid, source, 1-float64(i)/10, storage.CurrentExtractionVersion)
			}
		}
		if i == 1 || i == 3 {
			exec(`INSERT INTO session_tags VALUES (?,'keep')`, source)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		kind, tag      string
		extra          []string
		count, support int
	}{
		{"corrections", "missing", nil, 0, 0}, {"sequences", "missing", []string{"--min-support", "1"}, 0, 0},
		{"corrections", "keep", []string{"--limit", "1", "--offset", "1"}, 1, 0},
		{"sequences", "keep", []string{"--min-support", "2", "--min-length", "2", "--max-length", "2"}, 1, 2},
		{"sequences", "keep", []string{"--min-support", "3"}, 0, 0},
		{"sequences", "keep", []string{"--min-support", "1", "--min-length", "1", "--max-length", "2", "--limit", "1", "--offset", "1"}, 1, 2},
	} {
		t.Run(fmt.Sprint(tc.kind, tc.tag, tc.extra), func(t *testing.T) {
			args := append([]string{"patterns", "--kind", tc.kind, "--all-projects", "--tag", tc.tag, "--json"}, tc.extra...)
			out, _, err := runCmd(args...)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Count    int
				Patterns []struct {
					UUID    string
					Support int
					Share   string
				}
			}
			if err := json.Unmarshal([]byte(out), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Count != tc.count {
				t.Errorf("count=%d want %d: %s", payload.Count, tc.count, out)
			}
			for _, p := range payload.Patterns {
				if tc.kind == "corrections" && tc.count > 0 && p.UUID != "tag-3-0" {
					t.Errorf("paged UUID=%q want tag-3-0", p.UUID)
				}
				if tc.kind == "sequences" && (p.Support != tc.support || p.Share != "100.0%") {
					t.Errorf("support/share=%d/%s want %d/100.0%%", p.Support, p.Share, tc.support)
				}
			}
		})
	}
}
