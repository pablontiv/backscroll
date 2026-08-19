package compat

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

const (
	uuidA = "11111111-1111-4111-8111-111111111111"
	uuidB = "22222222-2222-4222-8222-222222222222"
	uuidC = "33333333-3333-4333-8333-333333333333"
)

func TestRecoverIdentityAndConflictMatrixAcrossInputs(t *testing.T) {
	empty := ""
	emptyProject := ""

	tests := []struct {
		name             string
		records          []models.IndexedRecord
		wantRecords      int
		wantDuplicates   int
		wantDiagnostics  map[Code]int
		wantSummaryParts []string
	}{
		{
			name: "valid UUID identity beats differing path ordinal and reports payload conflict",
			records: []models.IndexedRecord{
				recordWithUUID(uuidA, "/active/path", 1),
				recordWithUUID(uuidA, "/stranded/path", 9),
			},
			wantDiagnostics:  map[Code]int{CodeRecoveryConflict: 2},
			wantSummaryParts: []string{uuidA},
		},
		{
			name: "empty UUID falls back to exact path ordinal and collapses nil and empty UUID",
			records: []models.IndexedRecord{
				recordWithoutUUID("/exact/path", 7, nil),
				recordWithoutUUID("/exact/path", 7, &empty),
			},
			wantRecords:    1,
			wantDuplicates: 1,
		},
		{
			name: "equal UUID identity and equal canonical payload collapses",
			records: []models.IndexedRecord{
				recordWithUUID(uuidA, "/same/path", 1),
				recordWithUUID(uuidA, "/same/path", 1),
			},
			wantRecords:    1,
			wantDuplicates: 1,
		},
		{
			name: "equal UUID identity and different text payload conflicts",
			records: []models.IndexedRecord{
				recordWithUUID(uuidA, "/same/path", 1),
				withText(recordWithUUID(uuidA, "/same/path", 1), "different text"),
			},
			wantDiagnostics:  map[Code]int{CodeRecoveryConflict: 2},
			wantSummaryParts: []string{uuidA},
		},
		{
			name: "equal path ordinal identity and nil versus empty project pointer conflicts",
			records: []models.IndexedRecord{
				recordWithoutUUID("/same/path", 1, nil),
				withProject(recordWithoutUUID("/same/path", 1, nil), &emptyProject),
			},
			wantDiagnostics:  map[Code]int{CodeRecoveryConflict: 2},
			wantSummaryParts: []string{"/same/path", "1"},
		},
		{
			name: "equal content under different UUID identities remains two records",
			records: []models.IndexedRecord{
				recordWithUUID(uuidA, "/same/path", 1),
				recordWithUUID(uuidB, "/same/path", 1),
			},
			wantRecords: 2,
		},
		{
			name: "invalid non-empty UUID is uninterpretable and never falls back to path ordinal",
			records: []models.IndexedRecord{
				recordWithUUID("not-a-uuid", "/safe/path", 3),
				recordWithoutUUID("/safe/path", 3, nil),
			},
			wantDiagnostics:  map[Code]int{CodeUninterpretableRow: 1},
			wantSummaryParts: []string{"not-a-uuid"},
		},
		{
			name: "empty UUID with empty path or negative ordinal is uninterpretable",
			records: []models.IndexedRecord{
				recordWithoutUUID("", 3, nil),
				recordWithoutUUID("/safe/path", -1, &empty),
			},
			wantDiagnostics:  map[Code]int{CodeUninterpretableRow: 2},
			wantSummaryParts: []string{"/safe/path", "-1"},
		},
	}

	for _, tt := range tests {
		for _, layout := range []struct {
			name   string
			inputs []RecoveryInput
		}{
			{name: "within-active-input", inputs: []RecoveryInput{recoveryInput("active", 13, tt.records...)}},
			{name: "across-active-and-stranded-inputs", inputs: splitAcrossActiveAndStranded(tt.records)},
		} {
			t.Run(tt.name+"/"+layout.name, func(t *testing.T) {
				plan, diagnostics, err := PlanRecovery(layout.inputs)
				if err != nil {
					t.Fatalf("PlanRecovery error = %v", err)
				}

				assertInputShapes(t, plan, layout.inputs)
				assertDiagnosticCounts(t, diagnostics, tt.wantDiagnostics)
				assertDiagnosticSummariesContain(t, diagnostics, tt.wantSummaryParts)

				if len(tt.wantDiagnostics) > 0 {
					if len(plan.Records) != 0 {
						t.Fatalf("plan records = %d, want 0 when diagnostics reject the plan", len(plan.Records))
					}
					return
				}
				if len(diagnostics) != 0 {
					t.Fatalf("diagnostics = %+v, want none", diagnostics)
				}
				if len(plan.Records) != tt.wantRecords || plan.ExactDuplicates != tt.wantDuplicates {
					t.Fatalf("plan records=%d duplicates=%d, want records=%d duplicates=%d", len(plan.Records), plan.ExactDuplicates, tt.wantRecords, tt.wantDuplicates)
				}
				inputRows := 0
				for _, input := range layout.inputs {
					inputRows += len(input.Records)
				}
				if got := len(plan.Records) + plan.ExactDuplicates; got != inputRows {
					t.Fatalf("clean accounting records + duplicates = %d, want input rows %d", got, inputRows)
				}
			})
		}
	}
}

func TestPlanRecoveryAccountsForEveryInputRow(t *testing.T) {
	active := recoveryInput("active", 13,
		recordWithUUID(uuidA, "/active/a", 1),
		recordWithUUID(uuidA, "/active/a", 1),
		recordWithoutUUID("/active/path", 2, nil),
	)
	stranded := recoveryInput("stranded", 7,
		recordWithoutUUID("/active/path", 2, nil),
		recordWithUUID(uuidB, "/stranded/b", 3),
	)

	plan, diagnostics, err := PlanRecovery([]RecoveryInput{active, stranded})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("PlanRecovery error=%v diagnostics=%+v", err, diagnostics)
	}
	assertInputShapes(t, plan, []RecoveryInput{active, stranded})
	if len(plan.Records) != 3 || plan.ExactDuplicates != 2 {
		t.Fatalf("records=%d duplicates=%d, want records=3 duplicates=2", len(plan.Records), plan.ExactDuplicates)
	}
	if got, want := len(plan.Records)+plan.ExactDuplicates, len(active.Records)+len(stranded.Records); got != want {
		t.Fatalf("records + duplicates = %d, want input rows %d", got, want)
	}

	conflicting := recoveryInput("conflicting", 6,
		withText(recordWithUUID(uuidC, "/conflict", 1), "left"),
		withText(recordWithUUID(uuidC, "/conflict", 1), "right"),
		recordWithUUID("not-a-uuid", "/safe", 0),
	)
	plan, diagnostics, err = PlanRecovery([]RecoveryInput{active, conflicting})
	if err != nil {
		t.Fatalf("PlanRecovery conflict error = %v", err)
	}
	if len(plan.Records) != 0 {
		t.Fatalf("conflict plan records = %d, want 0", len(plan.Records))
	}
	assertDiagnosticCounts(t, diagnostics, map[Code]int{CodeRecoveryConflict: 2, CodeUninterpretableRow: 1})
	assertDiagnosticSummariesContain(t, diagnostics, []string{uuidC, "not-a-uuid"})
}

func TestPlanRecoveryPreservesDistinctHashEquivalentIdentities(t *testing.T) {
	first := recordWithUUID(uuidA, "/same/path", 1)
	second := recordWithUUID(uuidB, "/same/path", 1)
	ambiguousA := withSourceAndRole(recordWithUUID(uuidC, "/injective", 1), "ab", "c")
	ambiguousB := withSourceAndRole(recordWithUUID("44444444-4444-4444-8444-444444444444", "/injective", 1), "a", "bc")

	plan, diagnostics, err := PlanRecovery([]RecoveryInput{recoveryInput("active", 13, first, second, ambiguousA, ambiguousB)})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("PlanRecovery error=%v diagnostics=%+v", err, diagnostics)
	}
	if len(plan.Records) != 4 {
		t.Fatalf("records = %d, want 4 distinct identities", len(plan.Records))
	}

	hashesByUUID := map[string]string{}
	for _, record := range plan.Records {
		if record.Record.UUID == nil {
			t.Fatalf("record without UUID in UUID-only test: %+v", record.Record)
		}
		hashesByUUID[*record.Record.UUID] = record.PayloadHash
	}
	if hashesByUUID[uuidA] == "" || hashesByUUID[uuidA] != hashesByUUID[uuidB] {
		t.Fatalf("equal payload under distinct identities hashes = %q and %q, want equal non-empty hashes", hashesByUUID[uuidA], hashesByUUID[uuidB])
	}
	if hashesByUUID[uuidC] == "" || hashesByUUID[uuidC] == hashesByUUID["44444444-4444-4444-8444-444444444444"] {
		t.Fatalf("ambiguous field payload hashes = %q and %q, want distinct length-prefixed hashes", hashesByUUID[uuidC], hashesByUUID["44444444-4444-4444-8444-444444444444"])
	}
}

func TestPlanRecoveryIsDeterministic(t *testing.T) {
	inputs := []RecoveryInput{
		recoveryInput("active", 13,
			recordWithUUID(uuidB, "/uuid/b", 2),
			recordWithoutUUID("/path/z", 9, nil),
			recordWithUUID(uuidA, "/uuid/a", 1),
			recordWithoutUUID("/path/a", 1, nil),
		),
		recoveryInput("stranded", 7,
			recordWithUUID(uuidC, "/uuid/c", 3),
		),
	}
	reordered := []RecoveryInput{
		recoveryInput("active", 13,
			recordWithoutUUID("/path/a", 1, nil),
			recordWithUUID(uuidA, "/uuid/a", 1),
			recordWithoutUUID("/path/z", 9, nil),
			recordWithUUID(uuidB, "/uuid/b", 2),
		),
		recoveryInput("stranded", 7,
			recordWithUUID(uuidC, "/uuid/c", 3),
		),
	}

	first, diagnostics, err := PlanRecovery(inputs)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("PlanRecovery first error=%v diagnostics=%+v", err, diagnostics)
	}
	second, diagnostics, err := PlanRecovery(reordered)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("PlanRecovery second error=%v diagnostics=%+v", err, diagnostics)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("plans differ after input row reorder:\nfirst=%+v\nsecond=%+v", first, second)
	}

	gotOrder := make([]string, 0, len(first.Records))
	for _, record := range first.Records {
		if record.Record.UUID != nil && *record.Record.UUID != "" {
			gotOrder = append(gotOrder, "uuid:"+*record.Record.UUID)
			continue
		}
		gotOrder = append(gotOrder, fmt.Sprintf("path:%s:%d", record.Record.SourcePath, record.Record.Ordinal))
	}
	wantOrder := []string{
		"path:/path/a:1",
		"path:/path/z:9",
		"uuid:" + uuidA,
		"uuid:" + uuidB,
		"uuid:" + uuidC,
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("record order = %#v, want %#v", gotOrder, wantOrder)
	}
}

func recoveryInput(label string, appliedVersion int, records ...models.IndexedRecord) RecoveryInput {
	return RecoveryInput{
		Shape: SchemaShape{
			AppliedVersion: appliedVersion,
			Signature:      "sha256:" + label,
		},
		Records:  records,
		RowCount: len(records),
	}
}

func splitAcrossActiveAndStranded(records []models.IndexedRecord) []RecoveryInput {
	if len(records) == 0 {
		return []RecoveryInput{recoveryInput("active", 13), recoveryInput("stranded", 7)}
	}
	return []RecoveryInput{
		recoveryInput("active", 13, records[0]),
		recoveryInput("stranded", 7, records[1:]...),
	}
}

func recordWithUUID(uuid, sourcePath string, ordinal int64) models.IndexedRecord {
	record := baseRecord(sourcePath, ordinal)
	record.UUID = strPtr(uuid)
	return record
}

func recordWithoutUUID(sourcePath string, ordinal int64, uuid *string) models.IndexedRecord {
	record := baseRecord(sourcePath, ordinal)
	record.UUID = uuid
	return record
}

func baseRecord(sourcePath string, ordinal int64) models.IndexedRecord {
	project := "project"
	timestamp := "2026-08-18T23:00:00Z"
	return models.IndexedRecord{
		Source:      "claude",
		SourcePath:  sourcePath,
		Ordinal:     ordinal,
		Role:        "user",
		Text:        "hello recovery",
		Project:     &project,
		Timestamp:   &timestamp,
		ContentType: "text/plain",
	}
}

func withText(record models.IndexedRecord, text string) models.IndexedRecord {
	record.Text = text
	return record
}

func withProject(record models.IndexedRecord, project *string) models.IndexedRecord {
	record.Project = project
	return record
}

func withSourceAndRole(record models.IndexedRecord, source, role string) models.IndexedRecord {
	record.Source = source
	record.Role = role
	return record
}

func strPtr(value string) *string {
	return &value
}

func assertInputShapes(t *testing.T, plan RecoveryPlan, inputs []RecoveryInput) {
	t.Helper()
	if len(plan.InputShapes) != len(inputs) {
		t.Fatalf("input shape count = %d, want %d", len(plan.InputShapes), len(inputs))
	}
	for i, input := range inputs {
		if plan.InputShapes[i] != input.Shape {
			t.Fatalf("input shape %d = %+v, want %+v", i, plan.InputShapes[i], input.Shape)
		}
	}
}

func assertDiagnosticCounts(t *testing.T, diagnostics []Diagnostic, want map[Code]int) {
	t.Helper()
	got := map[Code]int{}
	for _, diagnostic := range diagnostics {
		got[diagnostic.Code]++
	}
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic counts = %#v from diagnostics %+v, want %#v", got, diagnostics, want)
	}
}

func assertDiagnosticSummariesContain(t *testing.T, diagnostics []Diagnostic, parts []string) {
	t.Helper()
	joined := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Summary == "" {
			t.Fatalf("empty diagnostic summary in %+v", diagnostics)
		}
		joined = append(joined, diagnostic.Summary)
	}
	all := strings.Join(joined, "\n")
	for _, part := range parts {
		if !strings.Contains(all, part) {
			t.Fatalf("diagnostic summaries %q do not contain %q", all, part)
		}
	}
}
