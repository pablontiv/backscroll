package compat

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

//go:embed testdata/release-schemas/*
var embeddedReleaseSchemaFS embed.FS

var releaseSchemaFS fs.FS = embeddedReleaseSchemaFS

type Catalog struct {
	FirstGoRelease  string
	LatestGoRelease string
	Releases        []struct {
		Tag              string
		Fixture          string
		ProvenanceSHA256 string
	}
}

func LoadCatalog() (Catalog, error) {
	return loadCatalogFromFS(releaseSchemaFS)
}

func loadCatalogFromFS(fsys fs.FS) (Catalog, error) {
	data, err := fs.ReadFile(fsys, "testdata/release-schemas/manifest.json")
	if err != nil {
		return Catalog{}, fmt.Errorf("read release schema catalog: %w", err)
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("parse release schema catalog: %w", err)
	}
	if catalog.FirstGoRelease != "v0.3.7" {
		return Catalog{}, fmt.Errorf("release schema catalog first Go release = %q, want v0.3.7", catalog.FirstGoRelease)
	}
	if compareSemver(catalog.LatestGoRelease, "v3.2.5") < 0 {
		return Catalog{}, fmt.Errorf("release schema catalog latest Go release = %q, want at least v3.2.5", catalog.LatestGoRelease)
	}

	seen := make(map[string]bool, len(catalog.Releases))
	for _, release := range catalog.Releases {
		if release.Tag == "" || release.Fixture == "" || release.ProvenanceSHA256 == "" {
			return Catalog{}, fmt.Errorf("release schema catalog has incomplete release mapping: %+v", release)
		}
		if seen[release.Tag] {
			return Catalog{}, fmt.Errorf("release schema catalog has duplicate release tag %q", release.Tag)
		}
		seen[release.Tag] = true
		fixturePath := "testdata/release-schemas/" + release.Fixture
		fixtureBytes, err := fs.ReadFile(fsys, fixturePath)
		if err != nil {
			return Catalog{}, fmt.Errorf("release schema fixture %q: %w", release.Fixture, err)
		}
		actualSHA256 := fmt.Sprintf("%x", sha256.Sum256(fixtureBytes))
		if actualSHA256 != release.ProvenanceSHA256 {
			return Catalog{}, fmt.Errorf("release schema fixture %q SHA-256 = %s, want %s", release.Fixture, actualSHA256, release.ProvenanceSHA256)
		}
	}
	if !seen[catalog.FirstGoRelease] || !seen[catalog.LatestGoRelease] {
		return Catalog{}, fmt.Errorf("release schema catalog endpoints missing: %s..%s", catalog.FirstGoRelease, catalog.LatestGoRelease)
	}

	return catalog, nil
}

func compareSemver(left, right string) int {
	var lMajor, lMinor, lPatch int
	var rMajor, rMinor, rPatch int
	_, _ = fmt.Sscanf(left, "v%d.%d.%d", &lMajor, &lMinor, &lPatch)
	_, _ = fmt.Sscanf(right, "v%d.%d.%d", &rMajor, &rMinor, &rPatch)
	if lMajor != rMajor {
		return lMajor - rMajor
	}
	if lMinor != rMinor {
		return lMinor - rMinor
	}
	return lPatch - rPatch
}
