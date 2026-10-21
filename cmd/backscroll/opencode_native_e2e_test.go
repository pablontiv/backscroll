package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/pablontiv/backscroll/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCodeNativeSessionsCLI(t *testing.T) {
	e := newQueryEchoE2E(t)
	root := e.addReaderManifest("opencode", "opencode")
	original := filepath.Join(t.TempDir(), "original.db")
	source := openOpenCodeFixture(t, original)
	alpha, beta := filepath.Join(e.root, "project-a"), filepath.Join(e.root, "project-b")
	if _, err := source.Exec(`CREATE TABLE session(id TEXT PRIMARY KEY,directory TEXT);INSERT INTO session VALUES('s1',?),('s2',?),('s3',NULL)`, alpha, beta); err != nil {
		t.Fatal(err)
	}
	for i, sid := range []string{"s1", "s2"} {
		id := fmt.Sprintf("m%d", i)
		ts := 1767225600000 + i
		if _, err := source.Exec(`INSERT INTO message VALUES(?,?,?,?,?)`, id, sid, ts, ts, `{"role":"assistant"}`); err != nil {
			t.Fatal(err)
		}
		text := map[string]any{"type": "text", "text": "nativequartz orchard"}
		if i == 0 {
			text["text"] = "nativequartz go test"
		}
		command := "git status"
		if i == 0 {
			command = "go test"
		}
		for j, part := range []map[string]any{text, {"type": "tool", "tool": "Bash", "state": map[string]any{"input": map[string]any{"command": command}, "output": "completed"}}} {
			data, _ := json.Marshal(part)
			if _, err := source.Exec(`INSERT INTO part VALUES(?,?,?,?,?,?)`, fmt.Sprintf("p%d-%d", i, j), id, sid, ts, ts, string(data)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	// Copy a closed native DB into the manifest capability; IDs cannot depend on filename.
	bytes, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	capability := filepath.Join(root, "opencode.db")
	if err := os.WriteFile(capability, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(e.root, "home", ".config", "backscroll")
	if err := os.MkdirAll(registry, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(registry, "projects.toml"), []byte(fmt.Sprintf("[[projects]]\nid='alpha-registry'\nroots=[%q]\n[[projects]]\nid='beta-registry'\nroots=[%q]\n", alpha, beta)), 0600); err != nil {
		t.Fatal(err)
	}
	var listing struct {
		Count    int
		Sessions []storage.SessionEntry
	}
	if err := json.Unmarshal([]byte(e.run("list", "--all-projects", "--json")), &listing); err != nil || listing.Count != 2 {
		t.Fatalf("list=%+v err=%v", listing, err)
	}
	for _, s := range listing.Sessions {
		if s.Path != capability || s.SessionID == "" || (s.Project != "alpha-registry" && s.Project != "beta-registry") {
			t.Fatalf("ownership=%+v", s)
		}
	}
	var found []struct {
		UUID      *string
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(e.run("search", "--text", "nativequartz", "--project", "alpha-registry", "--fields", "minimal", "--json")), &found); err != nil || len(found) != 1 || found[0].SessionID != "s1" {
		t.Fatalf("search=%+v err=%v", found, err)
	}
	var neighborhood struct{ Records []contextRecordOutput }
	if err := json.Unmarshal([]byte(e.run("context", "--uuid", *found[0].UUID, "--before", "50", "--after", "50", "--json", "--max-tokens", "16384")), &neighborhood); err != nil || len(neighborhood.Records) != 3 {
		t.Fatalf("context=%+v err=%v", neighborhood, err)
	}
	for _, r := range neighborhood.Records {
		if r.SessionID != "s1" || r.Project != "alpha-registry" {
			t.Fatalf("context crossed session: %+v", r)
		}
	}
	if err := json.Unmarshal([]byte(e.run("search", "--text", "nativequartz", "--all-projects", "--tag", "testing", "--fields", "minimal", "--json")), &found); err != nil || len(found) != 1 || found[0].SessionID != "s1" {
		t.Fatalf("tags crossed sessions: %+v err=%v", found, err)
	}
	e.run("patterns", "--kind", "sequences", "--project", "alpha-registry", "--min-support", "1", "--min-length", "1", "--json")
	index, err := storage.OpenReadOnly(e.database)
	if err != nil {
		t.Fatal(err)
	}
	seq, err := index.LoadToolSequences(storage.LoadSequencesOpts{})
	if err != nil || len(seq) != 2 {
		t.Fatalf("sequences=%+v err=%v", seq, err)
	}
	var uuid string
	var ordinal int
	var rowid int64
	if err := index.DB().QueryRow(`SELECT uuid,ordinal,id FROM search_items WHERE text='nativequartz go test'`).Scan(&uuid, &ordinal, &rowid); err != nil {
		t.Fatal(err)
	}
	index.Close()
	source, err = sql.Open("sqlite", capability)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`UPDATE message SET time_created=-1 WHERE id='m1';UPDATE part SET data='{"type":"text","text":"changed nativequartz"}' WHERE id='p0-0';INSERT INTO message VALUES('added','s3',0,0,'{"role":"user"}');INSERT INTO part VALUES('added-part','added','s3',0,0,'{"type":"text","text":"fallbacknativequartz"}')`); err != nil {
		t.Fatal(err)
	}
	source.Close()
	e.run("status", "--json")
	index, err = storage.OpenReadOnly(e.database)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	var gotOrdinal int
	var gotID int64
	var gotText, gotSession, gotProject string
	if err := index.DB().QueryRow(`SELECT ordinal,id,text,session_id,project FROM search_items WHERE uuid=?`, uuid).Scan(&gotOrdinal, &gotID, &gotText, &gotSession, &gotProject); err != nil {
		t.Fatal(err)
	}
	if gotOrdinal != ordinal || gotID != rowid || gotText != "nativequartz go test" || gotSession != "s1" || gotProject != "alpha-registry" {
		t.Fatal("mutation or reordering replaced retained ownership")
	}
	if err := index.DB().QueryRow(`SELECT project,project_evidence FROM search_items WHERE session_id='s3'`).Scan(&gotProject, &gotSession); err != nil || gotSession != "unknown" {
		t.Fatalf("absent native cwd proof=%s/%s err=%v", gotProject, gotSession, err)
	}

	index.Close()
	// Expire one sibling from the native source, retaining its indexed corpus.
	source, err = sql.Open("sqlite", capability)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`DELETE FROM part WHERE session_id='s1'; DELETE FROM message WHERE session_id='s1'; DELETE FROM session WHERE id='s1'`); err != nil {
		t.Fatal(err)
	}
	source.Close()
	for i := 0; i < 2; i++ {
		if err := json.Unmarshal([]byte(e.run("search", "--text", "nativequartz", "--all-projects", "--tag", "testing", "--fields", "minimal", "--json")), &found); err != nil || len(found) != 1 || found[0].SessionID != "s1" {
			t.Fatalf("expired native tags=%+v err=%v", found, err)
		}
		if err := json.Unmarshal([]byte(e.run("list", "--all-projects", "--json")), &listing); err != nil {
			t.Fatal(err)
		}
		for _, item := range listing.Sessions {
			if item.SessionID == "s1" {
				tagged := false
				for _, tag := range item.Tags {
					if tag == "testing" {
						tagged = true
					}
				}
				if !tagged {
					t.Fatalf("retained list tags=%+v", item)
				}
			}
		}
	}
	// Historical NULL identities may have a project internally, but must keep
	// the pre-native JSON contract when executed through the actual CLI.
	writable, err := storage.Open(e.database)
	if err != nil {
		t.Fatal(err)
	}
	var legacyUUID string
	if err := writable.DB().QueryRow("SELECT uuid FROM search_items WHERE session_id='s3'").Scan(&legacyUUID); err != nil {
		t.Fatal(err)
	}
	if _, err := writable.DB().Exec("UPDATE search_items SET session_id=NULL WHERE session_id='s3'"); err != nil {
		t.Fatal(err)
	}
	writable.Close()
	var historical struct{ Records []map[string]any }
	if err := json.Unmarshal([]byte(e.run("context", "--uuid", legacyUUID, "--json")), &historical); err != nil {
		t.Fatal(err)
	}
	if len(historical.Records) != 1 {
		t.Fatalf("historical context=%+v", historical)
	}
	if _, ok := historical.Records[0]["project"]; ok {
		t.Fatalf("historical JSON gained project: %+v", historical)
	}
	t.Log("copied fixture: native sessions/projects, registry precedence, context/sequence/tag isolation, stable perennial identities, absent-cwd unknown proof verified through actual CLI")
}

func TestOpenCodeFencedNormalizationCLI(t *testing.T) {
	e := newQueryEchoE2E(t)
	root := e.addReaderManifest("normalization", "opencode")
	path := filepath.Join(root, "opencode.db")
	source := openOpenCodeFixture(t, path)
	if _, err := source.Exec(`CREATE TABLE session(id TEXT PRIMARY KEY,directory TEXT); INSERT INTO session VALUES('native167','/projects/normalization')`); err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{
		"  quartz167 beacon\n```python\nvalue =  167\nprint(value)\n```\n  finish\tmarker  ",
		"  quartz167 beacon\nvalue =  167\nprint(value)\n  finish\tmarker  ",
		"quartz167 <system-reminder>native visible</system-reminder>\n  second    line",
	} {
		id := fmt.Sprintf("native167-%d", i)
		if _, err := source.Exec(`INSERT INTO message VALUES(?,?,?,?,?)`, id, "native167", i, i, `{"role":"user"}`); err != nil {
			t.Fatal(err)
		}
		part, _ := json.Marshal(map[string]any{"type": "text", "text": text})
		if _, err := source.Exec(`INSERT INTO part VALUES(?,?,?,?,?,?)`, id+"-part", id, "native167", i, i, string(part)); err != nil {
			t.Fatal(err)
		}
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	code := e.searchJSON("quartz167", "code", 20)
	if len(code) != 1 || code[0].Content != "quartz167 beacon ```python value = 167 print(value) ``` finish marker" {
		t.Fatalf("code-filtered normalized fenced fixture = %+v", code)
	}
	plain := e.searchJSON("quartz167", "text", 20)
	if len(plain) != 2 {
		t.Fatalf("text-filtered controls = %+v", plain)
	}
	index, err := storage.OpenReadOnly(e.database)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	var text, origin, session, evidence string
	if err := index.DB().QueryRow(`SELECT text,origin,session_id,project_evidence FROM search_items WHERE text LIKE '%system-reminder%'`).Scan(&text, &origin, &session, &evidence); err != nil {
		t.Fatal(err)
	}
	if text != "quartz167 <system-reminder>native visible</system-reminder> second line" || origin != "human" || session != "native167" || evidence != "native_cwd" {
		t.Fatalf("native envelope/ownership = %q/%q/%q/%q", text, origin, session, evidence)
	}
}
