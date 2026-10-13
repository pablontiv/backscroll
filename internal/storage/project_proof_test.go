package storage

import (
	"context"
	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/projects"
	"path/filepath"
	"testing"
)

func TestProjectRepairPreservesNativeCwdProof(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "proof.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := "/expired/.claude/projects/-home-shared-proj/session.jsonl"
	if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: path, Hash: "h", Project: "native-label", Messages: []IndexedMessage{{UUID: "native", Text: "unchanged", ContentType: "text"}}}}); err != nil {
		t.Fatal(err)
	}
	// Seed native proof independently of the repair path.
	if _, err := db.db.Exec("UPDATE search_items SET project_evidence = 'native_cwd'"); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := db.db.QueryRow("SELECT id FROM search_items").Scan(&before); err != nil {
		t.Fatal(err)
	}
	reg := projects.ProjectRegistry{Projects: []projects.ProjectConfig{{ID: "registry", Roots: []string{"/home/shared/proj"}}}}
	if _, err := db.ReresolveProjectsWithRegistry(context.Background(), reg); err != nil {
		t.Fatal(err)
	}
	if err := db.RebuildFTSContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	var id int64
	var label, uuid, text string
	if err := db.db.QueryRow("SELECT id,project,uuid,text FROM search_items").Scan(&id, &label, &uuid, &text); err != nil {
		t.Fatal(err)
	}
	if id != before || label != "native-label" || uuid != "native" || text != "unchanged" {
		t.Fatalf("native row changed: %d %s %s %s", id, label, uuid, text)
	}
}

func TestProjectRepairUsesAliasesAndGuardsEachRow(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "repair.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := "/expired/.claude/projects/-home-shared-proj/session.jsonl"
	for i, evidence := range []string{"unknown", "path_fallback", "native_cwd", "registry"} {
		if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: path, Hash: "h", Project: "initial", ProjectEvidence: evidence, Messages: []IndexedMessage{{Ordinal: i, UUID: evidence, Text: "stable " + evidence, ContentType: "text"}}}}); err != nil {
			t.Fatal(err)
		}
	}
	reg := projects.ProjectRegistry{Projects: []projects.ProjectConfig{{ID: "explicit", Roots: []string{"/Users/Shared/proj"}, Aliases: []string{"/home/shared/proj"}}}}
	if _, err := db.ReresolveProjectsWithRegistry(context.Background(), reg); err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []string{"unknown", "path_fallback", "native_cwd", "registry"} {
		var label, proof string
		if err := db.db.QueryRow("SELECT project,project_evidence FROM search_items WHERE uuid=?", evidence).Scan(&label, &proof); err != nil {
			t.Fatal(err)
		}
		want := "initial"
		if evidence == "unknown" || evidence == "path_fallback" {
			want = "explicit"
			if proof != "registry" {
				t.Fatal(proof)
			}
		}
		if label != want {
			t.Fatalf("%s label=%s want %s", evidence, label, want)
		}
	}
}

func TestPerennialNativeProjectCannotBeReplaced(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "replay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := IndexedFile{Source: "session", SourcePath: "/expired/session.jsonl", Hash: "h", Project: "original", ProjectEvidence: "native_cwd", Messages: []IndexedMessage{{UUID: "identity", Text: "original text", ContentType: "text"}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	file.Project = "replacement"
	file.ProjectEvidence = "registry"
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	var label string
	if err := db.db.QueryRow("SELECT project FROM search_items WHERE uuid='identity'").Scan(&label); err != nil {
		t.Fatal(err)
	}
	if label != "original" {
		t.Fatal(label)
	}
}

func TestRecoveryPreservesNativeProjectEvidence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := IndexedFile{Source: "session", SourcePath: "/expired/session.jsonl", Hash: "h", Project: "original", ProjectEvidence: "native_cwd", Messages: []IndexedMessage{{UUID: "11111111-1111-4111-8111-111111111111", Text: "original text", ContentType: "text"}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	input, diag, err := ReadRecoveryInput(context.Background(), db)
	if err != nil || diag != nil {
		t.Fatalf("%v %+v", err, diag)
	}
	plan, diags, err := compat.PlanRecovery([]compat.RecoveryInput{input})
	if err != nil || len(diags) != 0 {
		t.Fatalf("%v %+v", err, diags)
	}
	path, err := CreateRecoveryDestination(context.Background(), t.TempDir(), plan)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	var proof string
	if err := recovered.DB().QueryRow("SELECT project_evidence FROM search_items WHERE uuid='11111111-1111-4111-8111-111111111111'").Scan(&proof); err != nil {
		t.Fatal(err)
	}
	if proof != "native_cwd" {
		t.Fatal(proof)
	}
}
