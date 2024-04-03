package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

func eligibilityExec(t *testing.T, db *Database, query string, args ...any) {
	t.Helper()
	if _, err := db.db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func eligibilityItem(t *testing.T, db *Database, uuid, path, project, timestamp string, ordinal int) {
	t.Helper()
	eligibilityExec(t, db, `INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,project,content_type,timestamp) VALUES ('session',?,?,'assistant','error: fixture',?,?,'tool',NULLIF(?,''))`, path, ordinal, uuid, project, timestamp)
}

func TestPatternEligibilityUUIDPeers(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	const stamp = "2026-01-05T00:00:00Z"
	// The UUID anchor moved coordinates; the satellite retains its old path.
	eligibilityItem(t, db, "anchor", "/canonical", "wanted", stamp, 9)
	eligibilityItem(t, db, "peer-one", "/old", "other", stamp, 0)
	eligibilityItem(t, db, "peer-two", "/old", "other", stamp, 0)
	eligibilityExec(t, db, `INSERT INTO session_tags VALUES ('/canonical','keep'),('/old','reject')`)
	eligibilityExec(t, db, `INSERT INTO tool_events(message_uuid,source_path,ordinal,tool_name,command_head,is_error,extraction_version) VALUES ('anchor','/old',0,'Bash','go',1,1)`)
	eligibilityExec(t, db, `INSERT INTO message_templates(signature,normalization_version,template_text,first_seen,last_seen) VALUES ('fixture',2,'error: fixture',?,?)`, stamp, stamp)
	eligibilityExec(t, db, `INSERT INTO template_matches(template_id,item_uuid,source_path,ordinal) VALUES (1,'anchor','/old',0)`)
	eligibilityExec(t, db, `INSERT INTO correction_signals(item_uuid,source_path,ordinal,detector,confidence,extraction_version) VALUES ('anchor','/old',0,'test',1,3)`)
	for _, tc := range []struct {
		name, project, tag string
		want               int
	}{
		{"all", "", "", 1}, {"canonical project", "wanted", "", 1}, {"peer project", "other", "", 0},
		{"canonical tag", "", "keep", 1}, {"peer tag", "", "reject", 0}, {"conjunctive scope", "other", "keep", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := AggregateOptions{Project: tc.project, Tag: tc.tag}
			commands, err := db.AggregateCommands(opts)
			if err != nil {
				t.Fatal(err)
			}
			total := 0
			for _, r := range commands {
				total += r.Count
			}
			if total != tc.want {
				t.Errorf("flat commands=%d want %d", total, tc.want)
			}
			excluded := 99
			trend, err := db.AggregateCommandsTrend(opts, &excluded)
			if err != nil {
				t.Fatal(err)
			}
			total = 0
			for _, r := range trend {
				total += r.Count
			}
			if total != tc.want || excluded != 0 {
				t.Errorf("trend commands=%d excluded=%d want %d/0", total, excluded, tc.want)
			}
			failures, err := db.AggregateFailures(opts)
			if err != nil {
				t.Fatal(err)
			}
			total = 0
			for _, r := range failures {
				total += r.Count
				if r.SignalledEvents != tc.want {
					t.Errorf("flat coverage=%d want %d", r.SignalledEvents, tc.want)
				}
			}
			if total != tc.want {
				t.Errorf("flat failures=%d want %d", total, tc.want)
			}
			weekly, err := db.AggregateFailuresTrend(opts, &excluded)
			if err != nil {
				t.Fatal(err)
			}
			total = 0
			for _, r := range weekly {
				total += r.Count
				if r.SignalledEvents != tc.want {
					t.Errorf("trend coverage=%d want %d", r.SignalledEvents, tc.want)
				}
			}
			if total != tc.want || excluded != 0 {
				t.Errorf("trend failures=%d excluded=%d want %d/0", total, excluded, tc.want)
			}
			templates, err := db.AggregateTemplates(TemplateQueryOpts{Project: tc.project, Tag: tc.tag, MinSupport: 1})
			if err != nil {
				t.Fatal(err)
			}
			total = 0
			for _, r := range templates {
				total += r.OccurrenceCount
				if len(r.ProjectsAffected) != 1 || r.ProjectsAffected[0] != "wanted" {
					t.Errorf("template projects=%v", r.ProjectsAffected)
				}
			}
			if total != tc.want {
				t.Errorf("templates=%d want %d", total, tc.want)
			}
			corrections, err := db.AggregateCorrections(CorrectionAggOpts{Project: tc.project, Tag: tc.tag})
			if err != nil {
				t.Fatal(err)
			}
			if len(corrections) != tc.want {
				t.Errorf("corrections=%d want %d", len(corrections), tc.want)
			}
			{
				seqs, err := db.LoadToolSequences(LoadSequencesOpts{Project: tc.project, Tag: tc.tag})
				if err != nil {
					t.Fatal(err)
				}
				if len(seqs) != tc.want {
					t.Errorf("sequences=%d want %d", len(seqs), tc.want)
				}
			}
		})
	}
}

func TestPatternEligibilityHalfOpenDates(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	const after = "2026-01-05T00:00:00Z"
	const before = "2026-01-06T00:00:00Z"
	for i, stamp := range []string{"2026-01-04T23:59:59Z", after, "2026-01-05T12:00:00Z", before, ""} {
		uuid := fmt.Sprint("date-", i)
		path := fmt.Sprint("/date-", i)
		eligibilityItem(t, db, uuid, path, "p", stamp, 0)
		eligibilityExec(t, db, `INSERT INTO tool_events(message_uuid,source_path,ordinal,tool_name,command_head,is_error,extraction_version) VALUES (?,?,0,'Bash',?,1,1)`, uuid, path, uuid)
		eligibilityExec(t, db, `INSERT INTO message_templates(signature,normalization_version,template_text,first_seen,last_seen) VALUES (?,2,?,'2026-01-01','2026-01-01')`, uuid, uuid)
		eligibilityExec(t, db, `INSERT INTO template_matches(template_id,item_uuid,source_path,ordinal) VALUES (?,?,?,0)`, i+1, uuid, path)
	}
	for _, tc := range []struct {
		name, after, before string
		want                int
	}{
		{"lower inclusive", after, "", 3}, {"upper exclusive", "", before, 3}, {"half open", after, before, 2}, {"empty", after, after, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {

			var lower, upper *time.Time
			if tc.after != "" {
				v, err := time.Parse(time.RFC3339, tc.after)
				if err != nil {
					t.Fatal(err)
				}
				lower = &v
			}
			if tc.before != "" {
				v, err := time.Parse(time.RFC3339, tc.before)
				if err != nil {
					t.Fatal(err)
				}
				upper = &v
			}
			found, _, err := db.Search("fixture", models.SearchOptions{After: lower, Before: upper, ContentType: "tool", AllProjects: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(found) != tc.want {
				t.Errorf("search=%d want %d", len(found), tc.want)
			}
			listed, err := db.ListItemsV2(ListOptions{After: lower, Before: upper, AllProjects: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(listed) != tc.want {
				t.Errorf("list=%d want %d", len(listed), tc.want)
			}
			var lowerString, upperString *string
			if tc.after != "" {
				lowerString = &tc.after
			}
			if tc.before != "" {
				upperString = &tc.before
			}
			records, err := db.QueryIndexedRecords(IndexedRecordQuery{After: lowerString, Before: upperString})
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != tc.want {
				t.Errorf("records=%d want %d", len(records), tc.want)
			}
			opts := AggregateOptions{StartDate: tc.after, EndDate: tc.before}
			c, err := db.AggregateCommands(opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(c) != tc.want {
				t.Errorf("commands=%d want %d", len(c), tc.want)
			}
			f, err := db.AggregateFailures(opts)
			if err != nil {
				t.Fatal(err)
			}
			total := 0
			for _, r := range f {
				total += r.Count
				if r.SignalledEvents != tc.want {
					t.Errorf("coverage=%d want %d", r.SignalledEvents, tc.want)
				}
			}
			if total != tc.want {
				t.Errorf("failures=%d want %d", total, tc.want)
			}
			cw, err := db.AggregateCommandsTrend(opts, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(cw) != tc.want {
				t.Errorf("commands trend=%d want %d", len(cw), tc.want)
			}
			fw, err := db.AggregateFailuresTrend(opts, nil)
			if err != nil {
				t.Fatal(err)
			}
			total = 0
			for _, r := range fw {
				total += r.Count
			}
			if total != tc.want {
				t.Errorf("failures trend=%d want %d", total, tc.want)
			}
			tm, err := db.AggregateTemplates(TemplateQueryOpts{After: tc.after, Before: tc.before, MinSupport: 1})
			if err != nil {
				t.Fatal(err)
			}
			if len(tm) != tc.want {
				t.Errorf("templates=%d want %d", len(tm), tc.want)
			}
			seq, err := db.LoadToolSequences(LoadSequencesOpts{After: tc.after, Before: tc.before})
			if err != nil {
				t.Fatal(err)
			}
			if len(seq) != tc.want {
				t.Errorf("sequences=%d want %d", len(seq), tc.want)
			}
		})
	}
}

func TestPatternEligibilitySequencesUseCanonicalSession(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	for ordinal, tool := range []string{"Read", "Write"} {
		uuid := fmt.Sprint("session-anchor-", ordinal)
		eligibilityItem(t, db, uuid, "/canonical-session", "p", "2026-01-05T00:00:00Z", ordinal+10)
		eligibilityExec(t, db, `INSERT INTO tool_events(message_uuid,source_path,ordinal,tool_name,extraction_version) VALUES (?,?,0,?,1)`, uuid, fmt.Sprint("/old-session-", ordinal), tool)
	}
	eligibilityExec(t, db, `INSERT INTO session_tags VALUES ('/canonical-session','keep')`)
	for _, tag := range []string{"", "keep", "missing"} {
		seqs, err := db.LoadToolSequences(LoadSequencesOpts{Tag: tag})
		if err != nil {
			t.Fatal(err)
		}
		if tag == "missing" {
			if len(seqs) != 0 {
				t.Fatalf("missing tag=%+v", seqs)
			}
			continue
		}
		if len(seqs) != 1 || seqs[0].SessionID != "/canonical-session" || len(seqs[0].Items) != 2 {
			t.Errorf("tag=%q canonical sequence=%+v", tag, seqs)
		}
	}
}

func TestPatternEligibilitySameCoordinateAndLegacyFallback(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	const stamp = "2026-01-05T00:00:00Z"
	eligibilityItem(t, db, "proven", "/same", "wanted", stamp, 0)
	eligibilityItem(t, db, "historical-peer", "/same", "other", stamp, 0)
	eligibilityItem(t, db, "unique", "/unique", "wanted", stamp, 0)
	eligibilityItem(t, db, "empty-uuid-anchor", "/empty", "wanted", stamp, 0)
	eligibilityExec(t, db, `INSERT INTO session_tags VALUES ('/same','keep'),('/unique','keep'),('/empty','keep')`)
	for _, row := range []struct{ uuid, path string }{
		{"proven", "/same"}, {"", "/unique"}, {"", "/empty"}, {"missing-proven-uuid", "/missing"},
	} {
		// Use NULL for the legacy unique coordinate and empty string for the other.
		var uuid any = row.uuid
		if row.path == "/unique" {
			uuid = nil
		}
		eligibilityExec(t, db, `INSERT INTO tool_events(message_uuid,source_path,ordinal,tool_name,command_head,is_error,extraction_version) VALUES (?,?,0,'Bash','go',1,1)`, uuid, row.path)
	}
	eligibilityItem(t, db, "missing-peer", "/missing", "other", stamp, 0)
	// UUID-less ambiguous metadata may not borrow any peer's scope.
	eligibilityItem(t, db, "ambiguous-a", "/ambiguous", "wanted", stamp, 0)
	eligibilityItem(t, db, "ambiguous-b", "/ambiguous", "other", stamp, 0)
	eligibilityExec(t, db, `INSERT INTO tool_events(source_path,ordinal,tool_name,command_head,is_error,extraction_version) VALUES ('/ambiguous',0,'Bash','go',1,0)`)
	for _, tc := range []struct {
		project string
		want    int
	}{{"", 5}, {"wanted", 3}, {"other", 0}} {
		got, err := db.AggregateCommands(AggregateOptions{Project: tc.project})
		if err != nil {
			t.Fatal(err)
		}
		total := 0
		for _, r := range got {
			total += r.Count
		}
		if total != tc.want {
			t.Errorf("project=%q flat=%d want %d", tc.project, total, tc.want)
		}
		excluded := 0
		trend, err := db.AggregateCommandsTrend(AggregateOptions{Project: tc.project}, &excluded)
		if err != nil {
			t.Fatal(err)
		}
		total = 0
		for _, r := range trend {
			total += r.Count
		}
		want := tc.want
		if tc.project == "" {
			want = 3
		}
		if total != want || (tc.project == "" && excluded != 2) {
			t.Errorf("project=%q trend=%d excluded=%d", tc.project, total, excluded)
		}
	}
	// Scope is resolved before applying filters; the wanted project must not
	// make the ambiguous coordinate appear unique after removing its other peer.
	seqs, err := db.LoadToolSequences(LoadSequencesOpts{Project: "wanted", Tag: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 3 {
		t.Errorf("scoped sessions=%+v want 3", seqs)
	}
}

func TestPatternEligibilityUndatedDiagnostics(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	eligibilityItem(t, db, "undated", "/undated", "wanted", "", 0)
	eligibilityItem(t, db, "dated-peer", "/undated", "other", "2026-01-05T00:00:00Z", 0)
	eligibilityExec(t, db, `INSERT INTO session_tags VALUES ('/undated','keep')`)
	eligibilityExec(t, db, `INSERT INTO tool_events(message_uuid,source_path,ordinal,tool_name,command_head,is_error,extraction_version) VALUES ('undated','/undated',0,'Bash','go',1,1)`)
	for _, project := range []string{"", "wanted", "other"} {
		opts := AggregateOptions{Project: project, Tag: "keep", StartDate: "2026-01-01T00:00:00Z"}
		excluded := 99
		commands, err := db.AggregateCommandsTrend(opts, &excluded)
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		if project == "other" {
			want = 0
		}
		if len(commands) != 0 || excluded != want {
			t.Errorf("project=%q commands=%+v excluded=%d want %d", project, commands, excluded, want)
		}
		failures, err := db.AggregateFailuresTrend(opts, &excluded)
		if err != nil {
			t.Fatal(err)
		}
		if len(failures) != 0 || excluded != want {
			t.Errorf("project=%q failures=%+v excluded=%d want %d", project, failures, excluded, want)
		}
	}
}

func TestPatternEligibilityPaginationAfterScope(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	for i, tool := range []string{"First", "Second", "Third"} {
		uuid, path := fmt.Sprint("page-", i), fmt.Sprint("/page-", i)
		eligibilityItem(t, db, uuid, path, "p", "2026-01-05T00:00:00Z", 0)
		eligibilityExec(t, db, `INSERT INTO tool_events(message_uuid,source_path,ordinal,tool_name,command_head,is_error,extraction_version) VALUES (?,?,0,?,'go',1,1)`, uuid, path, tool)
		if i > 0 {
			eligibilityExec(t, db, `INSERT INTO session_tags VALUES (?,'keep')`, path)
		}
	}
	opts := AggregateOptions{Tag: "keep", Offset: 1, Limit: 1}
	commands, err := db.AggregateCommands(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].ToolName != "Third" {
		t.Errorf("commands=%+v", commands)
	}
	commandTrend, err := db.AggregateCommandsTrend(opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(commandTrend) != 1 || commandTrend[0].ToolName != "Third" {
		t.Errorf("commands trend=%+v", commandTrend)
	}
	failures, err := db.AggregateFailures(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 || failures[0].ToolName != "Third" || failures[0].SignalledEvents != 2 {
		t.Errorf("failures=%+v", failures)
	}
	failureTrend, err := db.AggregateFailuresTrend(opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(failureTrend) != 1 || failureTrend[0].ToolName != "Third" || failureTrend[0].SignalledEvents != 2 {
		t.Errorf("failures trend=%+v", failureTrend)
	}
}
