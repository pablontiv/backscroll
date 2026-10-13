package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/projects"
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

func TestProjectRepairsLeavePendingNativeRowsUntouched(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pending.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := "/expired/.claude/projects/-home-shared-proj/session.jsonl"
	file := IndexedFile{Source: "session", SourcePath: path, Hash: "h", Project: "unknown", ProjectEvidence: "unknown", Messages: []IndexedMessage{{UUID: "pending", Text: "pending", ContentType: "text"}, {Ordinal: 1, UUID: "completed", Text: "completed", ContentType: "text"}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("UPDATE search_items SET project_evidence=NULL WHERE uuid='pending'"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReresolveProjectsContext(context.Background(), func(string) string { return "path-label" }); err != nil {
		t.Fatal(err)
	}
	reg := projects.ProjectRegistry{Projects: []projects.ProjectConfig{{ID: "registry", Roots: []string{"/home/shared/proj"}}}}
	if _, err := db.ReresolveProjectsWithRegistryContext(context.Background(), reg); err != nil {
		t.Fatal(err)
	}
	var label string
	var proof sql.NullString
	if err := db.db.QueryRow("SELECT project,project_evidence FROM search_items WHERE uuid='pending'").Scan(&label, &proof); err != nil {
		t.Fatal(err)
	}
	if label != "unknown" || proof.Valid {
		t.Fatalf("pending native row consumed: %s %+v", label, proof)
	}
	if err := db.db.QueryRow("SELECT project,project_evidence FROM search_items WHERE uuid='completed'").Scan(&label, &proof); err != nil {
		t.Fatal(err)
	}
	if label != "registry" || proof.String != "registry" {
		t.Fatalf("completed repair missing: %s %+v", label, proof)
	}
}

func TestCompletedProjectParseClosesRetainedPendingEvidence(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint(empty), func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "completed.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			file := IndexedFile{Source: "session", SourcePath: "/session.jsonl", Hash: "h", Project: "historical", Messages: []IndexedMessage{{UUID: "retained", Text: "payload", ContentType: "text"}}}
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec("UPDATE search_items SET project_evidence=NULL"); err != nil {
				t.Fatal(err)
			}
			file.ProjectEvidence = "unknown"
			if empty {
				file.Messages = nil
			}
			if err := db.SyncFiles([]IndexedFile{file}); err != nil {
				t.Fatal(err)
			}
			pending, err := db.PendingProjectEvidencePathsContext(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 0 {
				t.Fatalf("completed parse remains pending: %v", pending)
			}
			var proof string
			if err := db.db.QueryRow("SELECT project_evidence FROM search_items WHERE uuid='retained'").Scan(&proof); err != nil {
				t.Fatal(err)
			}
			if proof != "unknown" {
				t.Fatal(proof)
			}
		})
	}
}

func TestRecoveryPreservesPendingAndCompletedProjectReplay(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := IndexedFile{Source: "session", SourcePath: "/session.jsonl", Hash: "h", Project: "historical", ProjectEvidence: "unknown", Messages: []IndexedMessage{{UUID: "11111111-1111-4111-8111-111111111111", Text: "pending", ContentType: "text"}, {Ordinal: 1, UUID: "22222222-2222-4222-8222-222222222222", Text: "completed", ContentType: "text"}}}
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("UPDATE search_items SET project_evidence=NULL WHERE ordinal=0"); err != nil {
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
	recovered, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	var pending, completed sql.NullString
	if err := recovered.db.QueryRow("SELECT project_evidence FROM search_items WHERE ordinal=0").Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if err := recovered.db.QueryRow("SELECT project_evidence FROM search_items WHERE ordinal=1").Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if pending.Valid || completed.String != "unknown" {
		t.Fatalf("recovery collapsed pending/completed: %+v %+v", pending, completed)
	}
	paths, err := recovered.PendingProjectEvidencePathsContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != file.SourcePath {
		t.Fatalf("pending replay lost: %v", paths)
	}
	file.Project, file.ProjectEvidence = "native-label", "native_cwd"
	file.Messages = file.Messages[:1]
	if err := recovered.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if err := recovered.db.QueryRow("SELECT project_evidence FROM search_items WHERE ordinal=0").Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending.String != "native_cwd" {
		t.Fatalf("recovered pending row cannot gain native proof: %+v", pending)
	}
}

func TestRecoveryDuplicatePendingDestinationReadback(t *testing.T) {
	for _, pendingFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(pendingFirst), func(t *testing.T) {
			uuid, project := "11111111-1111-4111-8111-111111111111", "historical"
			pending := models.IndexedRecord{UUID: &uuid, Source: "session", SourcePath: "/unavailable/session.jsonl", Ordinal: 0, Role: "user", Text: "retained payload", Project: &project, ContentType: "text"}
			completed := pending
			completed.ProjectEvidence = "unknown"
			inputs := []compat.RecoveryInput{{Records: []models.IndexedRecord{completed}}, {Records: []models.IndexedRecord{pending}}}
			if pendingFirst {
				inputs[0], inputs[1] = inputs[1], inputs[0]
			}
			plan, diags, err := compat.PlanRecovery(inputs)
			if err != nil || len(diags) != 0 {
				t.Fatalf("%v %+v", err, diags)
			}
			path, err := CreateRecoveryDestination(context.Background(), t.TempDir(), plan)
			if err != nil {
				t.Fatal(err)
			}
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var proof sql.NullString
			if err := db.db.QueryRow("SELECT project_evidence FROM search_items").Scan(&proof); err != nil {
				t.Fatal(err)
			}
			if proof.Valid {
				t.Fatalf("pending marker lost on install: %+v", proof)
			}
			readback, diag, err := ReadRecoveryInput(context.Background(), db)
			if err != nil || diag != nil {
				t.Fatalf("%v %+v", err, diag)
			}
			if len(readback.Records) != 1 || readback.Records[0].ProjectEvidence != "" {
				t.Fatalf("pending lost on readback: %+v", readback)
			}
			paths, err := db.PendingProjectEvidencePathsContext(context.Background())
			if err != nil || len(paths) != 1 || paths[0] != pending.SourcePath {
				t.Fatalf("replay eligibility=%v err=%v", paths, err)
			}
		})
	}
}
