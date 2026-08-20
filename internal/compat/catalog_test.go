package compat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCheckedInReleaseSchemaManifestIsComplete(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.FirstGoRelease != "v0.3.7" || catalog.LatestGoRelease != "v3.2.5" {
		t.Fatalf("catalog bounds = %s..%s", catalog.FirstGoRelease, catalog.LatestGoRelease)
	}
	seen := map[string]bool{}
	for _, release := range catalog.Releases {
		if release.Tag == "" || release.Fixture == "" || release.ProvenanceSHA256 == "" || seen[release.Tag] {
			t.Fatalf("invalid release mapping: %+v", release)
		}
		seen[release.Tag] = true
		if _, err := fs.Stat(releaseSchemaFS, "testdata/release-schemas/"+release.Fixture); err != nil {
			t.Fatalf("fixture %q: %v", release.Fixture, err)
		}
	}
	if !seen["v0.3.7"] || !seen["v3.2.5"] {
		t.Fatalf("release endpoints missing: %v", seen)
	}
}

func TestReleaseSchemaManifestRejectsMissingFixture(t *testing.T) {
	_, err := loadCatalogFromFS(fstest.MapFS{
		"testdata/release-schemas/manifest.json": {Data: []byte(`{
			"FirstGoRelease": "v0.3.7",
			"LatestGoRelease": "v3.2.5",
			"Releases": [{"Tag": "v0.3.7", "Fixture": "missing.sql", "ProvenanceSHA256": "abc123"}]
		}`)},
	})
	if err == nil || !strings.Contains(err.Error(), "missing.sql") {
		t.Fatalf("missing fixture error = %v", err)
	}
}

func TestReleaseSchemaManifestRejectsLatestBeforeV3_2_5(t *testing.T) {
	_, err := loadCatalogFromFS(fstest.MapFS{
		"testdata/release-schemas/manifest.json": {Data: []byte(`{
			"FirstGoRelease": "v0.3.7",
			"LatestGoRelease": "v3.2.4",
			"Releases": [{"Tag": "v0.3.7", "Fixture": "v1.sql", "ProvenanceSHA256": "abc123"}]
		}`)},
		"testdata/release-schemas/v1.sql": {Data: []byte("-- fixture\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "v3.2.5") {
		t.Fatalf("latest bound error = %v", err)
	}
}

func TestLoadCatalogUsesCheckedInSignaturesWithoutExecutingFixtureSQL(t *testing.T) {
	poisonSQL := []byte("BEGIN TRANSACTION; this is intentionally not executable fixture SQL; COMMIT;")
	poisonSHA := fmt.Sprintf("%x", sha256.Sum256(poisonSQL))

	withReleaseSchemaFS(t, fstest.MapFS{
		"testdata/release-schemas/manifest.json": {Data: []byte(fmt.Sprintf(`{
			"FirstGoRelease": "v0.3.7",
			"LatestGoRelease": "v3.2.5",
			"Releases": [
				{"Tag": "v0.3.7", "Fixture": "poison.sql", "ProvenanceSHA256": %q, "Signature": "sha256:poisoned", "AppliedVersion": 13},
				{"Tag": "v3.2.5", "Fixture": "poison.sql", "ProvenanceSHA256": %q, "Signature": "sha256:poisoned", "AppliedVersion": 13}
			]
		}`, poisonSHA, poisonSHA))},
		"testdata/release-schemas/poison.sql": {Data: poisonSQL},
	})

	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("load catalog executed fixture SQL or rejected checked-in signature data: %v", err)
	}
	if got := catalog.CurrentSignature(); got != "sha256:poisoned" {
		t.Fatalf("current signature = %q, want checked-in signature", got)
	}
}

func TestCurrentSignatureFollowsLatestReleaseMapping(t *testing.T) {
	fixtureSQL := []byte("CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_on TEXT NOT NULL, checksum TEXT NOT NULL);")
	fixtureSHA := fmt.Sprintf("%x", sha256.Sum256(fixtureSQL))

	withReleaseSchemaFS(t, fstest.MapFS{
		"testdata/release-schemas/manifest.json": {Data: []byte(fmt.Sprintf(`{
			"FirstGoRelease": "v0.3.7",
			"LatestGoRelease": "v3.2.6",
			"Releases": [
				{"Tag": "v0.3.7", "Fixture": "v13.sql", "ProvenanceSHA256": %q, "Signature": "sha256:old-latest", "AppliedVersion": 13},
				{"Tag": "v3.2.5", "Fixture": "v13.sql", "ProvenanceSHA256": %q, "Signature": "sha256:old-latest", "AppliedVersion": 13},
				{"Tag": "v3.2.6", "Fixture": "v14.sql", "ProvenanceSHA256": %q, "Signature": "sha256:new-latest", "AppliedVersion": 14}
			]
		}`, fixtureSHA, fixtureSHA, fixtureSHA))},
		"testdata/release-schemas/v13.sql": {Data: fixtureSQL},
		"testdata/release-schemas/v14.sql": {Data: fixtureSQL},
	})

	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.CurrentSignature(); got != "sha256:new-latest" {
		t.Fatalf("current signature = %q, want latest release mapping signature", got)
	}
}

func TestReleaseSchemaFixtureSignaturesMatchCheckedInSQL(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	checked := map[string]bool{}
	for _, entry := range catalogShapeFixtures(t, catalog) {
		t.Run(entry.fixture, func(t *testing.T) {
			if checked[entry.fixture] {
				return
			}
			checked[entry.fixture] = true
			db := openFixtureCopy(t, entry.fixture)
			defer db.Close()

			shape, err := inspectShape(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			if shape.Signature != entry.signature {
				t.Fatalf("signature = %s, want checked-in %s", shape.Signature, entry.signature)
			}
		})
	}
}

func TestReleaseSchemaManifestRejectsFixtureHashDrift(t *testing.T) {
	original, err := fs.ReadFile(releaseSchemaFS, "testdata/release-schemas/v1.sql")
	if err != nil {
		t.Fatal(err)
	}
	provenance := fmt.Sprintf("%x", sha256.Sum256(original))
	modified := append([]byte(nil), original...)
	modified = append(modified, []byte("\n-- modified fixture bytes\n")...)

	_, err = loadCatalogFromFS(fstest.MapFS{
		"testdata/release-schemas/manifest.json": {Data: []byte(fmt.Sprintf(`{
			"FirstGoRelease": "v0.3.7",
			"LatestGoRelease": "v3.2.5",
			"Releases": [
				{"Tag": "v0.3.7", "Fixture": "v1.sql", "ProvenanceSHA256": %q, "Signature": "sha256:fixture", "AppliedVersion": 1, "HasSourceMetadata": true},
				{"Tag": "v3.2.5", "Fixture": "v1.sql", "ProvenanceSHA256": %q, "Signature": "sha256:fixture", "AppliedVersion": 1, "HasSourceMetadata": true}
			]
		}`, provenance, provenance))},
		"testdata/release-schemas/v1.sql": {Data: modified},
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("fixture hash drift error = %v", err)
	}
}

func TestFixtureMigrationRowsMatchPublishedCurrentLedger(t *testing.T) {
	authoritative := loadPublishedCurrentMigrationRows(t)

	fixturePaths, err := fs.Glob(releaseSchemaFS, "testdata/release-schemas/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(fixturePaths)
	if len(fixturePaths) == 0 {
		t.Fatal("no SQL fixtures found")
	}

	for _, fixturePath := range fixturePaths {
		t.Run(filepath.Base(fixturePath), func(t *testing.T) {
			fixtureSQL, err := fs.ReadFile(releaseSchemaFS, fixturePath)
			if err != nil {
				t.Fatal(err)
			}
			fixtureRows := loadFixtureMigrationRows(t, fixtureSQL)
			if len(fixtureRows) == 0 {
				t.Fatal("fixture has no schema_migrations rows")
			}
			for _, row := range fixtureRows {
				expected, ok := authoritative[row.version]
				if !ok {
					t.Fatalf("fixture has unknown migration version %d", row.version)
				}
				if filepath.Base(fixturePath) == "v13-development-alter-built.sql" && row.version == 9 {
					const historicalV9Checksum = "a84857185adb25a812c78f05b1ba4ee4d28e52b52b28812e077c03e7ae539917"
					if row.name != expected.name || row.checksum != historicalV9Checksum {
						t.Fatalf("observed development migration 9 row = (%q, %q), want (%q, %q)", row.name, row.checksum, expected.name, historicalV9Checksum)
					}
					continue
				}
				if row.name != expected.name || row.checksum != expected.checksum {
					t.Fatalf("migration %d row = (%q, %q), want (%q, %q)", row.version, row.name, row.checksum, expected.name, expected.checksum)
				}
			}
		})
	}
}

func TestObservedDevelopmentV13IsStructurallyEquivalentToLegacyV13(t *testing.T) {
	development := openFixtureCopy(t, "v13-development-alter-built.sql")
	defer development.Close()
	legacy := openFixtureCopy(t, "v13-legacy-alter-built.sql")
	defer legacy.Close()

	developmentShape, err := inspectShape(context.Background(), development)
	if err != nil {
		t.Fatal(err)
	}
	legacyShape, err := inspectShape(context.Background(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(developmentShape.columnsByTable, legacyShape.columnsByTable) {
		t.Fatal("development V13 columns differ from supported legacy V13")
	}

	developmentObjects, err := loadSQLiteObjects(context.Background(), development)
	if err != nil {
		t.Fatal(err)
	}
	legacyObjects, err := loadSQLiteObjects(context.Background(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if len(developmentObjects) != len(legacyObjects) {
		t.Fatalf("SQLite object counts differ: development=%d legacy=%d", len(developmentObjects), len(legacyObjects))
	}
	for i := range developmentObjects {
		got, want := developmentObjects[i], legacyObjects[i]
		if got.typ != want.typ || got.name != want.name || got.table != want.table {
			t.Fatalf("SQLite object %d identity differs: development=%+v legacy=%+v", i, got, want)
		}
		if got.name != "search_items" && got.sql != want.sql {
			t.Fatalf("SQLite object %s SQL differs outside documented search_items DDL", got.name)
		}
	}

	developmentSQL, err := fs.ReadFile(releaseSchemaFS, "testdata/release-schemas/v13-development-alter-built.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacySQL, err := fs.ReadFile(releaseSchemaFS, "testdata/release-schemas/v13-legacy-alter-built.sql")
	if err != nil {
		t.Fatal(err)
	}
	developmentRows := loadFixtureMigrationRows(t, developmentSQL)
	legacyRows := loadFixtureMigrationRows(t, legacySQL)
	if len(developmentRows) != len(legacyRows) {
		t.Fatalf("migration row counts differ: development=%d legacy=%d", len(developmentRows), len(legacyRows))
	}
	for i := range developmentRows {
		got, want := developmentRows[i], legacyRows[i]
		if got.version != want.version || got.name != want.name {
			t.Fatalf("migration %d identity differs: development=%+v legacy=%+v", i, got, want)
		}
		if got.version != 9 && got.checksum != want.checksum {
			t.Fatalf("migration %d checksum differs outside documented V9 lineage", got.version)
		}
	}
}

func TestUnmanifestedFixtureShapesAreDocumentedLocally(t *testing.T) {
	for _, fixture := range []string{"v2.sql", "v3-no-source-metadata.sql", "v5-without-source-metadata.sql", "v6.sql"} {
		t.Run(fixture, func(t *testing.T) {
			data, err := fs.ReadFile(releaseSchemaFS, "testdata/release-schemas/"+fixture)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if !strings.Contains(text, "No manifest release tag maps to this fixture.") || !strings.Contains(text, "compatibility triangulation") {
				t.Fatalf("fixture lacks local documentation for its unmanifested shape")
			}
		})
	}
}

type migrationRow struct {
	version  int
	name     string
	checksum string
}

func catalogShapeFixtures(t *testing.T, catalog Catalog) []struct{ fixture, signature string } {
	t.Helper()
	var result []struct{ fixture, signature string }
	catalogValue := reflect.ValueOf(catalog)
	for _, fieldName := range []string{"Releases", "UnmanifestedFixtures"} {
		field := catalogValue.FieldByName(fieldName)
		if !field.IsValid() {
			continue
		}
		for i := 0; i < field.Len(); i++ {
			entry := field.Index(i)
			fixture := entry.FieldByName("Fixture")
			signature := entry.FieldByName("Signature")
			if !fixture.IsValid() || !signature.IsValid() || fixture.String() == "" || signature.String() == "" {
				t.Fatalf("catalog %s[%d] lacks fixture signature data", fieldName, i)
			}
			result = append(result, struct{ fixture, signature string }{fixture: fixture.String(), signature: signature.String()})
		}
	}
	if len(result) == 0 {
		t.Fatal("catalog has no fixture signature data")
	}
	return result
}

func withReleaseSchemaFS(t *testing.T, fsys fs.FS) {
	t.Helper()
	original := releaseSchemaFS
	releaseSchemaFS = fsys
	t.Cleanup(func() { releaseSchemaFS = original })
}

func loadPublishedCurrentMigrationRows(t *testing.T) map[int]migrationRow {
	t.Helper()

	fixtureSQL, err := fs.ReadFile(releaseSchemaFS, "testdata/release-schemas/v13.sql")
	if err != nil {
		t.Fatal(err)
	}
	rows := loadFixtureMigrationRows(t, fixtureSQL)

	result := map[int]migrationRow{}
	for _, row := range rows {
		result[row.version] = row
	}
	return result
}

func loadFixtureMigrationRows(t *testing.T, fixtureSQL []byte) []migrationRow {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(string(fixtureSQL)); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query("SELECT version, name, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var result []migrationRow
	for rows.Next() {
		var row migrationRow
		if err := rows.Scan(&row.version, &row.name, &row.checksum); err != nil {
			t.Fatal(err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
