package compat

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/pablontiv/backscroll/internal/models"
)

var errUnsafeIdentity = errors.New("unsafe recovery identity")

type recordIdentity struct {
	Kind       string
	UUID       string
	SourcePath string
	Ordinal    int64
}

// PlanRecovery builds a deterministic, read-only union of canonical recovery
// records. If any input row cannot be interpreted safely, or if one identity has
// multiple canonical payloads, the returned records are intentionally empty and
// diagnostics describe every rejected occurrence.
func PlanRecovery(inputs []RecoveryInput) (RecoveryPlan, []Diagnostic, error) {
	plan := RecoveryPlan{InputShapes: make([]SchemaShape, 0, len(inputs))}
	byIdentity := map[recordIdentity]map[string][]recordOccurrence{}
	var diagnostics []Diagnostic

	for inputIndex, input := range inputs {
		plan.InputShapes = append(plan.InputShapes, input.Shape)
		for rowIndex, record := range input.Records {
			identity, err := identityOf(record)
			if err != nil {
				diagnostics = append(diagnostics, uninterpretablePlanDiagnostic(inputIndex, rowIndex, record, err))
				continue
			}
			hash := payloadHash(record, identity)
			if byIdentity[identity] == nil {
				byIdentity[identity] = map[string][]recordOccurrence{}
			}
			byIdentity[identity][hash] = append(byIdentity[identity][hash], recordOccurrence{
				inputIndex: inputIndex,
				rowIndex:   rowIndex,
				identity:   identity,
				hash:       hash,
				record:     record,
			})
		}
	}

	identities := make([]recordIdentity, 0, len(byIdentity))
	for identity := range byIdentity {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool { return identityLess(identities[i], identities[j]) })

	var canonical []plannedRecord
	for _, identity := range identities {
		groups := byIdentity[identity]
		if len(groups) > 1 {
			var rejected []recordOccurrence
			for _, occurrences := range groups {
				rejected = append(rejected, occurrences...)
			}
			sortOccurrences(rejected)
			for _, occurrence := range rejected {
				diagnostics = append(diagnostics, conflictPlanDiagnostic(occurrence, len(groups)))
			}
			continue
		}
		for hash, occurrences := range groups {
			sortOccurrences(occurrences)
			canonical = append(canonical, plannedRecord{
				identity: identity,
				hash:     hash,
				record:   occurrences[0].record,
			})
			plan.ExactDuplicates += len(occurrences) - 1
		}
	}

	if len(diagnostics) != 0 {
		plan.Records = nil
		plan.ExactDuplicates = 0
		return plan, diagnostics, nil
	}

	sort.Slice(canonical, func(i, j int) bool {
		if !sameIdentity(canonical[i].identity, canonical[j].identity) {
			return identityLess(canonical[i].identity, canonical[j].identity)
		}
		return canonical[i].hash < canonical[j].hash
	})
	plan.Records = make([]CanonicalRecord, 0, len(canonical))
	for _, record := range canonical {
		plan.Records = append(plan.Records, CanonicalRecord{
			Record:      record.record,
			PayloadHash: record.hash,
		})
	}
	return plan, nil, nil
}

func identityOf(r models.IndexedRecord) (recordIdentity, error) {
	if r.UUID != nil && *r.UUID != "" {
		if _, err := uuid.Parse(*r.UUID); err != nil {
			return recordIdentity{}, err
		}
		return recordIdentity{Kind: "uuid", UUID: *r.UUID}, nil
	}
	if r.SourcePath == "" || r.Ordinal < 0 {
		return recordIdentity{}, errUnsafeIdentity
	}
	return recordIdentity{Kind: "path_ordinal", SourcePath: r.SourcePath, Ordinal: r.Ordinal}, nil
}

type recordOccurrence struct {
	inputIndex int
	rowIndex   int
	identity   recordIdentity
	hash       string
	record     models.IndexedRecord
}

type plannedRecord struct {
	identity recordIdentity
	hash     string
	record   models.IndexedRecord
}

func payloadHash(record models.IndexedRecord, identity recordIdentity) string {
	var encoded bytes.Buffer
	writeString(&encoded, record.Source)
	if identity.Kind != "path_ordinal" {
		writeString(&encoded, record.SourcePath)
		writeInt64(&encoded, record.Ordinal)
	}
	writeString(&encoded, record.Role)
	writeString(&encoded, record.Text)
	writeStringPtr(&encoded, record.Project)
	if identity.Kind == "path_ordinal" {
		writeString(&encoded, "")
	}
	writeStringPtr(&encoded, record.Timestamp)
	writeString(&encoded, record.ContentType)

	sum := sha256.Sum256(encoded.Bytes())
	return fmt.Sprintf("%x", sum)
}

func writeString(buffer *bytes.Buffer, value string) {
	_ = buffer.WriteByte(1)
	writeLengthPrefixed(buffer, []byte(value))
}

func writeStringPtr(buffer *bytes.Buffer, value *string) {
	if value == nil {
		_ = buffer.WriteByte(0)
		return
	}
	_ = buffer.WriteByte(1)
	writeLengthPrefixed(buffer, []byte(*value))
}

func writeInt64(buffer *bytes.Buffer, value int64) {
	_ = buffer.WriteByte(1)
	writeLengthPrefixed(buffer, []byte(fmt.Sprintf("%d", value)))
}

func writeLengthPrefixed(buffer *bytes.Buffer, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = buffer.Write(length[:])
	_, _ = buffer.Write(value)
}

func identityLess(left, right recordIdentity) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.UUID != right.UUID {
		return left.UUID < right.UUID
	}
	if left.SourcePath != right.SourcePath {
		return left.SourcePath < right.SourcePath
	}
	return left.Ordinal < right.Ordinal
}

func sameIdentity(left, right recordIdentity) bool {
	return left.Kind == right.Kind && left.UUID == right.UUID && left.SourcePath == right.SourcePath && left.Ordinal == right.Ordinal
}

func sortOccurrences(occurrences []recordOccurrence) {
	sort.Slice(occurrences, func(i, j int) bool {
		if occurrences[i].inputIndex != occurrences[j].inputIndex {
			return occurrences[i].inputIndex < occurrences[j].inputIndex
		}
		return occurrences[i].rowIndex < occurrences[j].rowIndex
	})
}

func uninterpretablePlanDiagnostic(inputIndex, rowIndex int, record models.IndexedRecord, cause error) Diagnostic {
	identityEvidence := fmt.Sprintf("uuid=%s source_path=%q ordinal=%d", pointerEvidence(record.UUID), record.SourcePath, record.Ordinal)
	return Diagnostic{
		Code:    CodeUninterpretableRow,
		Summary: fmt.Sprintf("recovery input %d row %d has uninterpretable identity (%s): %v", inputIndex, rowIndex, identityEvidence, cause),
	}
}

func conflictPlanDiagnostic(occurrence recordOccurrence, payloadCount int) Diagnostic {
	return Diagnostic{
		Code:    CodeRecoveryConflict,
		Summary: fmt.Sprintf("recovery input %d row %d conflicts for %s with %d payload hashes", occurrence.inputIndex, occurrence.rowIndex, describeIdentity(occurrence.identity), payloadCount),
	}
}

func describeIdentity(identity recordIdentity) string {
	if identity.Kind == "uuid" {
		return "uuid " + identity.UUID
	}
	return fmt.Sprintf("path_ordinal source_path=%q ordinal=%d", identity.SourcePath, identity.Ordinal)
}

func pointerEvidence(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%q", *value)
}
