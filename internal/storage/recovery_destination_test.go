package storage

import (
	"context"
	"database/sql"
	"errors"
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

func TestPublishedGoLineagesUpgradeLosslessly(t *testing.T) {
	ctx := context.Background()
	catalog, err := compat.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	seenFixtures := map[string][]string{}
	for _, release := range catalog.Releases {
		seenFixtures[release.Fixture] = append(seenFixtures[release.Fixture], release.Tag)
	}
	if len(seenFixtures) == 0 {
		t.Fatal("catalog has no published Go releases")
	}

	fixtures := make([]string, 0, len(seenFixtures))
	for fixture := range seenFixtures {
		fixtures = append(fixtures, fixture)
	}
	sort.Strings(fixtures)

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			dbPath := createFixtureDatabase(t, fixture)
			want := seedPublishedReleaseRecoverySentinels(t, dbPath, fixture)

			db, diag, err := OpenCompatible(ctx, dbPath)
			if err != nil || diag != nil {
				t.Fatalf("OpenCompatible for published releases %v fixture %s err=%v diagnostic=%+v", seenFixtures[fixture], fixture, err, diag)
			}
			defer func() { _ = db.Close() }()

			assertCurrentShape(t, db.DB())
			assertPublishedReleaseSentinels(t, db.DB(), want)
			assertFTSQueryable(t, db.DB(), "publishedlineagealpha", 1)
			assertToolFTSQueryable(t, db.DB(), "publishedlineagecmd", 1)

			input, diag, err := ReadRecoveryInput(ctx, db)
			if err != nil || diag != nil {
				t.Fatalf("ReadRecoveryInput after published-release migration err=%v diagnostic=%+v", err, diag)
			}
			assertPublishedReleaseRecoveryInput(t, input, want)
			plan, diagnostics, err := compat.PlanRecovery([]compat.RecoveryInput{input})
			if err != nil || len(diagnostics) != 0 {
				t.Fatalf("PlanRecovery after published-release migration err=%v diagnostics=%+v", err, diagnostics)
			}
			if len(plan.Records) != len(want.Records) {
				t.Fatalf("recovery plan records = %d, want %d", len(plan.Records), len(want.Records))
			}

			destPath, err := CreateRecoveryDestination(ctx, filepath.Dir(dbPath), plan)
			if err != nil {
				t.Fatalf("CreateRecoveryDestination from migrated published release: %v", err)
			}
			defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()
			if err := VerifyRecoveryDestination(ctx, destPath, plan); err != nil {
				t.Fatalf("VerifyRecoveryDestination from migrated published release: %v", err)
			}
			assertRecoveryDestinationRecords(t, destPath, plan)
			assertRecoveryDestinationFTS(t, destPath, "publishedlineagealpha", 1, "publishedlineagecmd", 1)
		})
	}
}

type publishedReleaseSentinels struct {
	Records     []models.IndexedRecord
	IndexedPath string
	IndexedHash string
}

func seedPublishedReleaseRecoverySentinels(t *testing.T, dbPath, fixture string) publishedReleaseSentinels {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	records := []models.IndexedRecord{
		{
			Source:      "session",
			SourcePath:  "/published/" + fixture + "/text.jsonl",
			Ordinal:     0,
			Role:        "user",
			Text:        "publishedlineagealpha text survives migration",
			Project:     stringPtr("published-project"),
			UUID:        stringPtr("11111111-1111-4111-8111-111111111111"),
			Timestamp:   stringPtr("2026-08-18T00:00:00Z"),
			ContentType: "text",
		},
		{
			Source:      "session",
			SourcePath:  "/published/" + fixture + "/tool.jsonl",
			Ordinal:     1,
			Role:        "assistant",
			Text:        "publishedlineagecmd tool survives migration",
			Project:     stringPtr("published-project"),
			UUID:        stringPtr("22222222-2222-4222-8222-222222222222"),
			Timestamp:   stringPtr("2026-08-18T00:00:01Z"),
			ContentType: "tool",
		},
	}
	for _, record := range records {
		if _, err := db.Exec(`INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, record.Source, record.SourcePath, record.Ordinal, record.Role, record.Text, pointerTestString(record.Timestamp), pointerTestString(record.UUID), pointerTestString(record.Project), record.ContentType); err != nil {
			t.Fatalf("insert published release sentinel into %s: %v", fixture, err)
		}
	}

	indexedPath := "/published/" + fixture + "/indexed.jsonl"
	indexedHash := "published-hash-" + fixture
	if _, err := db.Exec(`INSERT INTO indexed_files (path, hash, last_indexed) VALUES (?, ?, ?)`, indexedPath, indexedHash, "2026-08-18T00:00:02Z"); err != nil {
		t.Fatalf("insert published release indexed_files sentinel into %s: %v", fixture, err)
	}
	return publishedReleaseSentinels{Records: records, IndexedPath: indexedPath, IndexedHash: indexedHash}
}

func assertPublishedReleaseSentinels(t *testing.T, db *sql.DB, want publishedReleaseSentinels) {
	t.Helper()
	for _, wantRecord := range want.Records {
		var got models.IndexedRecord
		var project, uuid, timestamp string
		if err := db.QueryRow(`SELECT source, source_path, ordinal, role, text, project, uuid, timestamp, content_type
			FROM search_items WHERE uuid = ?`, pointerTestString(wantRecord.UUID)).Scan(&got.Source, &got.SourcePath, &got.Ordinal, &got.Role, &got.Text, &project, &uuid, &timestamp, &got.ContentType); err != nil {
			t.Fatalf("query migrated published release sentinel %s: %v", pointerTestString(wantRecord.UUID), err)
		}
		got.Project = &project
		got.UUID = &uuid
		got.Timestamp = &timestamp
		if !reflect.DeepEqual(got, wantRecord) {
			t.Fatalf("migrated published release sentinel mismatch\ngot:  %+v\nwant: %+v", got, wantRecord)
		}
	}
	var gotHash string
	if err := db.QueryRow(`SELECT hash FROM indexed_files WHERE path = ?`, want.IndexedPath).Scan(&gotHash); err != nil {
		t.Fatalf("query migrated published release indexed_files sentinel: %v", err)
	}
	if gotHash != want.IndexedHash {
		t.Fatalf("indexed_files hash = %q, want %q", gotHash, want.IndexedHash)
	}
}

func assertPublishedReleaseRecoveryInput(t *testing.T, input compat.RecoveryInput, want publishedReleaseSentinels) {
	t.Helper()
	if input.RowCount != len(want.Records) || len(input.Records) != len(want.Records) {
		t.Fatalf("recovery input row count = %d records = %d, want %d", input.RowCount, len(input.Records), len(want.Records))
	}
	got := append([]models.IndexedRecord(nil), input.Records...)
	wantRecords := append([]models.IndexedRecord(nil), want.Records...)
	sortRecoveryDestinationRecords(got)
	sortRecoveryDestinationRecords(wantRecords)
	if !reflect.DeepEqual(got, wantRecords) {
		t.Fatalf("recovery input records mismatch\ngot:  %+v\nwant: %+v", got, wantRecords)
	}
}

func stringPtr(value string) *string {
	return &value
}

func removeRecoveryDestinationTestFiles(path string) error {
	var errs []error
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func TestRecoveryDestinationErrorPublicBehavior(t *testing.T) {
	cause := errors.New("primary cause")
	cleanup := errors.New("cleanup cause")
	withPath := &RecoveryDestinationError{Path: "candidate.db", Cause: cause, CleanupErr: cleanup}
	if got := withPath.Error(); !strings.Contains(got, "candidate.db") || !strings.Contains(got, "primary cause") {
		t.Fatalf("pathful error text = %q", got)
	}
	if got := withPath.Unwrap(); len(got) != 2 || got[0] != cause || got[1] != cleanup {
		t.Fatalf("unwrap pathful = %#v, want cause and cleanup", got)
	}

	withoutPath := &RecoveryDestinationError{Cause: cause}
	if got := withoutPath.Error(); strings.Contains(got, "candidate.db") || !strings.Contains(got, "primary cause") {
		t.Fatalf("pathless error text = %q", got)
	}
	if got := withoutPath.Unwrap(); len(got) != 1 || got[0] != cause {
		t.Fatalf("unwrap pathless = %#v, want only cause", got)
	}
}

func TestRecoveryDestinationPublicValidationFailures(t *testing.T) {
	ctx := context.Background()
	if _, err := CreateRecoveryDestination(ctx, t.TempDir(), compat.RecoveryPlan{}); err == nil || !strings.Contains(err.Error(), "inapplicable") {
		t.Fatalf("CreateRecoveryDestination nil plan error = %v, want inapplicable plan", err)
	}
	if err := VerifyRecoveryDestination(ctx, filepath.Join(t.TempDir(), "missing.db"), compat.RecoveryPlan{}); err == nil || !strings.Contains(err.Error(), "inapplicable") {
		t.Fatalf("VerifyRecoveryDestination nil plan error = %v, want inapplicable plan", err)
	}

	validEmptyPlan := compat.RecoveryPlan{Records: []compat.CanonicalRecord{}}
	if _, err := CreateRecoveryDestination(ctx, filepath.Join(t.TempDir(), "missing"), validEmptyPlan); err == nil || !strings.Contains(err.Error(), "create recovery destination temp") {
		t.Fatalf("CreateRecoveryDestination missing dir error = %v, want create temp failure", err)
	}
	if err := VerifyRecoveryDestination(ctx, filepath.Join(t.TempDir(), "missing.db"), validEmptyPlan); err == nil || !strings.Contains(err.Error(), "open recovery destination immutable read-only") {
		t.Fatalf("VerifyRecoveryDestination missing file error = %v, want open immutable failure", err)
	}
}

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
	defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()
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
	defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()
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
	assertRecoveryDestinationAccounting(t, db.DB(), plan)
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
	defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()

	mutateRecoveryDatabase(t, destPath, `
		INSERT INTO indexed_files(path, hash)
		VALUES ('/tampered/invented-source.jsonl', 'invented-hash');
	`)
	if err := VerifyRecoveryDestination(ctx, destPath, plan); err == nil {
		t.Fatal("VerifyRecoveryDestination accepted a tampered destination with invented source accounting")
	}
}

func TestRecoveryDestinationVerificationRejectsInvalidRecoveredAccounting(t *testing.T) {
	cases := []struct {
		name   string
		mutate string
	}{
		{
			name:   "missing path",
			mutate: `DELETE FROM indexed_files WHERE path = '/sessions/source.jsonl';`,
		},
		{
			name:   "real-looking but unverified hash",
			mutate: `UPDATE indexed_files SET hash = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE path = '/sessions/source.jsonl';`,
		},
		{
			name:   "non-null last indexed",
			mutate: `UPDATE indexed_files SET last_indexed = '2026-08-19T00:00:00Z' WHERE path = '/sessions/source.jsonl';`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			sourcePath := filepath.Join(dir, "source.db")
			createRecoveryDestinationSourceDB(t, sourcePath, []IndexedFile{
				recoveryDestinationIndexedFile("/sessions/source.jsonl", "source-hash", []IndexedMessage{{
					Ordinal: 0, Role: "assistant", Text: "accounting tamper sentinel", UUID: "11111111-1111-4111-8111-111111111111",
					Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
				}}),
			})
			plan := recoveryDestinationPlanFromDBs(t, ctx, sourcePath)
			destPath, err := CreateRecoveryDestination(ctx, dir, plan)
			if err != nil {
				t.Fatalf("CreateRecoveryDestination: %v", err)
			}

			mutateRecoveryDatabase(t, destPath, tc.mutate)
			if err := VerifyRecoveryDestination(ctx, destPath, plan); err == nil {
				t.Fatalf("VerifyRecoveryDestination accepted %s recovered source accounting tampering", tc.name)
			}
		})
	}
}

func TestRecoveryDestinationVerificationRejectsOrphanedDerivedRows(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	createRecoveryDestinationSourceDB(t, sourcePath, []IndexedFile{
		recoveryDestinationIndexedFile("/sessions/source.jsonl", "source-hash", []IndexedMessage{{
			Ordinal: 0, Role: "user", Text: "foreign key tamper sentinel", UUID: "11111111-1111-4111-8111-111111111111",
			Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
		}}),
	})
	plan := recoveryDestinationPlanFromDBs(t, ctx, sourcePath)
	destPath, err := CreateRecoveryDestination(ctx, dir, plan)
	if err != nil {
		t.Fatalf("CreateRecoveryDestination: %v", err)
	}
	defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()

	mutateRecoveryDatabase(t, destPath, `
		INSERT INTO template_matches(template_id, source_path, ordinal)
		VALUES (999, '/sessions/source.jsonl', 0);
	`)
	if err := VerifyRecoveryDestination(ctx, destPath, plan); err == nil || !strings.Contains(err.Error(), "foreign_key_check") {
		t.Fatalf("VerifyRecoveryDestination orphaned derived row error = %v, want foreign key diagnostics", err)
	}
}

func TestRecoveryDestinationVerificationRejectsDeletedRows(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	createRecoveryDestinationSourceDB(t, sourcePath, []IndexedFile{
		recoveryDestinationIndexedFile("/sessions/source.jsonl", "source-hash", []IndexedMessage{{
			Ordinal: 0, Role: "user", Text: "row deletion tamper sentinel", UUID: "11111111-1111-4111-8111-111111111111",
			Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
		}}),
	})
	plan := recoveryDestinationPlanFromDBs(t, ctx, sourcePath)
	destPath, err := CreateRecoveryDestination(ctx, dir, plan)
	if err != nil {
		t.Fatalf("CreateRecoveryDestination: %v", err)
	}
	defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()

	mutateRecoveryDatabase(t, destPath, `DELETE FROM search_items;`)
	if err := VerifyRecoveryDestination(ctx, destPath, plan); err == nil {
		t.Fatal("VerifyRecoveryDestination accepted a destination with deleted recovery rows")
	}
}

func TestRecoveryDestinationVerificationRejectsEqualCountWrongIdentityAndPayload(t *testing.T) {
	cases := []struct {
		name   string
		mutate string
	}{
		{
			name: "identity",
			mutate: `UPDATE search_items
				SET uuid = '99999999-9999-4999-8999-999999999999'
				WHERE source_path = '/sessions/source.jsonl' AND ordinal = 0;`,
		},
		{
			name: "payload",
			mutate: `UPDATE search_items
				SET role = 'assistant'
				WHERE source_path = '/sessions/source.jsonl' AND ordinal = 0;`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			sourcePath := filepath.Join(dir, "source.db")
			createRecoveryDestinationSourceDB(t, sourcePath, []IndexedFile{
				recoveryDestinationIndexedFile("/sessions/source.jsonl", "source-hash", []IndexedMessage{{
					Ordinal: 0, Role: "user", Text: "equal count tamper sentinel", UUID: "11111111-1111-4111-8111-111111111111",
					Timestamp: "2026-08-18T00:00:00Z", ContentType: "text", ExtractionVersion: CurrentExtractionVersion,
				}}),
			})
			plan := recoveryDestinationPlanFromDBs(t, ctx, sourcePath)
			destPath, err := CreateRecoveryDestination(ctx, dir, plan)
			if err != nil {
				t.Fatalf("CreateRecoveryDestination: %v", err)
			}
			defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()

			mutateRecoveryDatabase(t, destPath, tc.mutate)
			if err := VerifyRecoveryDestination(ctx, destPath, plan); err == nil {
				t.Fatalf("VerifyRecoveryDestination accepted equal-count %s tampering", tc.name)
			}
		})
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
	defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()

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
	defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()
	if err := VerifyRecoveryDestination(ctx, destPath, plan); err != nil {
		t.Fatalf("VerifyRecoveryDestination with path/ordinal identities: %v", err)
	}
}

func TestRecoveryDestinationAcceptsUntokenizedRecordsPersistently(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	plan := compat.RecoveryPlan{Records: []compat.CanonicalRecord{
		{Record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/punctuation.jsonl", Ordinal: 0, Role: "user", Text: "!!", ContentType: "text"}},
		{Record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/tool-punctuation.jsonl", Ordinal: 1, Role: "assistant", Text: "??", ContentType: "tool"}},
	}}

	destPath, err := CreateRecoveryDestination(ctx, dir, plan)
	if err != nil {
		t.Fatalf("CreateRecoveryDestination with untokenized records: %v", err)
	}
	defer func() { _ = removeRecoveryDestinationTestFiles(destPath) }()
	if err := VerifyRecoveryDestination(ctx, destPath, plan); err != nil {
		t.Fatalf("VerifyRecoveryDestination with untokenized records: %v", err)
	}
	assertRecoveryDestinationRecords(t, destPath, plan)
	assertRecoveryDestinationFTS(t, destPath, "punctuationabsent", 0, "toolabsent", 0)
}

func TestRecoveryDestinationRejectsUnsafePlanIdentitiesAndCleansUp(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		record models.IndexedRecord
	}{
		{
			name:   "invalid_uuid",
			record: models.IndexedRecord{Source: "session", SourcePath: "/sessions/invalid-uuid.jsonl", Ordinal: 0, Role: "user", Text: "invalid uuid record", UUID: stringPtr("not-a-uuid"), ContentType: "text"},
		},
		{
			name:   "missing_fallback_identity",
			record: models.IndexedRecord{Source: "session", SourcePath: "", Ordinal: -1, Role: "assistant", Text: "missing fallback identity", ContentType: "text"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			plan := compat.RecoveryPlan{Records: []compat.CanonicalRecord{{Record: tc.record}}}
			if _, err := CreateRecoveryDestination(ctx, dir, plan); err == nil {
				t.Fatalf("CreateRecoveryDestination accepted unsafe plan identity %+v", tc.record)
			}
			assertNoRecoveryDestinationTemps(t, dir)
		})
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
	assertRecoveryDestinationAccounting(t, db.DB(), plan)
}

func assertRecoveryDestinationAccounting(t *testing.T, db *sql.DB, plan compat.RecoveryPlan) {
	t.Helper()
	rows, err := db.Query(`
		SELECT path, hash, last_indexed
		FROM indexed_files
		ORDER BY path
	`)
	if err != nil {
		t.Fatalf("query recovered source accounting: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var gotPaths []string
	for rows.Next() {
		var path, hash string
		var lastIndexed sql.NullString
		if err := rows.Scan(&path, &hash, &lastIndexed); err != nil {
			t.Fatalf("scan recovered source accounting: %v", err)
		}
		if hash != recoveredSourceHash || lastIndexed.Valid {
			t.Fatalf("accounting for %s = hash %q last_indexed %+v", path, hash, lastIndexed)
		}
		gotPaths = append(gotPaths, path)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read recovered source accounting: %v", err)
	}

	seen := make(map[string]struct{}, len(plan.Records))
	for _, planned := range plan.Records {
		seen[planned.Record.SourcePath] = struct{}{}
	}
	wantPaths := make([]string, 0, len(seen))
	for path := range seen {
		wantPaths = append(wantPaths, path)
	}
	sort.Strings(wantPaths)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("recovered source accounting paths = %#v, want %#v", gotPaths, wantPaths)
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
