package compat

import (
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestPlanRecoveryMergesUnknownWithProvenOriginWithoutChangingIdentity(t *testing.T) {
	unknown := recordWithUUID(uuidA, "/origin/session.jsonl", 1)
	unknown.Origin = models.OriginUnknown
	unknown.SearchEcho = true
	proven := unknown
	proven.Origin = models.OriginHuman
	proven.SearchEcho = false

	for _, records := range [][]models.IndexedRecord{{unknown, proven}, {proven, unknown}} {
		plan, diagnostics, err := PlanRecovery([]RecoveryInput{recoveryInput("origin", 16, records...)})
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("PlanRecovery error=%v diagnostics=%+v", err, diagnostics)
		}
		if len(plan.Records) != 1 || plan.ExactDuplicates != 1 {
			t.Fatalf("records=%d duplicates=%d, want 1 and 1", len(plan.Records), plan.ExactDuplicates)
		}
		got := plan.Records[0].Record
		if got.Origin != models.OriginHuman {
			t.Fatalf("merged origin = %q, want %q", got.Origin, models.OriginHuman)
		}
		if !got.SearchEcho {
			t.Fatal("origin enrichment erased independent search-echo evidence")
		}
	}

	unknownPlan, diagnostics, err := PlanRecovery([]RecoveryInput{recoveryInput("legacy", 15, unknown)})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("unknown PlanRecovery error=%v diagnostics=%+v", err, diagnostics)
	}
	provenPlan, diagnostics, err := PlanRecovery([]RecoveryInput{recoveryInput("current", 16, proven)})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("proven PlanRecovery error=%v diagnostics=%+v", err, diagnostics)
	}
	if unknownPlan.Records[0].PayloadHash != provenPlan.Records[0].PayloadHash {
		t.Fatal("origin participated in payload hash and would prevent enrichment")
	}
}

func TestPlanRecoveryAcceptsMatchingProvenOrigins(t *testing.T) {
	first := recordWithUUID(uuidB, "/origin/matching.jsonl", 2)
	first.Origin = models.OriginAutomation
	second := first
	second.SearchEcho = true

	plan, diagnostics, err := PlanRecovery([]RecoveryInput{
		recoveryInput("active", 16, first),
		recoveryInput("stranded", 16, second),
	})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("PlanRecovery error=%v diagnostics=%+v", err, diagnostics)
	}
	if len(plan.Records) != 1 || plan.ExactDuplicates != 1 || plan.Records[0].Record.Origin != models.OriginAutomation || !plan.Records[0].Record.SearchEcho {
		t.Fatalf("matching-origin plan = %+v", plan)
	}
}

func TestPlanRecoveryRejectsContradictoryProvenOrigins(t *testing.T) {
	human := recordWithUUID(uuidC+"#r0", "/origin/conflict.jsonl", 3)
	human.Origin = models.OriginHuman
	assistant := human
	assistant.Origin = models.OriginAssistant

	plan, diagnostics, err := PlanRecovery([]RecoveryInput{
		recoveryInput("active", 16, human),
		recoveryInput("stranded", 16, assistant),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Records != nil || plan.ExactDuplicates != 0 || len(diagnostics) != 2 {
		t.Fatalf("plan=%+v diagnostics=%+v", plan, diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != CodeRecoveryConflict || !strings.Contains(diagnostic.Summary, "contradictory proven message origins") {
			t.Fatalf("diagnostic=%+v", diagnostic)
		}
	}
}

func TestPlanRecoveryRejectsInvalidOrigin(t *testing.T) {
	record := recordWithUUID(uuidA, "/origin/invalid.jsonl", 4)
	record.Origin = models.MessageOrigin("operator")

	plan, diagnostics, err := PlanRecovery([]RecoveryInput{recoveryInput("invalid", 16, record)})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Records != nil || len(diagnostics) != 1 || diagnostics[0].Code != CodeUninterpretableRow || !strings.Contains(diagnostics[0].Summary, "invalid message origin") {
		t.Fatalf("plan=%+v diagnostics=%+v", plan, diagnostics)
	}
}
