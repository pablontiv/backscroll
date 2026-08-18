package compat

import (
	"io/fs"
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
