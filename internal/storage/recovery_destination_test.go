package storage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/models"
)

func TestRecoverUnionPreservesActiveAndStrandedRecords(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	strandedPath := filepath.Join(dir, "stranded.db")

	duplicateUUID := "33333333-3333-4333-8333-333333333333"
	activeUniqueUUID := "11111111-1111-4111-8111-111111111111"
	strandedUniqueUUID := "22222222-2222-4222-8222-222222222222"
	createRecoveryDestinationSourceDB(t, activePath, []IndexedFile{
		recoveryDestinationIndexedFile("/sessions/active.jsonl", "active-hash", []IndexedMessage{{
			Ordinal: 0, Role: "user", Text: "active prose sentinelalpha", UUID: activeUniqueUUID,
			Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
		}}),
		recoveryDestinationIndexedFile("/sessions/duplicate.jsonl", "duplicate-active-hash", []IndexedMessage{{
			Ordinal: 0, Role: "assistant", Text: "shared duplicate prose", UUID: duplicateUUID,
			Timestamp: "2026-08-18T00:00:02Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
		}}),
	})
	createRecoveryDestinationSourceDB(t, strandedPath, []IndexedFile{
		recoveryDestinationIndexedFile("/sessions/stranded.jsonl", "stranded-hash", []IndexedMessage{{
			Ordinal: 0, Role: "assistant", Text: "Bash command=strandedtoolomega", UUID: strandedUniqueUUID,
			Timestamp: "2026-08-18T00:00:01Z", ContentType: "tool", ToolName: "Bash", CommandHead: "strandedtoolomega", ExtractionVersion: CurrentExtractionVersion,
		}}),
		recoveryDestinationIndexedFile("/sessions/duplicate.jsonl", "duplicate-stranded-hash", []IndexedMessage{{
			Ordinal: 0, Role: "assistant", Text: "shared duplicate prose", UUID: duplicateUUID,
			Timestamp: "2026-08-18T00:00:02Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
		}}),
	})
	plan := recoveryDestinationPlanFromDBs(t, ctx, activePath, strandedPath)
	activeSnapshot := snapshotRecoveryDB(t, activePath)
	strandedSnapshot := snapshotRecoveryDB(t, strandedPath)
	if got, want := len(plan.Records), 3; got != want {
		t.Fatalf("planned records = %d, want %d", got, want)
	}
	if got, want := plan.ExactDuplicates, 1; got != want {
		t.Fatalf("planned exact duplicates = %d, want %d", got, want)
	}

	destPath, err := CreateRecoveryDestination(ctx, dir, plan)
	if err != nil {
		t.Fatalf("CreateRecoveryDestination: %v", err)
	}
	defer func() { _ = recoveryDestinationRemoveFiles(destPath) }()
	if destPath == activePath || destPath == strandedPath {
		t.Fatalf("destination path %s reused an input path", destPath)
	}

	assertRecoveryDestinationRecords(t, destPath, plan)
	assertRecoveryDestinationFTS(t, destPath, "sentinelalpha", 1, "strandedtoolomega", 1)
	assertRecoveryDBSnapshot(t, activePath, activeSnapshot)
	assertRecoveryDBSnapshot(t, strandedPath, strandedSnapshot)
}

func TestRecoveryDestinationStartsFreshAtCurrentSchema(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "legacy-source.db")
	createRecoveryDestinationSourceDB(t, sourcePath, []IndexedFile{
		recoveryDestinationIndexedFile("/sessions/source.jsonl", "source-hash", []IndexedMessage{{
			Ordinal: 0, Role: "user", Text: "fresh current schema sentinel", UUID: "11111111-1111-4111-8111-111111111111",
			Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
		}}),
	})
	plan := recoveryDestinationPlanFromDBs(t, ctx, sourcePath)

	destPath, err := CreateRecoveryDestination(ctx, dir, plan)
	if err != nil {
		t.Fatalf("CreateRecoveryDestination: %v", err)
	}
	defer func() { _ = recoveryDestinationRemoveFiles(destPath) }()
	if filepath.Dir(destPath) != dir || !strings.HasPrefix(filepath.Base(destPath), ".backscroll-recover-") {
		t.Fatalf("destination path %s is not a sibling recovery temp in %s", destPath, dir)
	}
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("stat destination: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("destination permissions = %v, want 0600", got)
	}

	db, err := OpenReadOnly(destPath)
	if err != nil {
		t.Fatalf("open destination readonly: %v", err)
	}
	defer func() { _ = db.Close() }()
	assertCurrentShape(t, db.DB())
	if recoveryDestinationColumnExists(t, db, "search_items", "source_metadata") {
		t.Fatal("destination has legacy source_metadata column; want fresh current schema")
	}
	var indexedFiles int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files`).Scan(&indexedFiles); err != nil {
		t.Fatalf("count indexed_files: %v", err)
	}
	if indexedFiles != 0 {
		t.Fatalf("indexed_files rows = %d, want 0 in fresh recovery destination", indexedFiles)
	}
}

func TestRecoveryDestinationIndependentVerificationRejectsTamper(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	createRecoveryDestinationSourceDB(t, sourcePath, []IndexedFile{
		recoveryDestinationIndexedFile("/sessions/source.jsonl", "source-hash", []IndexedMessage{{
			Ordinal: 0, Role: "assistant", Text: "tamper detector sentinel", UUID: "11111111-1111-4111-8111-111111111111",
			Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
		}}),
	})
	plan := recoveryDestinationPlanFromDBs(t, ctx, sourcePath)
	destPath, err := CreateRecoveryDestination(ctx, dir, plan)
	if err != nil {
		t.Fatalf("CreateRecoveryDestination: %v", err)
	}
	defer func() { _ = recoveryDestinationRemoveFiles(destPath) }()

	mutateRecoveryDatabase(t, destPath, `
		INSERT INTO indexed_files(path, hash)
		VALUES ('/tampered/invented-source.jsonl', 'invented-hash');
	`)
	if err := VerifyRecoveryDestination(ctx, destPath, plan); err == nil {
		t.Fatal("VerifyRecoveryDestination accepted a tampered destination with invented source accounting")
	}
}

func TestRecoveryDestinationVerificationRejectsEqualCountWrongFTSSurface(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	createRecoveryDestinationSourceDB(t, sourcePath, []IndexedFile{
		recoveryDestinationIndexedFile("/sessions/prose.jsonl", "prose-hash", []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "first message keeps representativealpha", UUID: "11111111-1111-4111-8111-111111111111", ContentType: "text", ExtractionVersion: CurrentExtractionVersion},
			{Ordinal: 1, Role: "assistant", Text: "second message must stay messagesurfacebeta", UUID: "22222222-2222-4222-8222-222222222222", ContentType: "text", ExtractionVersion: CurrentExtractionVersion},
		}),
		recoveryDestinationIndexedFile("/sessions/tool.jsonl", "tool-hash", []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "Bash command=toolonlysurfacegamma", UUID: "33333333-3333-4333-8333-333333333333", ContentType: "tool", ToolName: "Bash", CommandHead: "toolonlysurfacegamma", ExtractionVersion: CurrentExtractionVersion},
		}),
	})
	plan := recoveryDestinationPlanFromDBs(t, ctx, sourcePath)
	destPath, err := CreateRecoveryDestination(ctx, dir, plan)
	if err != nil {
		t.Fatalf("CreateRecoveryDestination: %v", err)
	}
	defer func() { _ = recoveryDestinationRemoveFiles(destPath) }()

	secondMessageID := recoveryDestinationRowID(t, destPath, "/sessions/prose.jsonl", 1)
	toolID := recoveryDestinationRowID(t, destPath, "/sessions/tool.jsonl", 0)
	mutateRecoveryDatabase(t, destPath, `
		INSERT INTO messages_fts(messages_fts, rowid, text)
		SELECT 'delete', id, text FROM search_items WHERE id = `+strconv.FormatInt(secondMessageID, 10)+`;
		INSERT INTO messages_fts(rowid, text)
		SELECT id, text FROM search_items WHERE id = `+strconv.FormatInt(toolID, 10)+`;
	`)
	if err := VerifyRecoveryDestination(ctx, destPath, plan); err == nil {
		t.Fatal("VerifyRecoveryDestination accepted equal-count tampering with a tool row on messages_fts")
	}
}

func TestRecoveryDestinationAcceptsPathOrdinalIdentityWithNilAndEmptyUUID(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	emptyUUID := ""
	plan := compat.RecoveryPlan{Records: []compat.CanonicalRecord{
		{Record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/nil-uuid.jsonl", Ordinal: 0, Role: "user", Text: "nil uuid ordinal zero messagealpha", UUID: nil, ContentType: "text"}},
		{Record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/empty-uuid-a.jsonl", Ordinal: 0, Role: "assistant", Text: "empty uuid ordinal zero toolbeta", UUID: &emptyUUID, ContentType: "tool"}},
		{Record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/empty-uuid-b.jsonl", Ordinal: 1, Role: "assistant", Text: "second empty uuid distinct path ordinal gammabeta", UUID: &emptyUUID, ContentType: "text"}},
	}}

	destPath, err := CreateRecoveryDestination(ctx, dir, plan)
	if err != nil {
		t.Fatalf("CreateRecoveryDestination with path/ordinal identities: %v", err)
	}
	defer func() { _ = recoveryDestinationRemoveFiles(destPath) }()
	if err := VerifyRecoveryDestination(ctx, destPath, plan); err != nil {
		t.Fatalf("VerifyRecoveryDestination with path/ordinal identities: %v", err)
	}
}

func TestRecoveryDestinationRejectsDuplicatePathOrdinalIdentityAndCleansUp(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	emptyUUID := ""
	plan := compat.RecoveryPlan{Records: []compat.CanonicalRecord{
		{Record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/duplicate-path.jsonl", Ordinal: 0, Role: "user", Text: "first duplicate path ordinal", UUID: nil, ContentType: "text"}},
		{Record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/duplicate-path.jsonl", Ordinal: 0, Role: "assistant", Text: "second duplicate path ordinal", UUID: &emptyUUID, ContentType: "text"}},
	}}

	if _, err := CreateRecoveryDestination(ctx, dir, plan); err == nil {
		t.Fatal("CreateRecoveryDestination accepted duplicate path/ordinal identities")
	}
	assertNoRecoveryDestinationTemps(t, dir)
}

func TestRecoverConflictOrUninterpretableRollsBackEverything(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "active.db")
	createRecoveryDestinationSourceDB(t, sourcePath, []IndexedFile{
		recoveryDestinationIndexedFile("/sessions/active.jsonl", "active-hash", []IndexedMessage{{
			Ordinal: 0, Role: "user", Text: "safe source remains untouched", UUID: "11111111-1111-4111-8111-111111111111",
			Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
		}}),
	})
	plan := recoveryDestinationPlanFromDBs(t, ctx, sourcePath)
	sourceSnapshot := snapshotRecoveryDB(t, sourcePath)
	conflictUUID := "22222222-2222-4222-8222-222222222222"
	plan.Records = append(plan.Records,
		compat.CanonicalRecord{Record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/conflict-a.jsonl", Ordinal: 0, Role: "user", Text: "first conflicting payload", UUID: &conflictUUID, ContentType: "text"}},
		compat.CanonicalRecord{Record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/conflict-b.jsonl", Ordinal: 1, Role: "assistant", Text: "second conflicting payload", UUID: &conflictUUID, ContentType: "text"}},
	)

	_, err := CreateRecoveryDestination(ctx, dir, plan)
	if err == nil {
		t.Fatal("CreateRecoveryDestination succeeded for a plan with conflicting import identities")
	}
	assertNoRecoveryDestinationTemps(t, dir)
	assertRecoveryDBSnapshot(t, sourcePath, sourceSnapshot)

	diagnosticBearingPlan := plan
	diagnosticBearingPlan.Records = nil
	_, err = CreateRecoveryDestination(ctx, dir, diagnosticBearingPlan)
	if err == nil {
		t.Fatal("CreateRecoveryDestination succeeded for an uninterpretable/diagnostic-bearing plan")
	}
	assertNoRecoveryDestinationTemps(t, dir)
	assertRecoveryDBSnapshot(t, sourcePath, sourceSnapshot)
}

func createRecoveryDestinationSourceDB(t *testing.T, path string, files []IndexedFile) {
	t.Helper()
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open source database %s: %v", path, err)
	}
	if err := db.SyncFiles(files); err != nil {
		_ = db.Close()
		t.Fatalf("sync source database %s: %v", path, err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close source database %s: %v", path, err)
	}
}

func recoveryDestinationIndexedFile(path, hash string, messages []IndexedMessage) IndexedFile {
	return IndexedFile{SourcePath: path, Source: "session", Hash: hash, Project: "project", Messages: messages}
}

func recoveryDestinationPlanFromDBs(t *testing.T, ctx context.Context, paths ...string) compat.RecoveryPlan {
	t.Helper()
	inputs := make([]compat.RecoveryInput, 0, len(paths))
	for _, path := range paths {
		db, err := OpenReadOnly(path)
		if err != nil {
			t.Fatalf("open readonly %s: %v", path, err)
		}
		input, diag, err := ReadRecoveryInput(ctx, db)
		closeErr := db.Close()
		if err != nil || diag != nil {
			t.Fatalf("ReadRecoveryInput %s err=%v diag=%+v", path, err, diag)
		}
		if closeErr != nil {
			t.Fatalf("close readonly %s: %v", path, closeErr)
		}
		inputs = append(inputs, input)
	}
	plan, diagnostics, err := compat.PlanRecovery(inputs)
	if err != nil {
		t.Fatalf("PlanRecovery: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("PlanRecovery diagnostics: %+v", diagnostics)
	}
	return plan
}

func assertRecoveryDestinationRecords(t *testing.T, path string, plan compat.RecoveryPlan) {
	t.Helper()
	db, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open destination readonly: %v", err)
	}
	defer func() { _ = db.Close() }()
	input, diag, err := ReadRecoveryInput(context.Background(), db)
	if err != nil || diag != nil {
		t.Fatalf("ReadRecoveryInput destination err=%v diag=%+v", err, diag)
	}
	got := append([]models.IndexedRecord(nil), input.Records...)
	want := make([]models.IndexedRecord, 0, len(plan.Records))
	for _, planned := range plan.Records {
		want = append(want, planned.Record)
	}
	sortRecoveryDestinationRecords(got)
	sortRecoveryDestinationRecords(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("destination records mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
	var indexedFiles int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files`).Scan(&indexedFiles); err != nil {
		t.Fatalf("count indexed_files: %v", err)
	}
	if indexedFiles != 0 {
		t.Fatalf("indexed_files rows = %d, want 0; recovery must not invent source hashes", indexedFiles)
	}
}

func assertRecoveryDestinationFTS(t *testing.T, path, msgTerm string, wantMsg int, toolTerm string, wantTool int) {
	t.Helper()
	db, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open destination readonly for FTS: %v", err)
	}
	defer func() { _ = db.Close() }()
	var msgHits, toolHits int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH ?`, msgTerm).Scan(&msgHits); err != nil {
		t.Fatalf("query messages_fts: %v", err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM tool_fts WHERE tool_fts MATCH ?`, toolTerm).Scan(&toolHits); err != nil {
		t.Fatalf("query tool_fts: %v", err)
	}
	if msgHits != wantMsg || toolHits != wantTool {
		t.Fatalf("FTS hits messages=%d tool=%d, want messages=%d tool=%d", msgHits, toolHits, wantMsg, wantTool)
	}
}

func recoveryDestinationColumnExists(t *testing.T, db *Database, table, column string) bool {
	t.Helper()
	rows, err := db.DB().Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("table_info %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table_info %s: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read table_info %s: %v", table, err)
	}
	return false
}

func assertNoRecoveryDestinationTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".backscroll-recover-*.db*"))
	if err != nil {
		t.Fatal(err)
	}
	kept := matches[:0]
	for _, match := range matches {
		base := filepath.Base(match)
		if strings.HasPrefix(base, ".backscroll-recover-") {
			kept = append(kept, match)
		}
	}
	if len(kept) != 0 {
		t.Fatalf("recovery destination temp files remain: %v", kept)
	}
}

func recoveryDestinationRowID(t *testing.T, path, sourcePath string, ordinal int64) int64 {
	t.Helper()
	db, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open destination readonly for row id: %v", err)
	}
	defer func() { _ = db.Close() }()
	var id int64
	if err := db.DB().QueryRow(`SELECT id FROM search_items WHERE source_path = ? AND ordinal = ?`, sourcePath, ordinal).Scan(&id); err != nil {
		t.Fatalf("lookup row id for %s ordinal %d: %v", sourcePath, ordinal, err)
	}
	return id
}

func sortRecoveryDestinationRecords(records []models.IndexedRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].SourcePath != records[j].SourcePath {
			return records[i].SourcePath < records[j].SourcePath
		}
		if records[i].Ordinal != records[j].Ordinal {
			return records[i].Ordinal < records[j].Ordinal
		}
		if pointerTestString(records[i].UUID) != pointerTestString(records[j].UUID) {
			return pointerTestString(records[i].UUID) < pointerTestString(records[j].UUID)
		}
		return records[i].Text < records[j].Text
	})
}

func pointerTestString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
