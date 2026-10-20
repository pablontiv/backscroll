package storage

import (
	"context"
	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/models"
	"testing"
)

func TestNativeSessionRecoveryPreservesProofAndTags(t *testing.T) {
	uuid, project := "11111111-1111-4111-8111-111111111111", "alpha"
	proven := models.IndexedRecord{UUID: &uuid, SessionID: "s1", Source: "session", SourcePath: "/expired.db", Role: "user", Origin: models.OriginHuman, Text: "go test", Project: &project, ProjectEvidence: "native_cwd", ContentType: "text"}
	historical := proven
	historical.SessionID = ""
	fallback := "legacy-path"
	historical.Project = &fallback
	historical.ProjectEvidence = "unknown"
	plan, diags, err := compat.PlanRecovery([]compat.RecoveryInput{{Records: []models.IndexedRecord{historical, proven}, IngestionFingerprints: map[string]string{"/expired.db": "native-fingerprint"}}})
	if err != nil || len(diags) != 0 {
		t.Fatalf("plan %v %v", err, diags)
	}
	path, err := CreateRecoveryDestination(context.Background(), t.TempDir(), plan)
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	input, diag, err := ReadRecoveryInput(context.Background(), db)
	if err != nil || diag != nil || len(input.Records) != 1 || input.Records[0].SessionID != "s1" {
		t.Fatalf("readback %+v %v %+v", input, err, diag)
	}
	if got := input.Records[0]; got.Project == nil || *got.Project != project || got.ProjectEvidence != "native_cwd" || input.IngestionFingerprints["/expired.db"] != "native-fingerprint" {
		t.Fatalf("recovered proof/fingerprint: %+v %+v", got, input.IngestionFingerprints)
	}
	var tags int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM session_identity_tags WHERE session_id='s1' AND tag='testing'`).Scan(&tags); err != nil || tags != 1 {
		t.Fatalf("recovered tags=%d %v", tags, err)
	}
	conflict := proven
	conflict.SessionID = "s2"
	_, diags, err = compat.PlanRecovery([]compat.RecoveryInput{{Records: []models.IndexedRecord{proven, conflict}}})
	if err != nil || len(diags) != 2 {
		t.Fatalf("contradictory session accepted: %v %+v", err, diags)
	}
}

func TestNativeSessionRecoveryRejectsSiblingProjectContradiction(t *testing.T) {
	firstID, secondID, firstProject, secondProject := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "alpha", "beta"
	first := models.IndexedRecord{UUID: &firstID, SessionID: "native", Source: "session", SourcePath: "/expired.db", Role: "user", Text: "first", ContentType: "text", Project: &firstProject, ProjectEvidence: "native_cwd"}
	second := first
	second.UUID = &secondID
	second.Project = &secondProject
	second.Text = "second"
	plan, diagnostics, err := compat.PlanRecovery([]compat.RecoveryInput{{Records: []models.IndexedRecord{first, second}}})
	if err != nil || len(diagnostics) == 0 || len(plan.Records) != 0 {
		t.Fatalf("contradictory native proof: %+v %+v %v", plan, diagnostics, err)
	}
}
