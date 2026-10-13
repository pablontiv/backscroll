package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

func TestListPaginationValidation(t *testing.T) {
	for _, args := range [][]string{{"--order", "timestamp:descending"}, {"--order", "timestamp:asc-extra"}, {"--recent", "0", "--limit", "0"}, {"--recent", "1", "--order", "timestamp:asc"}, {"--limit", "-1"}, {"--offset", "-1"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			path, cleanup := testEnv(t)
			defer cleanup()
			if _, _, e := runCmd(append([]string{"list", "--all-projects"}, args...)...); e == nil {
				t.Fatal("invalid invocation accepted")
			}
			if _, e := os.Stat(path + ".startup-sync.lock"); !os.IsNotExist(e) {
				t.Fatalf("startup lock touched: %v", e)
			}
		})
	}
}

func TestTemplatesUnlimitedOffsetCLI(t *testing.T) {
	path, cleanup := testEnv(t)
	defer cleanup()
	db, e := storage.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 25; i++ {
		r, e := db.DB().Exec(`INSERT INTO message_templates(signature,normalization_version,template_text,first_seen,last_seen) VALUES (?,2,?,'2026-01-01','2026-01-01')`, fmt.Sprintf("sig%02d", i), fmt.Sprintf("error%02d", i))
		if e != nil {
			t.Fatal(e)
		}
		id, _ := r.LastInsertId()
		if _, e := db.DB().Exec(`INSERT INTO template_matches(template_id,item_uuid,source_path,ordinal) VALUES (?,?,?,0)`, id, fmt.Sprint(i), fmt.Sprint(i)); e != nil {
			t.Fatal(e)
		}
	}
	db.Close()
	out, _, e := runCmd("patterns", "--kind", "templates", "--all-projects", "--min-support", "1", "--limit", "0", "--offset", "1", "--json")
	if e != nil {
		t.Fatal(e)
	}
	var payload struct{ Count int }
	if e := json.Unmarshal([]byte(out), &payload); e != nil {
		t.Fatal(e)
	}
	if payload.Count != 24 {
		t.Fatalf("count=%d want 24: %s", payload.Count, out)
	}
}

func TestListEffectiveLimitCLI(t *testing.T) {
	path, cleanup := testEnv(t)
	defer cleanup()
	db, e := storage.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	for i := 24; i >= 0; i-- {
		name := fmt.Sprintf("/session%02d", i)
		if e := db.SyncFiles([]storage.IndexedFile{{SourcePath: name, Source: "session", Hash: name, Project: "p", Messages: []storage.IndexedMessage{{UUID: name, Text: "text", Ordinal: 0, ContentType: "text", Timestamp: "2026-01-01T00:00:00Z"}}}}); e != nil {
			t.Fatal(e)
		}
	}
	db.Close()
	for _, tc := range []struct {
		args []string
		want int
	}{{nil, 20}, {[]string{"--order", "timestamp:asc"}, 20}, {[]string{"--limit", "0", "--offset", "1"}, 24}, {[]string{"--recent", "0", "--offset", "1"}, 24}, {[]string{"--recent", "3", "--offset", "1"}, 3}} {
		out, _, e := runCmd(append([]string{"list", "--all-projects", "--json"}, tc.args...)...)
		if e != nil {
			t.Fatal(e)
		}
		var payload struct{ Count int }
		if e := json.Unmarshal([]byte(out), &payload); e != nil {
			t.Fatal(e)
		}
		if payload.Count != tc.want {
			t.Fatalf("%v count=%d want %d", tc.args, payload.Count, tc.want)
		}
	}
}
