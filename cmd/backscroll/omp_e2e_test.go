package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

func installOMPFixture(t *testing.T, reasoning bool) (string, string, []byte) {
	t.Helper()
	dbPath, _ := testEnv(t)
	root := filepath.Join(os.Getenv("HOME"), ".omp", "agent", "sessions")
	source := filepath.Join(root, "bucket-not-project-identity", "session.jsonl")
	fixture, err := os.ReadFile(filepath.Join(fixturesDir(), "omp-v3.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, source, string(fixture))
	// The preset itself, with only its root rewritten and the explicit reasoning
	// option selected before ingestion, is the integration contract.
	preset, err := os.ReadFile("../../inputs/omp.inputs.toml")
	if err != nil {
		t.Fatal(err)
	}
	manifest := replaceManifestRoots(t, "inputs/omp.inputs.toml", string(preset), root)
	if reasoning {
		manifest = strings.ReplaceAll(manifest, "index_reasoning = false", "index_reasoning = true")
	}
	writeFile(t, filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs", "omp.inputs.toml"), manifest)
	writeFile(t, filepath.Join(root, "bucket-not-project-identity", "session", "child.jsonl"), strings.ReplaceAll(string(fixture), "ompuserquartz", "ompchildquartz"))
	return dbPath, source, fixture
}

func ompRecall(t *testing.T, marker string, extra ...string) []map[string]any {
	t.Helper()
	args := append([]string{"search", "--text", marker, "--all-projects", "--lexical-only", "--json"}, extra...)
	out, stderr, err := runCmd(args...)
	if err != nil {
		t.Fatalf("recall %s: %v %s", marker, err, stderr)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("JSON %s: %v", out, err)
	}
	return rows
}

func TestOMPReaderRegistered(t *testing.T) {
	reg := newDefaultAutoSyncRegistry()
	r, ok := reg.Get("omp")
	if !ok || r.Name() != "omp" {
		t.Fatal("native OMP reader not registered")
	}
	pi, ok := reg.Get("pi")
	if !ok || pi == r {
		t.Fatal("OMP must not alias Pi")
	}
}

func TestOMPV3E2E(t *testing.T) {
	dbPath, source, _ := installOMPFixture(t, false)
	for _, tc := range []struct{ marker, role, kind string }{
		{"ompuserquartz", "user", "text"}, {"ompanswerquartz", "assistant", "text"},
		{"omptoolquartz", "assistant", "tool"}, {"ompoutputquartz", "tool", "tool"},
		{"ompdeveloperquartz", "developer", "text"}, {"ompsyntheticquartz", "user", "text"},
		{"ompagentquartz", "user", "text"}, {"ompunknownquartz", "user", "text"}, {"ompbranchquartz", "assistant", "text"},
	} {
		// Project identity comes from the header cwd, never the bucket directory.
		out, stderr, err := runCmd("search", "--text", tc.marker, "--project", "omp-fixture-project", "--source", "session", "--source-path", source, "--role", tc.role, "--content-type", tc.kind, "--after", "2026-09-01", "--before", "2026-09-02", "--lexical-only", "--json")
		if err != nil {
			t.Fatalf("%s: %v %s", tc.marker, err, stderr)
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s: want one row got %s; stderr=%s", tc.marker, out, stderr)
		}
	}
	for _, marker := range []string{"ompignoredtitle", "ompignoredextra", "ompignoredprovider", "ompignoredimage", "ompignoredblob", "ompignoreddetails", "ompignoredsignature", "ompignoredredacted", "ompignoredencrypted", "ompignoredadmin", "ompignoredderived", "ompignoredspecial", "ompignoredtimestamp", "ompignoredidentity", "ompignoredmalformed", "ompreasonquartz", "ompchildquartz"} {
		if rows := ompRecall(t, marker); len(rows) != 0 {
			t.Fatalf("excluded %s leaked: %+v", marker, rows)
		}
	}
	for _, filter := range [][]string{{"--role", "assistant"}, {"--source", "omp"}, {"--source-path", source + ".absent"}, {"--after", "2026-09-02"}, {"--before", "2026-09-01"}} {
		if rows := ompRecall(t, "ompuserquartz", filter...); len(rows) != 0 {
			t.Fatalf("filter %v ignored: %+v", filter, rows)
		}
	}
	db, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var human, automation, unknown int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE origin='human'`).Scan(&human); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE origin='automation'`).Scan(&automation); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE origin='unknown'`).Scan(&unknown); err != nil {
		t.Fatal(err)
	}
	if human != 1 || automation != 3 || unknown != 1 {
		t.Fatalf("native provenance persisted: human=%d automation=%d unknown=%d", human, automation, unknown)
	}
	var name, head string
	var code, isError int
	if err := db.DB().QueryRow(`SELECT tool_name, command_head, exit_code, is_error FROM tool_events WHERE source_path=?`, source).Scan(&name, &head, &code, &isError); err != nil {
		t.Fatal(err)
	}
	if name != "bash" || head != "echo" || code != 7 || isError != 1 {
		t.Fatalf("tool metadata: %s %s %d %d", name, head, code, isError)
	}
}

func TestOMPPresetIncrementalAndPerennialRecall(t *testing.T) {
	dbPath, source, fixture := installOMPFixture(t, true)
	recall := func(marker string) {
		t.Helper()
		if rows := ompRecall(t, marker); len(rows) != 1 {
			t.Fatalf("%s: %+v", marker, rows)
		}
	}
	recall("ompuserquartz")
	recall("ompreasonquartz")
	// Capture physical row ID as well as UUID, not just recall count.
	identity := func() (int64, string) {
		t.Helper()
		db, err := storage.OpenReadOnly(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var id int64
		var uuid string
		if err := db.DB().QueryRow(`SELECT id,uuid FROM search_items WHERE text LIKE '%ompuserquartz%'`).Scan(&id, &uuid); err != nil {
			t.Fatal(err)
		}
		return id, uuid
	}
	id, uuid := identity()
	recall("ompuserquartz")
	appendRecord := `{"type":"message","id":"append","parentId":"u","timestamp":"2026-09-01T12:00:05Z","message":{"role":"user","attribution":"user","content":"ompappendquartz"}}` + "\n"
	writeFile(t, source, string(fixture)+appendRecord)
	recall("ompappendquartz")
	recall("ompuserquartz")
	afterID, afterUUID := identity()
	if id != afterID || uuid != afterUUID {
		t.Fatalf("identity replaced: %d %s -> %d %s", id, uuid, afterID, afterUUID)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runCmd("rebuild"); err != nil {
		t.Fatalf("rebuild: %v %s", err, stderr)
	}
	recall("ompappendquartz")
	recall("ompuserquartz")
	recall("ompreasonquartz")
	afterID, afterUUID = identity()
	if id != afterID || uuid != afterUUID {
		t.Fatal("rebuild replaced perennial identity")
	}
}

func TestOMPAppendedToolResultEnrichesPerennialCall(t *testing.T) {
	dbPath, source, _ := installOMPFixture(t, false)
	const header = `{"type":"session","version":3,"id":"append-session","cwd":"/workspace/omp-fixture-project","timestamp":"2026-09-01T12:00:00Z"}` + "\n"
	const call = `{"type":"message","id":"call","timestamp":"2026-09-01T12:00:01Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"c","name":"bash","arguments":{"command":"backscroll search --text ompechoquartz"}}]}}` + "\n"
	const result = `{"type":"message","id":"result","timestamp":"2026-09-01T12:00:02Z","message":{"role":"toolResult","toolCallId":"c","toolName":"bash","isError":true,"details":{"exitCode":7,"__interrupted":true},"content":[{"type":"text","text":"ompechoquartz result"}]}}` + "\n"
	writeFile(t, source, header+call)
	if rows := ompRecall(t, "ompechoquartz", "--content-type", "tool"); len(rows) != 1 {
		t.Fatalf("call-only recall: %+v", rows)
	}
	type snapshot struct {
		itemID, eventID       int64
		text, uuid, origin    string
		itemEpoch, eventEpoch int
		interrupted           bool
		isError, code         *int
	}
	read := func() snapshot {
		t.Helper()
		db, err := storage.OpenReadOnly(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var s snapshot
		if err := db.DB().QueryRow(`SELECT s.id,e.id,s.text,s.uuid,s.origin,s.extraction_version,e.extraction_version,s.was_interrupted,e.is_error,e.exit_code FROM search_items s JOIN tool_events e ON e.message_uuid=s.uuid WHERE s.uuid='omp:append-session:call#b0'`).Scan(&s.itemID, &s.eventID, &s.text, &s.uuid, &s.origin, &s.itemEpoch, &s.eventEpoch, &s.interrupted, &s.isError, &s.code); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := read()
	if before.interrupted || before.isError != nil || before.code != nil {
		t.Fatalf("unproven call: %+v", before)
	}
	writeFile(t, source, header+call+result)
	if rows := ompRecall(t, "ompechoquartz"); len(rows) != 0 {
		t.Fatalf("direct-search echo leaked: %+v", rows)
	}
	if rows := ompRecall(t, "ompechoquartz", "--content-type", "tool"); len(rows) != 2 {
		t.Fatalf("explicit tool retrieval: %+v", rows)
	}
	assertProof := func() {
		t.Helper()
		after := read()
		if after.itemID != before.itemID || after.eventID != before.eventID || after.text != before.text || after.uuid != before.uuid || after.origin != before.origin || after.itemEpoch != before.itemEpoch || after.eventEpoch != before.eventEpoch || !after.interrupted || after.isError == nil || *after.isError != 1 || after.code == nil || *after.code != 7 {
			t.Fatalf("appended result proof/identity: before=%+v after=%+v", before, after)
		}
	}
	assertProof()
	writeFile(t, source, header+call)
	ompRecall(t, "ompechoquartz", "--content-type", "tool")
	assertProof()
	if rows := ompRecall(t, "ompechoquartz"); len(rows) != 0 {
		t.Fatalf("partial reparse erased echo proof: %+v", rows)
	}
}

func TestOMPReasoningTogglePreservesPerennialToolCoordinates(t *testing.T) {
	dbPath, source, _ := installOMPFixture(t, true)
	const initial = `{"type":"session","version":3,"id":"toggle","cwd":"/workspace/omp-fixture-project","timestamp":"2026-09-01T12:00:00Z"}
{"type":"message","id":"a","timestamp":"2026-09-01T12:00:01Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"omptogglereason"},{"type":"toolCall","id":"ca","name":"bash","arguments":{"command":"go test omptogglea"}}]}}
`
	const appended = `{"type":"message","id":"b","timestamp":"2026-09-01T12:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"cb","name":"bash","arguments":{"command":"make omptoggleb"}}]}}
{"type":"message","id":"rb","timestamp":"2026-09-01T12:00:03Z","message":{"role":"toolResult","toolCallId":"cb","toolName":"bash","isError":true,"details":{"exitCode":7},"content":[{"type":"text","text":"error: omptogglefailure"}]}}
`
	writeFile(t, source, initial)
	if rows := ompRecall(t, "omptogglea", "--content-type", "tool"); len(rows) != 1 {
		t.Fatalf("initial call: %+v", rows)
	}
	type anchor struct {
		itemID, eventID            int64
		ordinal, epoch, eventEpoch int
		text, uuid, origin         string
	}
	readA := func() anchor {
		t.Helper()
		db, err := storage.OpenReadOnly(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var a anchor
		if err := db.DB().QueryRow(`SELECT s.id,e.id,s.ordinal,s.extraction_version,e.extraction_version,s.text,s.uuid,s.origin FROM search_items s JOIN tool_events e ON e.message_uuid=s.uuid WHERE s.uuid='omp:toggle:a#b1'`).Scan(&a.itemID, &a.eventID, &a.ordinal, &a.epoch, &a.eventEpoch, &a.text, &a.uuid, &a.origin); err != nil {
			t.Fatal(err)
		}
		return a
	}
	before := readA()
	manifestPath := filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs", "omp.inputs.toml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, manifestPath, strings.ReplaceAll(string(manifest), "index_reasoning = true", "index_reasoning = false"))
	writeFile(t, source, initial+appended)
	if rows := ompRecall(t, "omptoggleb", "--content-type", "tool"); len(rows) != 1 {
		t.Fatalf("appended call: %+v", rows)
	}
	if after := readA(); after != before {
		t.Fatalf("retained call changed: before=%+v after=%+v", before, after)
	}
	db, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var collisions, events, anchors int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM (SELECT ordinal FROM search_items WHERE source_path=? GROUP BY ordinal HAVING COUNT(*)>1)`, source).Scan(&collisions); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM tool_events WHERE source_path=?`, source).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM tool_events e JOIN search_items s ON s.source_path=e.source_path AND s.ordinal=e.ordinal AND s.uuid=e.message_uuid WHERE e.source_path=?`, source).Scan(&anchors); err != nil {
		t.Fatal(err)
	}
	if collisions != 0 || events != 2 || anchors != 2 {
		t.Fatalf("coordinates: collisions=%d events=%d anchors=%d", collisions, events, anchors)
	}
	// Exercise actual CLI consumers rather than only counting physical rows.
	for _, kind := range []string{"commands", "failures", "sequences"} {
		out, stderr, err := runCmd("patterns", "--kind", kind, "--all-projects", "--min-support", "1", "--min-length", "2", "--max-length", "2", "--json")
		if err != nil {
			t.Fatalf("%s: %v %s", kind, err, stderr)
		}
		var payload struct {
			Count    int `json:"count"`
			Patterns []struct {
				CommandHead string `json:"command_head"`
				Count       int    `json:"count"`
				ExitCode    *int   `json:"exit_code"`
				Support     int    `json:"support"`
				Pattern     string `json:"pattern"`
			} `json:"patterns"`
		}
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range payload.Patterns {
			switch kind {
			case "commands":
				found = found || (p.CommandHead == "make" && p.Count == 1)
			case "failures":
				found = found || (p.ExitCode != nil && *p.ExitCode == 7 && p.Count == 1)
			case "sequences":
				found = found || (p.Support == 1 && strings.Contains(p.Pattern, " → "))
			}
		}
		if !found {
			t.Fatalf("%s omitted appended call B: %s", kind, out)
		}
	}
}

// Native OMP calls retain their project/tag coordinates after reasoning
// changes. Filtered pattern consumers must not join them to a displaced row.
func TestOMPFilteredPatternConsumersAfterReasoningChange(t *testing.T) {
	_, source, _ := installOMPFixture(t, true)
	const header = `{"type":"session","version":3,"id":"filters","cwd":"/workspace/omp-fixture-project","timestamp":"2026-09-01T12:00:00Z"}` + "\n"
	const entries = `{"type":"message","id":"a","timestamp":"2026-09-01T12:00:01Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"filterreason"},{"type":"toolCall","id":"a","name":"bash","arguments":{"command":"go test filterfirst"}}]}}
{"type":"message","id":"ar","timestamp":"2026-09-01T12:00:02Z","message":{"role":"toolResult","toolCallId":"a","toolName":"bash","isError":true,"details":{"exitCode":7},"content":[{"type":"text","text":"error: filterfailure 7"}]}}
{"type":"message","id":"b","timestamp":"2026-09-01T12:00:03Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"b","name":"bash","arguments":{"command":"make filtersecond"}}]}}
{"type":"message","id":"br","timestamp":"2026-09-01T12:00:04Z","message":{"role":"toolResult","toolCallId":"b","toolName":"bash","isError":true,"details":{"exitCode":9},"content":[{"type":"text","text":"error: filterfailure 9"}]}}
`
	writeFile(t, source, header+entries)
	if rows := ompRecall(t, "filterfirst", "--content-type", "tool"); len(rows) != 1 {
		t.Fatalf("initial call: %+v", rows)
	}
	manifestPath := filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs", "omp.inputs.toml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, manifestPath, strings.ReplaceAll(string(manifest), "index_reasoning = true", "index_reasoning = false"))
	writeFile(t, source, header+entries+`{"type":"message","id":"u","timestamp":"2026-09-01T12:00:05Z","message":{"role":"user","attribution":"user","content":"run tests"}}`+"\n")
	for _, kind := range []string{"commands", "failures"} {
		for _, trend := range []bool{false, true} {
			args := []string{"patterns", "--kind", kind, "--project", "omp-fixture-project", "--tag", "testing", "--limit", "1", "--offset", "1", "--json"}
			if trend {
				args = append(args, "--trend")
			}
			out, stderr, err := runCmd(args...)
			if err != nil {
				t.Fatalf("%s trend=%t: %v %s", kind, trend, err, stderr)
			}
			// Ties are deterministic: go precedes make and exit 7 precedes exit 9.
			marker := `"command_head":"make"`
			if kind == "failures" {
				marker = `"exit_code":9`
			}
			if !strings.Contains(out, marker) {
				t.Fatalf("filtered page lost appended call: %s", out)
			}
			for _, format := range []string{"--json", "--robot", ""} {
				emptyArgs := []string{"patterns", "--kind", kind, "--project", "missing-project", "--trend"}
				if format != "" {
					emptyArgs = append(emptyArgs, format)
				}
				empty, stderr, err := runCmd(emptyArgs...)
				if err != nil {
					t.Fatalf("empty trend: %v %s", err, stderr)
				}
				if strings.Contains(empty, "filter") || strings.Contains(empty, "make") {
					t.Fatalf("project filter leaked: %s", empty)
				}
				if format == "--json" && !strings.Contains(empty, `"trends":[]`) {
					t.Fatalf("empty trend contract: %s", empty)
				}
			}
		}
	}
	// Template pagination past the end is empty rather than out-of-range or
	// leaking templates from a different project.
	out, stderr, err := runCmd("patterns", "--kind", "templates", "--project", "omp-fixture-project", "--min-support", "1", "--offset", "100", "--json")
	if err != nil {
		t.Fatalf("template page: %v %s", err, stderr)
	}
	var payload struct {
		Count    int   `json:"count"`
		Patterns []any `json:"patterns"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 0 || len(payload.Patterns) != 0 {
		t.Fatalf("template page: %s", out)
	}
}

// Human and robot trends must publish actual native calls, including missing
// exit proof, instead of accidentally routing non-JSON output to empty pages.
func TestOMPNonJSONTrendOutputRetainsSignals(t *testing.T) {
	_, source, _ := installOMPFixture(t, false)
	writeFile(t, source, `{"type":"session","version":3,"id":"trend","cwd":"/workspace/omp-fixture-project","timestamp":"2026-09-01T12:00:00Z"}
{"type":"message","id":"a","timestamp":"2026-09-01T12:00:01Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"a","name":"bash","arguments":{"command":"go test trendprobe"}}]}}
{"type":"message","id":"ar","timestamp":"2026-09-01T12:00:02Z","message":{"role":"toolResult","toolCallId":"a","toolName":"bash","isError":true,"details":{"exitCode":7},"content":[{"type":"text","text":"error: trendprobe"}]}}
{"type":"message","id":"b","timestamp":"2026-09-01T12:00:03Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"b","name":"bash","arguments":{"command":"make trendprobe"}}]}}
{"type":"message","id":"br","timestamp":"2026-09-01T12:00:04Z","message":{"role":"toolResult","toolCallId":"b","toolName":"bash","isError":true,"content":[{"type":"text","text":"error: trendprobe"}]}}
`)
	for _, kind := range []string{"commands", "failures"} {
		for _, format := range []string{"", "--robot"} {
			args := []string{"patterns", "--kind", kind, "--project", "omp-fixture-project", "--trend"}
			if format != "" {
				args = append(args, format)
			}
			out, stderr, err := runCmd(args...)
			if err != nil {
				t.Fatalf("%s %s: %v %s", kind, format, err, stderr)
			}
			want := []string{"2026-W35"}
			if kind == "commands" {
				want = append(want, "go", "make")
			} else if format == "--robot" {
				want = append(want, "exit_code=7", "exit_code=?", "coverage=1/2")
			} else {
				want = append(want, "exit_code=7", "exit_code=?", "1 occurrences")
			}
			for _, marker := range want {
				if !strings.Contains(out, marker) {
					t.Fatalf("%s %s lost %q: %s", kind, format, marker, out)
				}
			}
			if strings.Contains(out, "No patterns found") {
				t.Fatalf("nonempty native trend reported empty: %s", out)
			}
		}
	}
}
