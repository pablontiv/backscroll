package compat

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

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
	plan := RecoveryPlan{
		InputShapes:           make([]SchemaShape, 0, len(inputs)),
		IngestionFingerprints: make(map[string]string),
	}
	ambiguousFingerprints := make(map[string]bool)
	byIdentity := map[recordIdentity]map[string][]recordOccurrence{}
	var diagnostics []Diagnostic
	type nativeSessionKey struct{ path, session string }
	nativeProjects := make(map[nativeSessionKey]*string)

	for inputIndex, input := range inputs {
		plan.InputShapes = append(plan.InputShapes, input.Shape)
		for path, fingerprint := range input.IngestionFingerprints {
			if path == "" || fingerprint == "" || ambiguousFingerprints[path] {
				continue
			}
			if existing, ok := plan.IngestionFingerprints[path]; ok && existing != fingerprint {
				delete(plan.IngestionFingerprints, path)
				ambiguousFingerprints[path] = true
				continue
			}
			plan.IngestionFingerprints[path] = fingerprint
		}
		for rowIndex, record := range input.Records {
			if record.SessionID != "" && record.ProjectEvidence == "native_cwd" {
				key := nativeSessionKey{record.SourcePath, record.SessionID}
				if project, found := nativeProjects[key]; found && !equalProject(project, record.Project) {
					diagnostics = append(diagnostics, Diagnostic{Code: CodeRecoveryConflict, Summary: fmt.Sprintf("recovery input %d row %d has contradictory native project proof for %q session %q", inputIndex, rowIndex, key.path, key.session)})
				} else {
					nativeProjects[key] = record.Project
				}
			}
			if record.Origin == "" {
				record.Origin = models.OriginUnknown
			} else if !models.ValidMessageOrigin(record.Origin) {
				diagnostics = append(diagnostics, uninterpretablePlanDiagnostic(inputIndex, rowIndex, record, fmt.Errorf("invalid message origin %q", record.Origin)))
				continue
			}
			if !models.ValidProjectEvidence(record.ProjectEvidence) {
				diagnostics = append(diagnostics, uninterpretablePlanDiagnostic(inputIndex, rowIndex, record, fmt.Errorf("invalid project evidence %q", record.ProjectEvidence)))
				continue
			}
			identity, err := identityOf(record)
			if err != nil {
				diagnostics = append(diagnostics, uninterpretablePlanDiagnostic(inputIndex, rowIndex, record, err))
				continue
			}
			// Group equal payloads independently of project proof, then reconcile
			// only compatible unknown/native enrichment below.
			groupRecord := record
			groupRecord.Project = nil
			hash := payloadHash(groupRecord, identity)
			if byIdentity[identity] == nil {
				byIdentity[identity] = map[string][]recordOccurrence{}
			}
			byIdentity[identity][hash] = append(byIdentity[identity][hash], recordOccurrence{
				inputIndex: inputIndex,
				rowIndex:   rowIndex,
				identity:   identity,
				hash:       payloadHash(record, identity),
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
		for _, occurrences := range groups {
			sortOccurrences(occurrences)
			// Provenance enriches an otherwise equal canonical payload. Legacy
			// unknown evidence never erases proof, and search-echo evidence remains
			// independent from message-origin evidence.
			record := occurrences[0].record
			originConflict := false
			sessionConflict := false
			projectConflict := false
			for _, occurrence := range occurrences {
				if record.SessionID != "" && occurrence.record.SessionID != "" && record.SessionID != occurrence.record.SessionID {
					sessionConflict = true
					break
				}
				other := occurrence.record
				if !equalProject(record.Project, other.Project) {
					switch {
					case other.SessionID != "" && other.ProjectEvidence == "native_cwd" && (record.ProjectEvidence == "" || record.ProjectEvidence == "unknown" || record.ProjectEvidence == "path_fallback"):
						record.Project = other.Project
					case record.SessionID != "" && record.ProjectEvidence == "native_cwd" && (other.ProjectEvidence == "" || other.ProjectEvidence == "unknown" || other.ProjectEvidence == "path_fallback"):
					default:
						projectConflict = true
					}
				}
				if record.SessionID == "" {
					record.SessionID = occurrence.record.SessionID
				}
				record.SearchEcho = record.SearchEcho || occurrence.record.SearchEcho
				record.ProjectEvidence = mergeProjectEvidence(record.ProjectEvidence, occurrence.record.ProjectEvidence)
				merged, ok := mergeMessageOrigins(record.Origin, occurrence.record.Origin)
				if !ok {
					originConflict = true
					break
				}
				record.Origin = merged
			}
			if originConflict || sessionConflict || projectConflict {
				for _, occurrence := range occurrences {
					diagnostic := originConflictPlanDiagnostic(occurrence)
					if sessionConflict {
						diagnostic.Summary = fmt.Sprintf("conflicting native session identity for %s", describeIdentity(occurrence.identity))
					}
					if projectConflict {
						diagnostic = conflictPlanDiagnostic(occurrence, 2)
					}
					diagnostics = append(diagnostics, diagnostic)
				}
				continue
			}
			canonical = append(canonical, plannedRecord{
				identity: identity,
				hash:     payloadHash(record, identity),
				record:   record,
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
	for i := range canonical {
		record := &canonical[i].record
		if record.SessionID != "" && (record.ProjectEvidence == "" || record.ProjectEvidence == "unknown" || record.ProjectEvidence == "path_fallback") {
			if project, found := nativeProjects[nativeSessionKey{record.SourcePath, record.SessionID}]; found {
				record.Project = project
				record.ProjectEvidence = "native_cwd"
				canonical[i].hash = payloadHash(*record, canonical[i].identity)
			}
		}
	}
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
		if err := ValidateRecoveryUUID(*r.UUID); err != nil {
			return recordIdentity{}, err
		}
		return recordIdentity{Kind: "uuid", UUID: *r.UUID}, nil
	}
	if r.SourcePath == "" || r.Ordinal < 0 {
		return recordIdentity{}, errUnsafeIdentity
	}
	return recordIdentity{Kind: "path_ordinal", SourcePath: r.SourcePath, Ordinal: r.Ordinal}, nil
}

// ValidateRecoveryUUID accepts a canonical RFC UUID or the exact Claude
// derived forms <uuid>#tN and <uuid>#rN. The complete accepted value remains
// the recovery identity; this function only validates its syntax.
func ValidateRecoveryUUID(value string) error {
	base := value
	if separator := strings.IndexByte(value, '#'); separator >= 0 {
		base = value[:separator]
		suffix := value[separator+1:]
		if len(suffix) < 2 || (suffix[0] != 't' && suffix[0] != 'r') {
			return fmt.Errorf("invalid Claude recovery UUID suffix in %q", value)
		}
		for _, digit := range suffix[1:] {
			if digit < '0' || digit > '9' {
				return fmt.Errorf("invalid Claude recovery UUID suffix in %q", value)
			}
		}
	}

	parsed, err := uuid.Parse(base)
	if err != nil {
		return fmt.Errorf("invalid recovery UUID %q: %w", value, err)
	}
	if parsed.String() != base {
		return fmt.Errorf("recovery UUID base %q is not canonical", base)
	}
	return nil
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

func mergeMessageOrigins(left, right models.MessageOrigin) (models.MessageOrigin, bool) {
	if left == models.OriginUnknown {
		return right, true
	}
	if right == models.OriginUnknown || left == right {
		return left, true
	}
	return models.OriginUnknown, false
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

func originConflictPlanDiagnostic(occurrence recordOccurrence) Diagnostic {
	return Diagnostic{
		Code:    CodeRecoveryConflict,
		Summary: fmt.Sprintf("recovery input %d row %d conflicts for %s with contradictory proven message origins", occurrence.inputIndex, occurrence.rowIndex, describeIdentity(occurrence.identity)),
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

func projectProofRank(value string) int {
	switch value {
	case "native_cwd":
		return 3
	case "registry":
		return 2
	case "path_fallback":
		return 1
	}
	return 0
}

// mergeProjectEvidence keeps attribution precedence separate from completion.
// Neither unknown nor pending proves a cwd. Without parser epoch/availability
// evidence, an unknown duplicate cannot certify that pending replay completed.
// Actual attribution proof retains its existing precedence and is never erased.
func mergeProjectEvidence(left, right string) string {
	leftRank, rightRank := projectProofRank(left), projectProofRank(right)
	if leftRank > rightRank {
		return left
	}
	if rightRank > leftRank {
		return right
	}
	if leftRank == 0 && (left == "" || right == "") {
		return ""
	}
	return left
}

func equalProject(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
