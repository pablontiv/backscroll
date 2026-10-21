package storage

import (
	"context"
	"github.com/pablontiv/backscroll/internal/models"
	"testing"
)

func TestNativeSessionIsolationAndPerennialOwnership(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	file := IndexedFile{SourcePath: "/native/opencode.db", Source: "session", Hash: "one", Messages: []IndexedMessage{
		{Ordinal: 0, UUID: "a", SessionID: "s1", Project: "alpha", ProjectEvidence: "native_cwd", Text: "native alpha", Role: "user", ContentType: "text"},
		{Ordinal: 1, UUID: "b", SessionID: "s2", Project: "beta", ProjectEvidence: "native_cwd", Text: "native beta", Role: "user", ContentType: "text"},
		{Ordinal: 2, UUID: "ta", SessionID: "s1", Project: "alpha", ProjectEvidence: "native_cwd", Text: "bash command=go test", Role: "assistant", ContentType: "tool", ToolName: "Bash", CommandHead: "go"},
		{Ordinal: 3, UUID: "tb", SessionID: "s2", Project: "beta", ProjectEvidence: "native_cwd", Text: "bash command=git status", Role: "assistant", ContentType: "tool", ToolName: "Bash", CommandHead: "git"},
	}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	sessions, err := db.ListItemsV2(ListOptions{AllProjects: true})
	if err != nil || len(sessions) != 2 {
		t.Fatalf("sessions=%+v err=%v", sessions, err)
	}
	results, _, err := db.Search("native", models.SearchOptions{Project: "alpha"})
	if err != nil || len(results) != 1 || results[0].SessionID != "s1" {
		t.Fatalf("search=%+v err=%v", results, err)
	}
	uuid := "a"
	records, err := db.QueryContextRecords(context.Background(), ContextRecordQuery{UUID: &uuid, Before: 50, After: 50})
	if err != nil || len(records) != 2 {
		t.Fatalf("context=%+v err=%v", records, err)
	}
	for _, r := range records {
		if r.SessionID != "s1" {
			t.Fatal("context crossed sessions")
		}
	}
	if _, err := db.DB().Exec(`INSERT INTO session_tags VALUES('/native/opencode.db','testing')`); err != nil {
		t.Fatal(err)
	}
	tagged, _, err := db.Search("native", models.SearchOptions{Tag: "testing", Project: "beta"})
	if err != nil || len(tagged) != 0 {
		t.Fatalf("path tags widened native sessions: %+v err=%v", tagged, err)
	}
	positive, _, err := db.Search("native", models.SearchOptions{Tag: "testing", Project: "alpha"})
	if err != nil || len(positive) != 1 {
		t.Fatalf("scoped tags not applied: %+v %v", positive, err)
	}
	commands, err := db.AggregateCommands(AggregateOptions{Project: "beta", Tag: "testing"})
	if err != nil || len(commands) != 0 {
		t.Fatalf("command tag filter crossed sessions: %+v %v", commands, err)
	}
	seq, err := db.LoadToolSequences(LoadSequencesOpts{})
	if err != nil || len(seq) != 2 {
		t.Fatalf("sequences=%+v err=%v", seq, err)
	}
	seq, err = db.LoadToolSequences(LoadSequencesOpts{Project: "alpha"})
	if err != nil || len(seq) != 1 || seq[0].SessionID != "s1" {
		t.Fatalf("filtered sequences=%+v err=%v", seq, err)
	}
	if _, err := db.DB().Exec(`UPDATE search_items SET project='fallback',project_evidence='unknown' WHERE uuid='a'`); err != nil {
		t.Fatal(err)
	}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	var proof, project string
	if err := db.DB().QueryRow(`SELECT project,project_evidence FROM search_items WHERE uuid='a'`).Scan(&project, &proof); err != nil || project != "alpha" || proof != "native_cwd" {
		t.Fatalf("native project enrichment=%s/%s %v", project, proof, err)
	}
	file.Hash = "two"
	file.Messages[0].Text = "mutated"
	file.Messages[0].Ordinal = 3
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	var text string
	var ordinal int
	if err := db.DB().QueryRow(`SELECT text,ordinal FROM search_items WHERE uuid='a'`).Scan(&text, &ordinal); err != nil || text != "native alpha" || ordinal != 0 {
		t.Fatalf("retained text=%s ordinal=%d err=%v", text, ordinal, err)
	}
	file.Messages[0].SessionID = "s2"
	if err := db.SyncFiles([]IndexedFile{file}); err == nil {
		t.Fatal("session reassignment accepted")
	}
	file.Messages[0].SessionID = "s1"
	file.Messages[0].Project = "beta"
	if err := db.SyncFiles([]IndexedFile{file}); err == nil {
		t.Fatal("native project reassignment accepted")
	}
	file.Messages[0].Project = "alpha"
	file.SourcePath = "/other.db"
	if err := db.SyncFiles([]IndexedFile{file}); err == nil {
		t.Fatal("global UUID collision accepted")
	}
}

func TestNativeSessionTagsSurviveSiblingReplay(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	file := IndexedFile{SourcePath: "/native.db", Source: "session", Hash: "one", Tags: []string{"docs"}, Messages: []IndexedMessage{
		{UUID: "s1-message", SessionID: "s1", Ordinal: 0, Text: "retainedquartz go test", Role: "user", ContentType: "text"},
		{UUID: "s2-message", SessionID: "s2", Ordinal: 1, Text: "siblingquartz ordinary", Role: "user", ContentType: "text"},
	}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	// A stale identity tag must be corrected from canonical retained payload.
	if _, err := db.DB().Exec("INSERT INTO session_identity_tags VALUES('/native.db','s2','testing')"); err != nil {
		t.Fatal(err)
	}
	file.Hash = "two"
	file.Messages = file.Messages[1:]
	for i := 0; i < 2; i++ {
		if err := db.SyncFiles([]IndexedFile{file}); err != nil {
			t.Fatal(err)
		}
		hits, _, err := db.Search("retainedquartz", models.SearchOptions{Tag: "testing"})
		if err != nil || len(hits) != 1 || hits[0].SessionID != "s1" {
			t.Fatalf("retained tagged hits=%+v err=%v", hits, err)
		}
		items, err := db.ListItemsV2(ListOptions{AllProjects: true})
		if err != nil || len(items) != 2 {
			t.Fatalf("tagged list=%+v err=%v", items, err)
		}
		for _, item := range items {
			hasTesting := false
			for _, tag := range item.Tags {
				if tag == "testing" {
					hasTesting = true
				}
			}
			if hasTesting != (item.SessionID == "s1") {
				t.Fatalf("list identity tags=%+v", item)
			}
		}
		hits, _, err = db.Search("siblingquartz", models.SearchOptions{Tag: "testing"})
		if err != nil || len(hits) != 0 {
			t.Fatalf("stale sibling tags=%+v err=%v", hits, err)
		}
		var legacy int
		if err := db.DB().QueryRow("SELECT COUNT(*) FROM session_tags WHERE source_path='/native.db' AND tag='docs'").Scan(&legacy); err != nil || legacy != 1 {
			t.Fatalf("legacy tags=%d err=%v", legacy, err)
		}
	}

	file.Hash = "three"
	file.IngestionFingerprint = "three-fingerprint"
	file.Messages = append(file.Messages, IndexedMessage{UUID: "s2-docs", SessionID: "s2", Ordinal: 2, Text: "sibling documentation", Role: "user", ContentType: "text"})
	for i := 0; i < 2; i++ {
		if err := db.SyncFiles([]IndexedFile{file}); err != nil {
			t.Fatal(err)
		}
		hits, _, err := db.Search("siblingquartz", models.SearchOptions{Tag: "docs"})
		if err != nil || len(hits) != 1 || hits[0].SessionID != "s2" {
			t.Fatalf("updated sibling tags=%+v err=%v", hits, err)
		}
	}
	// A late insertion failure must roll back corpus, hash and both tag scopes.
	if _, err := db.DB().Exec(`CREATE TRIGGER fail_identity_tags BEFORE INSERT ON session_identity_tags BEGIN SELECT RAISE(ABORT,'tag failure'); END`); err != nil {
		t.Fatal(err)
	}
	file.Hash = "failed"
	file.IngestionFingerprint = "failed-fingerprint"
	file.Messages = append(file.Messages, IndexedMessage{UUID: "new-message", SessionID: "s2", Ordinal: 2, Text: "new test", Role: "user", ContentType: "text"})
	file.Tags = []string{"config"}
	if err := db.SyncFiles([]IndexedFile{file}); err == nil {
		t.Fatal("tag failure accepted")
	}
	var count int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM search_items WHERE uuid='new-message'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback rows=%d err=%v", count, err)
	}

	var hash, fingerprint string
	if err := db.DB().QueryRow("SELECT hash,ingestion_fingerprint FROM indexed_files WHERE path='/native.db'").Scan(&hash, &fingerprint); err != nil || hash != "three" || fingerprint != "three-fingerprint" {
		t.Fatalf("rollback hash=%q fingerprint=%q err=%v", hash, fingerprint, err)
	}
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM session_tags WHERE source_path='/native.db' AND tag='docs'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback legacy tags=%d err=%v", count, err)
	}
	hits, _, err := db.Search("retainedquartz", models.SearchOptions{Tag: "testing"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("rollback tags=%+v err=%v", hits, err)
	}
}

func TestNativeSessionCorrectionsRetainConversationBoundaries(t *testing.T) {
	for _, mode := range []string{"sync", "backfill"} {
		t.Run(mode, func(t *testing.T) {
			db, cleanup := newTestDB(t)
			defer cleanup()
			file := IndexedFile{SourcePath: "/expired-native.db", Source: "session", Hash: "native", Messages: []IndexedMessage{
				{Ordinal: 0, UUID: "a-denial", SessionID: "a", Role: "assistant", Text: "permission denied", ContentType: "text"},
				{Ordinal: 1, UUID: "b-request", SessionID: "b", Role: "user", Text: "please continue", ContentType: "text"},
				{Ordinal: 2, UUID: "a-request", SessionID: "a", Role: "user", Text: "please continue", ContentType: "text"},
			}}
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			if mode == "backfill" {
				if _, err := db.DB().Exec("DELETE FROM correction_signals; DELETE FROM indexed_files"); err != nil {
					t.Fatal(err)
				}
				if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
					t.Fatal(err)
				}
			}
			rows, err := db.DB().Query("SELECT item_uuid FROM correction_signals WHERE detector='denial'")
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var identities []string
			for rows.Next() {
				var identity string
				if err := rows.Scan(&identity); err != nil {
					t.Fatal(err)
				}
				identities = append(identities, identity)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if len(identities) != 1 || identities[0] != "a-request" {
				t.Fatalf("logical conversation denial identities: %+v", identities)
			}
		})
	}
}

func TestNativeSessionRetainedUnknownProofEnrichesFromSiblingRecord(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	file := IndexedFile{SourcePath: "/native.db", Source: "session", Hash: "one", Messages: []IndexedMessage{{UUID: "old", SessionID: "native", Role: "user", Text: "retained", ContentType: "text", ProjectEvidence: "unknown"}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	file.Hash = "two"
	file.Messages = []IndexedMessage{{UUID: "new", SessionID: "native", Role: "user", Text: "appended", ContentType: "text", Project: "alpha", ProjectEvidence: "native_cwd"}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	items, err := db.ListItemsV2(ListOptions{AllProjects: true})
	if err != nil || len(items) != 1 || items[0].Project != "alpha" {
		t.Fatalf("logical retained project group: %+v %v", items, err)
	}
	var proof, text string
	if err := db.DB().QueryRow("SELECT project_evidence,text FROM search_items WHERE uuid='old'").Scan(&proof, &text); err != nil || proof != "native_cwd" || text != "retained" {
		t.Fatalf("retained proof=%s text=%s err=%v", proof, text, err)
	}
}
