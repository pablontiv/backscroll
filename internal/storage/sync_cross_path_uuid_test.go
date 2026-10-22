package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestSyncCrossPathUUIDDeterministicCanonicalOwnership(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			db := crossPathUUIDDatabase(t)
			failed, exit := true, 7
			shared := []IndexedMessage{
				{UUID: "opaque-shared", Ordinal: 3, Role: "user", Text: "That is not what I asked; use the synthetic implementation", ContentType: "text", Origin: models.OriginHuman},
				{UUID: "opaque-shared#t1", Ordinal: 4, Role: "assistant", Text: "Bash command=go test", ContentType: "tool", ToolName: "Bash", CommandHead: "go", ToolUseID: "native-call", IsError: &failed, ExitCode: &exit},
				{UUID: "opaque-result#r0", Ordinal: 6, Role: "user", Text: "error: synthetic connection refused", ContentType: "tool", IsError: &failed},
			}
			owner := IndexedFile{Source: "session", SourcePath: "/a-owner", Hash: "owner-hash", Project: "fixture", Messages: append([]IndexedMessage(nil), shared...)}
			owner.Messages = append(owner.Messages, IndexedMessage{UUID: "owner-tail", Ordinal: 4, Role: "assistant", Text: "owner unique tail", ContentType: "text"})
			fork := IndexedFile{Source: "session", SourcePath: "/z-fork", Hash: "fork-hash", Project: "fixture", Messages: append([]IndexedMessage(nil), shared...)}
			fork.Messages = append(fork.Messages, IndexedMessage{UUID: "fork-tail", Ordinal: 4, Role: "assistant", Text: "fork unique tail", ContentType: "text"})
			files := []IndexedFile{owner, fork}
			if reverse {
				files[0], files[1] = files[1], files[0]
			}
			before, _ := json.Marshal(files)
			if err := db.SyncFiles(files); err != nil {
				t.Fatal(err)
			}
			for uuid, want := range map[string]struct {
				path    string
				ordinal int
			}{
				"opaque-shared": {"/a-owner", 3}, "opaque-shared#t1": {"/a-owner", 4},
				"opaque-result#r0": {"/a-owner", 6}, "owner-tail": {"/a-owner", 0}, "fork-tail": {"/z-fork", 4},
			} {
				var path string
				var ordinal int
				if err := db.db.QueryRow(`SELECT source_path,ordinal FROM search_items WHERE uuid=?`, uuid).Scan(&path, &ordinal); err != nil || path != want.path || ordinal != want.ordinal {
					t.Fatalf("%s coordinate=(%s,%d), err=%v, want=%+v", uuid, path, ordinal, err, want)
				}
				rows, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: &uuid, Before: 10, After: 10})
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range rows {
					if row.SourcePath != want.path {
						t.Fatalf("UUID context crossed canonical owner: %+v", rows)
					}
				}
			}
			crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items`, 5)
			crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM tool_events WHERE source_path='/a-owner' AND ordinal=4`, 1)
			crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM template_matches WHERE source_path='/a-owner' AND ordinal=6`, 1)
			crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM correction_signals WHERE source_path='/a-owner' AND ordinal=3`, 1)
			commands, err := db.AggregateCommands(AggregateOptions{})
			if err != nil || len(commands) != 1 || commands[0].Count != 1 {
				t.Fatalf("canonical command census=%+v err=%v", commands, err)
			}
			stable := crossPathUUIDSnapshot(t, db)
			if err := db.SyncFiles(files); err != nil {
				t.Fatal(err)
			}
			// last_indexed is observation time, not an event anchor.
			if got := crossPathUUIDSnapshot(t, db); !reflect.DeepEqual(got, stable) {
				t.Fatalf("replay changed canonical rows or satellites: before=%v after=%v", stable, got)
			}
			after, _ := json.Marshal(files)
			if string(before) != string(after) {
				t.Fatal("canonical reconciliation mutated caller messages")
			}
		})
	}
}

func TestSyncCrossPathUUIDRetainedOwnerEnrichmentAbsentOrExpired(t *testing.T) {
	for _, expired := range []bool{false, true} {
		for _, failure := range []bool{false, true} {
			t.Run(fmt.Sprintf("expired=%v/failure=%v", expired, failure), func(t *testing.T) {
				db := crossPathUUIDDatabase(t)
				path := filepath.Join(t.TempDir(), "original.jsonl")
				if err := os.WriteFile(path, []byte("private synthetic owner"), 0o600); err != nil {
					t.Fatal(err)
				}
				size, mtime := int64(23), "2026-01-01T00:00:00Z"
				owner := IndexedFile{Source: "session", SourcePath: path, Hash: "original-hash", IngestionFingerprint: "original-fingerprint", Project: "fallback", ProjectEvidence: "path_fallback", FileSize: &size, FileMtime: &mtime, Tags: []string{"original-tag"}, Messages: []IndexedMessage{
					{UUID: "retained-call", Ordinal: 9, Role: "assistant", Text: "Bash command=go test", ContentType: "tool", ToolName: "Bash", CommandHead: "go", ExtractionVersion: 1},
					{UUID: "retained-human", Ordinal: 10, Role: "user", Text: "Please correct this synthetic implementation now", ContentType: "text", ExtractionVersion: 1},
					{UUID: "unrelated-open", Ordinal: 11, Role: "assistant", Text: "Bash command=backscroll search --text unrelated", ContentType: "tool", ExtractionVersion: 1},
					{UUID: "retained-result#r0", Ordinal: 12, Role: "user", Text: "error: synthetic connection refused", ContentType: "tool", ExtractionVersion: 1},
				}}
				if err := db.SyncFiles([]IndexedFile{owner}); err != nil {
					t.Fatal(err)
				}
				if _, err := db.db.Exec(`UPDATE search_items SET origin_version=NULL,search_echo=NULL,project_evidence=NULL WHERE uuid='unrelated-open'; UPDATE indexed_files SET last_indexed='2001-01-01'`); err != nil {
					t.Fatal(err)
				}
				var itemID, eventID int64
				if err := db.db.QueryRow(`SELECT s.id,e.id FROM search_items s JOIN tool_events e ON e.message_uuid=s.uuid WHERE s.uuid='retained-call'`).Scan(&itemID, &eventID); err != nil {
					t.Fatal(err)
				}
				if expired {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				exit := 0
				if failure {
					exit = 7
				}
				copySize, copyMtime := int64(99), "2026-02-01T00:00:00Z"
				copy := IndexedFile{Source: "session", SourcePath: "/a-copy", Hash: "copy-hash", IngestionFingerprint: "copy-fingerprint", FileSize: &copySize, FileMtime: &copyMtime, Project: "irrelevant-fallback", ProjectEvidence: "path_fallback", Messages: []IndexedMessage{
					{UUID: "retained-call", Ordinal: 0, Role: "assistant", Text: "Bash command=go test", ContentType: "tool", ToolName: "Bash", CommandHead: "go", ToolUseID: "proved-call", IsError: &failure, ExitCode: &exit, Origin: models.OriginAssistant, SessionID: "proved-session", Project: "proved-project", ProjectEvidence: "native_cwd", SearchEcho: true, ExtractionVersion: 99},
					{UUID: "retained-human", Ordinal: 1, Role: "user", Text: "Please correct this synthetic implementation now", ContentType: "text", Origin: models.OriginHuman, WasInterrupted: true, SessionID: "proved-session", Project: "proved-project", ProjectEvidence: "native_cwd", ExtractionVersion: 99},
					{UUID: "retained-result#r0", Ordinal: 2, Role: "user", Text: "error: synthetic connection refused", ContentType: "tool", IsError: new(true), Origin: models.OriginAutomation},
				}}
				before, _ := json.Marshal(copy)
				for range 2 {
					if err := db.SyncFiles([]IndexedFile{copy}); err != nil {
						t.Fatal(err)
					}
				}
				var gotItem, gotEvent int64
				var ownerPath, uuid, text, session, project, evidence, origin, native string
				var ordinal, epoch, exitCode int
				var isError, echo bool
				if err := db.db.QueryRow(`SELECT s.id,e.id,s.source_path,s.uuid,s.ordinal,s.text,s.extraction_version,s.session_id,s.project,s.project_evidence,s.origin,s.search_echo,e.native_call_id,e.is_error,e.exit_code FROM search_items s JOIN tool_events e ON e.message_uuid=s.uuid WHERE s.uuid='retained-call'`).Scan(&gotItem, &gotEvent, &ownerPath, &uuid, &ordinal, &text, &epoch, &session, &project, &evidence, &origin, &echo, &native, &isError, &exitCode); err != nil {
					t.Fatal(err)
				}
				if gotItem != itemID || gotEvent != eventID || ownerPath != path || uuid != "retained-call" || ordinal != 9 || text != owner.Messages[0].Text || epoch != 1 || session != "proved-session" || project != "proved-project" || evidence != "native_cwd" || origin != "assistant" || !echo || native != "proved-call" || isError != failure || exitCode != exit {
					t.Fatalf("retained enrichment=(%d,%d,%s,%s,%d,%s,%d,%s,%s,%s,%s,%v,%s,%v,%d)", gotItem, gotEvent, ownerPath, uuid, ordinal, text, epoch, session, project, evidence, origin, echo, native, isError, exitCode)
				}
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items WHERE uuid='retained-human' AND was_interrupted=1 AND origin='human' AND ordinal=10`, 1)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items WHERE uuid='unrelated-open' AND origin_version IS NULL AND search_echo IS NULL AND project_evidence IS NULL`, 1)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items WHERE source_path='/a-copy'`, 0)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM tool_events`, 1)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM correction_signals WHERE source_path='/a-copy'`, 0)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM correction_signals WHERE item_uuid='retained-human' AND detector='interrupt'`, 1)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM template_matches WHERE item_uuid='retained-result#r0' AND ordinal=12`, 1)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM template_matches WHERE source_path='/a-copy'`, 0)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM indexed_files WHERE hash='original-hash' AND ingestion_fingerprint='original-fingerprint' AND file_size=23 AND file_mtime='2026-01-01T00:00:00Z' AND last_indexed='2001-01-01'`, 1)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM indexed_files WHERE path='/a-copy' AND hash='copy-hash' AND ingestion_fingerprint='copy-fingerprint' AND file_size=99 AND file_mtime='2026-02-01T00:00:00Z'`, 1)
				after, _ := json.Marshal(copy)
				if string(before) != string(after) {
					t.Fatal("copy enrichment mutated caller messages")
				}
			})
		}
	}
}

func TestSyncCrossPathUUIDContradictionsAreAtomic(t *testing.T) {
	cases := []struct {
		name   string
		change func(*IndexedFile)
	}{
		{"text", func(f *IndexedFile) { f.Messages[0].Text = "contradiction" }},
		{"role", func(f *IndexedFile) { f.Messages[0].Role = "user" }},
		{"content-type", func(f *IndexedFile) { f.Messages[0].ContentType = "text" }},
		{"timestamp", func(f *IndexedFile) { f.Messages[0].Timestamp = "2026-01-02T00:00:00Z" }},
		{"source", func(f *IndexedFile) { f.Source = "plan" }},
		{"origin", func(f *IndexedFile) { f.Messages[0].Origin = models.OriginHuman }},
		{"session", func(f *IndexedFile) { f.Messages[0].SessionID = "contradiction" }},
		{"project", func(f *IndexedFile) { f.Messages[0].Project = "contradiction" }},
		{"tool", func(f *IndexedFile) { f.Messages[0].ToolName = "Read" }},
		{"command", func(f *IndexedFile) { f.Messages[0].CommandHead = "make" }},
		{"native-call", func(f *IndexedFile) { f.Messages[0].ToolUseID = "contradiction" }},
		{"known-false", func(f *IndexedFile) { value := true; f.Messages[0].IsError = &value }},
		{"known-zero", func(f *IndexedFile) { value := 7; f.Messages[0].ExitCode = &value }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := crossPathUUIDDatabase(t)
			ok, zero := false, 0
			owner := IndexedFile{Source: "session", SourcePath: "/owner", Hash: "original", IngestionFingerprint: "original-proof", Tags: []string{"original-tag"}, Messages: []IndexedMessage{{UUID: "proved-opaque", Ordinal: 7, Role: "assistant", Origin: models.OriginAssistant, Text: "Bash command=go test", Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool", ToolName: "Bash", CommandHead: "go", ToolUseID: "native-call", IsError: &ok, ExitCode: &zero, SessionID: "native-session", Project: "native-project", ProjectEvidence: "native_cwd"}}}
			prior := IndexedFile{Source: "session", SourcePath: "/prior", Hash: "prior-hash", Tags: []string{"prior-tag"}, Messages: []IndexedMessage{{UUID: "prior-row", Role: "user", Text: "That is not what I asked; correct the synthetic implementation", ContentType: "text"}}}
			if err := db.SyncFiles([]IndexedFile{owner, prior}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(`UPDATE search_items SET origin_version=NULL,search_echo=NULL WHERE uuid='prior-row'; UPDATE indexed_files SET last_indexed='2001-01-01'; INSERT INTO annotations(source_path,ordinal,kind,label,created_at) VALUES('/prior',0,'correction','retained','2001-01-01')`); err != nil {
				t.Fatal(err)
			}
			before := crossPathUUIDSnapshot(t, db)
			copy := owner
			copy.SourcePath, copy.Hash = "/foreign", "rejected"
			copy.Messages = append([]IndexedMessage(nil), owner.Messages...)
			tc.change(&copy)
			prior.Hash, prior.Messages = "changed-prior", nil
			if err := db.SyncFiles([]IndexedFile{prior, copy}); err == nil {
				t.Fatal("contradictory copy admitted")
			}
			if after := crossPathUUIDSnapshot(t, db); !reflect.DeepEqual(before, after) {
				t.Fatalf("failed copy mutated retained rows or file progress: before=%v after=%v", before, after)
			}
			crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM indexed_files WHERE last_indexed='2001-01-01'`, 2)
		})
	}
}

func TestSyncCrossPathUUIDIncomingProofContradictionIsAtomic(t *testing.T) {
	db := crossPathUUIDDatabase(t)
	unknown := IndexedMessage{UUID: "shared", Role: "user", Text: "same payload", ContentType: "text"}
	human, assistant := unknown, unknown
	human.Origin, assistant.Origin = models.OriginHuman, models.OriginAssistant
	files := []IndexedFile{
		{Source: "session", SourcePath: "/a", Hash: "a", Messages: []IndexedMessage{unknown}},
		{Source: "session", SourcePath: "/b", Hash: "b", Messages: []IndexedMessage{human}},
		{Source: "session", SourcePath: "/c", Hash: "c", Messages: []IndexedMessage{assistant}},
	}
	if err := db.SyncFiles(files); err == nil {
		t.Fatal("conflicting incoming proven origins admitted through unknown owner")
	}
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items`, 0)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM indexed_files`, 0)
}

func TestSyncCrossPathUUIDIncomingCompatibleEnrichment(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			db := crossPathUUIDDatabase(t)
			unknown := IndexedMessage{UUID: "new-call", Ordinal: 7, Role: "assistant", Text: "Bash command=go test", ContentType: "tool"}
			proven := unknown
			proven.Ordinal, proven.Origin = 0, models.OriginAssistant
			proven.SessionID, proven.Project, proven.ProjectEvidence = "native-session", "native-project", "native_cwd"
			proven.ToolName, proven.CommandHead, proven.ToolUseID = "Bash", "go", "native-call"
			proven.IsError, proven.ExitCode = new(false), new(int)
			proven.SearchEcho, proven.WasInterrupted = true, true
			files := []IndexedFile{
				{Source: "session", SourcePath: "/a-owner", Hash: "a", Project: "fallback", ProjectEvidence: "path_fallback", Messages: []IndexedMessage{unknown}},
				{Source: "session", SourcePath: "/z-copy", Hash: "z", Messages: []IndexedMessage{proven}},
			}
			if reverse {
				files[0], files[1] = files[1], files[0]
			}
			if err := db.SyncFiles(files); err != nil {
				t.Fatal(err)
			}
			crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items WHERE uuid='new-call' AND source_path='/a-owner' AND ordinal=7 AND origin='assistant' AND session_id='native-session' AND project='native-project' AND project_evidence='native_cwd' AND search_echo=1 AND was_interrupted=1`, 1)
			crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM tool_events WHERE message_uuid='new-call' AND source_path='/a-owner' AND ordinal=7 AND tool_name='Bash' AND command_head='go' AND native_call_id='native-call' AND is_error=0 AND exit_code=0`, 1)
			crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM indexed_files`, 2)
		})
	}
}

func TestSyncCrossPathUUIDOrdinalsAndSatellites(t *testing.T) {
	db := crossPathUUIDDatabase(t)
	owner := IndexedFile{Source: "session", SourcePath: "/owner", Hash: "owner", Messages: []IndexedMessage{{UUID: "shared-call", Ordinal: 9, Role: "assistant", Text: "Bash command=go test", ContentType: "tool", ToolName: "Bash", CommandHead: "go"}}}
	fork := IndexedFile{Source: "session", SourcePath: "/fork", Hash: "fork", Messages: []IndexedMessage{{UUID: "fork-anchor", Ordinal: 3, Text: "retained fork anchor", ContentType: "text"}}}
	if err := db.SyncFiles([]IndexedFile{owner, fork}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,content_type) VALUES('session','/fork',0,'user','retained UUID-less row','text'); INSERT INTO tool_events(source_path,ordinal,tool_name,extraction_version) VALUES('/fork',1,'LegacyTool',0)`); err != nil {
		t.Fatal(err)
	}
	fork.Hash = "updated-fork"
	fork.Messages = []IndexedMessage{
		{UUID: "shared-call", Ordinal: 0, Role: "assistant", Text: "Bash command=go test", ContentType: "tool", ToolName: "Bash", CommandHead: "go"},
		{UUID: "new-tail", Ordinal: 0, Role: "assistant", Text: "Bash command=make", ContentType: "tool", ToolName: "Bash", CommandHead: "make"},
		{UUID: "fork-anchor", Ordinal: 0, Text: "retained fork anchor", ContentType: "text"},
		{Ordinal: 4, Text: "partial parser snapshot", ContentType: "text"},
	}
	for range 2 {
		if err := db.SyncFiles([]IndexedFile{fork}); err != nil {
			t.Fatal(err)
		}
	}
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items WHERE source_path='/fork' AND uuid IS NULL AND ordinal=0`, 1)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM tool_events WHERE source_path='/fork' AND message_uuid IS NULL AND ordinal=1`, 1)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items s JOIN tool_events e ON e.message_uuid=s.uuid AND e.source_path=s.source_path AND e.ordinal=s.ordinal WHERE s.uuid='new-tail' AND s.source_path='/fork' AND s.ordinal=2`, 1)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items WHERE uuid='fork-anchor' AND ordinal=3`, 1)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM tool_events WHERE message_uuid='shared-call' AND source_path='/owner' AND ordinal=9`, 1)
	if fork.Messages[0].Ordinal != 0 || fork.Messages[1].Ordinal != 0 || fork.Messages[2].Ordinal != 0 {
		t.Fatal("reconciliation mutated caller ordinals")
	}
}

func TestSyncCrossPathUUIDRetainedCoordinateContradictionIsAtomic(t *testing.T) {
	for _, corruption := range []string{
		`UPDATE search_items SET ordinal=-1 WHERE uuid='retained'`,
		`UPDATE tool_events SET ordinal=8 WHERE message_uuid='retained'`,
	} {
		t.Run(corruption, func(t *testing.T) {
			db := crossPathUUIDDatabase(t)
			file := IndexedFile{Source: "session", SourcePath: "/owner", Hash: "retained", Messages: []IndexedMessage{{UUID: "retained", Ordinal: 7, Role: "assistant", Text: "Bash command=go", ContentType: "tool", ToolName: "Bash"}}}
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(corruption); err != nil {
				t.Fatal(err)
			}
			before := crossPathUUIDSnapshot(t, db)
			file.SourcePath, file.Hash = "/copy", "rejected"
			if err := db.SyncFiles([]IndexedFile{file}); err == nil {
				t.Fatal("contradictory retained canonical coordinate admitted")
			}
			if after := crossPathUUIDSnapshot(t, db); !reflect.DeepEqual(before, after) {
				t.Fatalf("coordinate rejection mutated retained state: before=%v after=%v", before, after)
			}
		})
	}
}

func TestSyncCrossPathUUIDAbsentOwnerBatchDerivesOnce(t *testing.T) {
	db := crossPathUUIDDatabase(t)
	const n = 100
	messages := make([]IndexedMessage, n)
	for i := range messages {
		messages[i] = IndexedMessage{UUID: fmt.Sprintf("shared-%d", i), Ordinal: i, Role: "user", Text: "error: synthetic connection refused", ContentType: "tool", SessionID: "native"}
	}
	owner := IndexedFile{Source: "session", SourcePath: "/owner", Hash: "retained", Messages: messages}
	if err := db.SyncFiles([]IndexedFile{owner}); err != nil {
		t.Fatal(err)
	}
	// Observe derived writes, including ignored template insertion attempts.
	// A repeated path scan/rebuild writes tags and attempts this template once
	// per event; a path batch writes each derived identity/template only once.
	if _, err := db.db.Exec(`
		CREATE TABLE derived_writes(kind TEXT);
		CREATE TRIGGER observe_identity_tags BEFORE INSERT ON session_identity_tags
		WHEN NEW.source_path='/owner' BEGIN INSERT INTO derived_writes VALUES('tag'); END;
		CREATE TRIGGER observe_templates BEFORE INSERT ON message_templates
		BEGIN INSERT INTO derived_writes VALUES('template'); END;
		UPDATE indexed_files SET last_indexed='2001-01-01'`); err != nil {
		t.Fatal(err)
	}
	copy := owner
	copy.SourcePath, copy.Hash = "/copy", "copy"
	copy.Messages = append([]IndexedMessage(nil), messages...)
	for i := range copy.Messages {
		copy.Messages[i].IsError = new(true)
		copy.Messages[i].WasInterrupted = true
	}
	copy.Messages[n-1].Project, copy.Messages[n-1].ProjectEvidence = "proved", "native_cwd"
	if err := db.SyncFiles([]IndexedFile{copy}); err != nil {
		t.Fatal(err)
	}
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items WHERE source_path='/owner' AND project='proved' AND project_evidence='native_cwd'`, n)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM search_items`, n)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM template_matches WHERE source_path='/owner'`, n)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM correction_signals WHERE source_path='/owner' AND detector='interrupt'`, n)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM derived_writes WHERE kind='template'`, 1)
	var tags int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM session_identity_tags WHERE source_path='/owner'`).Scan(&tags); err != nil || tags == 0 {
		t.Fatalf("native tags=%d err=%v", tags, err)
	}
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM derived_writes WHERE kind='tag'`, tags)
	crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM indexed_files WHERE path='/owner' AND hash='retained' AND last_indexed='2001-01-01'`, 1)
}

func TestSyncCrossPathUUIDFileTagsRespectNativeSessions(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			db := crossPathUUIDDatabase(t)
			shared := IndexedMessage{UUID: "shared", SessionID: "native", Role: "user", Text: "go test failing tests", ContentType: "text"}
			owner := IndexedFile{Source: "session", SourcePath: "/a", Hash: "a", Messages: []IndexedMessage{shared}}
			if mixed {
				owner.Messages = append(owner.Messages, IndexedMessage{UUID: "legacy", Ordinal: 1, Role: "user", Text: "update documentation README", ContentType: "text"})
			}
			if err := db.SyncFiles([]IndexedFile{owner, {Source: "session", SourcePath: "/b", Hash: "b", Messages: []IndexedMessage{shared}}}); err != nil {
				t.Fatal(err)
			}
			if mixed {
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM session_tags WHERE source_path='/a' AND tag='docs'`, 1)
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM session_tags WHERE source_path='/a'`, len(canonicalSyncTags(owner.Messages)))
			} else {
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM session_tags WHERE source_path='/a'`, 0)
			}
			crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM session_identity_tags WHERE source_path='/a' AND tag='testing'`, 1)
		})
	}
}

func TestSyncCrossPathUUIDLossySatelliteRequiresToolProof(t *testing.T) {
	for _, tool := range []bool{false, true} {
		t.Run(fmt.Sprint(tool), func(t *testing.T) {
			db := crossPathUUIDDatabase(t)
			message := IndexedMessage{UUID: "shared", Ordinal: 5, Role: "user", Text: "plain event", ContentType: "text"}
			owner := IndexedFile{Source: "session", SourcePath: "/owner", Hash: "owner", Messages: []IndexedMessage{message}}
			if err := db.SyncFiles([]IndexedFile{owner}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(`INSERT INTO tool_events(source_path,ordinal,tool_name,command_head,extraction_version) VALUES('/owner',5,'Bash','make',0)`); err != nil {
				t.Fatal(err)
			}
			copy := owner
			copy.SourcePath, copy.Hash = "/copy", "copy"
			copy.Messages = append([]IndexedMessage(nil), owner.Messages...)
			if tool {
				copy.Messages[0].ToolName, copy.Messages[0].CommandHead = "Bash", "make"
			}
			if err := db.SyncFiles([]IndexedFile{copy}); err != nil {
				t.Fatal(err)
			}
			if tool {
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM tool_events WHERE source_path='/owner' AND ordinal=5 AND message_uuid='shared'`, 1)
			} else {
				crossPathUUIDCount(t, db, `SELECT COUNT(*) FROM tool_events WHERE source_path='/owner' AND ordinal=5 AND message_uuid IS NULL`, 1)
			}
		})
	}
}

func crossPathUUIDDatabase(t *testing.T) *Database {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "canonical.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func crossPathUUIDCount(t *testing.T, db *Database, query string, want int) {
	t.Helper()
	var got int
	if err := db.db.QueryRow(query).Scan(&got); err != nil || got != want {
		t.Fatalf("%s = %d, err=%v, want %d", query, got, err, want)
	}
}

func crossPathUUIDSnapshot(t *testing.T, db *Database) map[string]string {
	t.Helper()
	result := make(map[string]string)
	for _, table := range []string{"search_items", "tool_events", "template_matches", "correction_signals", "indexed_files", "session_tags", "session_identity_tags", "annotations"} {
		query := "SELECT * FROM " + table + " ORDER BY 1,2"
		if table == "indexed_files" {
			query = "SELECT path,hash,file_size,file_mtime,ingestion_fingerprint FROM indexed_files ORDER BY path"
		}
		rows, err := db.db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		var records [][]any
		for rows.Next() {
			values, dest := make([]any, len(columns)), make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			records = append(records, values)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(records)
		if err != nil {
			t.Fatal(err)
		}
		result[table] = string(data)
	}
	return result
}
