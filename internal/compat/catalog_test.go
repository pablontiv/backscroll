package compat

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pablontiv/backscroll/internal/storage"
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
				{"Tag": "v0.3.7", "Fixture": "v1.sql", "ProvenanceSHA256": %q},
				{"Tag": "v3.2.5", "Fixture": "v1.sql", "ProvenanceSHA256": %q}
			]
		}`, provenance, provenance))},
		"testdata/release-schemas/v1.sql": {Data: modified},
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("fixture hash drift error = %v", err)
	}
}

func TestFixtureMigrationRowsMatchStorageMigrationRunner(t *testing.T) {
	authoritative := loadAuthoritativeMigrationRows(t)

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
				if row.name != expected.name || row.checksum != expected.checksum {
					t.Fatalf("migration %d row = (%q, %q), want (%q, %q)", row.version, row.name, row.checksum, expected.name, expected.checksum)
				}
			}
		})
	}
}

func TestUnmanifestedFixtureShapesAreDocumentedLocally(t *testing.T) {
	for _, fixture := range []string{"v2.sql", "v3-no-source-metadata.sql", "v5-without-source-metadata.sql"} {
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

func loadAuthoritativeMigrationRows(t *testing.T) map[int]migrationRow {
	t.Helper()

	db, err := storage.Open(filepath.Join(t.TempDir(), "backscroll.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close authoritative database: %v", err)
		}
	}()

	rows, err := db.DB().Query("SELECT version, name, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	result := map[int]migrationRow{}
	for rows.Next() {
		var row migrationRow
		if err := rows.Scan(&row.version, &row.name, &row.checksum); err != nil {
			t.Fatal(err)
		}
		result[row.version] = row
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
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
