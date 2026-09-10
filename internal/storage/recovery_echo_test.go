package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/models"
)

func TestRecoveryPreservesPositiveEchoEvidenceAcrossLegacyDuplicates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "expired.jsonl", Hash: "h", Messages: []IndexedMessage{{Ordinal: 0, Text: "orchard output", UUID: "11111111-1111-4111-8111-111111111111", Role: "user", ContentType: "tool", SearchEcho: true}}}}); err != nil {
		t.Fatal(err)
	}
	current, diag, err := ReadRecoveryInput(context.Background(), db)
	if err != nil || diag != nil {
		t.Fatalf("read: %v %v", diag, err)
	}
	if len(current.Records) != 1 || !current.Records[0].SearchEcho {
		t.Fatal("recovery read lost paired echo evidence")
	}
	legacy := current
	legacy.Records = append([]models.IndexedRecord(nil), current.Records...)
	legacy.Records[0].SearchEcho = false
	for _, inputs := range [][]compat.RecoveryInput{{legacy, current}, {current, legacy}} {
		plan, diags, err := compat.PlanRecovery(inputs)
		if err != nil || len(diags) > 0 {
			t.Fatalf("plan: %v %v", diags, err)
		}
		if len(plan.Records) != 1 || !plan.Records[0].Record.SearchEcho || plan.ExactDuplicates != 1 {
			t.Fatalf("union lost evidence: %+v", plan)
		}
		dest, err := CreateRecoveryDestination(context.Background(), t.TempDir(), plan)
		if err != nil {
			t.Fatal(err)
		}
		out, err := OpenReadOnly(dest)
		if err != nil {
			t.Fatal(err)
		}
		results, err := out.Search("orchard", models.SearchOptions{AllProjects: true})
		if err != nil || len(results) != 0 {
			t.Errorf("recovered echo reappeared: %v %v", results, err)
		}
		results, err = out.Search("orchard", models.SearchOptions{AllProjects: true, ContentType: "tool"})
		if err != nil || len(results) != 1 {
			t.Errorf("recovered explicit tool lost: %v %v", results, err)
		}
		out.Close()
	}
}
