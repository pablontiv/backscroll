package compat

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed testdata/release-schemas/*
var embeddedReleaseSchemaFS embed.FS

var releaseSchemaFS fs.FS = embeddedReleaseSchemaFS

type Catalog struct {
	FirstGoRelease       string
	LatestGoRelease      string
	Releases             []catalogRelease
	UnmanifestedFixtures []catalogFixture

	lineages     map[lineageKey]Lineage
	currentShape SchemaShape
}

type catalogRelease struct {
	Tag               string
	Fixture           string
	ProvenanceSHA256  string
	Signature         string
	AppliedVersion    int
	HasSourceMetadata bool
}

type catalogFixture struct {
	Fixture           string
	ProvenanceSHA256  string
	Signature         string
	AppliedVersion    int
	HasSourceMetadata bool
	Provenance        string
}

type Lineage struct {
	shape          SchemaShape
	remainingSteps []MigrationStep
}

type lineageKey struct {
	appliedVersion int
	signature      string
}

func keyForShape(shape SchemaShape) lineageKey {
	return lineageKey{appliedVersion: shape.AppliedVersion, signature: shape.Signature}
}

func (c Catalog) ByShape(shape SchemaShape) (Lineage, bool) {
	lineage, ok := c.lineages[keyForShape(shape)]
	return lineage, ok
}

func (c Catalog) IsKnownShape(shape SchemaShape) bool {
	_, ok := c.ByShape(shape)
	return ok
}

func (c Catalog) CurrentShape() SchemaShape { return c.currentShape }

func (l Lineage) RemainingSteps() []MigrationStep {
	steps := make([]MigrationStep, len(l.remainingSteps))
	copy(steps, l.remainingSteps)
	return steps
}

func LoadCatalog() (Catalog, error) {
	catalog, err := loadCatalogFromFS(releaseSchemaFS)
	if err != nil {
		return Catalog{}, err
	}
	if err := catalog.attachLineages(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
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
		if release.Tag == "" {
			return Catalog{}, fmt.Errorf("release schema catalog has release without tag: %+v", release)
		}
		if seen[release.Tag] {
			return Catalog{}, fmt.Errorf("release schema catalog has duplicate release tag %q", release.Tag)
		}
		seen[release.Tag] = true
		if err := validateCatalogFixture(fsys, fixtureFromRelease(release)); err != nil {
			return Catalog{}, err
		}
	}
	for _, fixture := range catalog.UnmanifestedFixtures {
		if fixture.Provenance == "" {
			return Catalog{}, fmt.Errorf("unmanifested release schema fixture %q lacks provenance note", fixture.Fixture)
		}
		if err := validateCatalogFixture(fsys, fixture); err != nil {
			return Catalog{}, err
		}
	}
	if !seen[catalog.FirstGoRelease] || !seen[catalog.LatestGoRelease] {
		return Catalog{}, fmt.Errorf("release schema catalog endpoints missing: %s..%s", catalog.FirstGoRelease, catalog.LatestGoRelease)
	}

	return catalog, nil
}

func fixtureFromRelease(release catalogRelease) catalogFixture {
	return catalogFixture{
		Fixture:           release.Fixture,
		ProvenanceSHA256:  release.ProvenanceSHA256,
		Signature:         release.Signature,
		AppliedVersion:    release.AppliedVersion,
		HasSourceMetadata: release.HasSourceMetadata,
	}
}

func validateCatalogFixture(fsys fs.FS, fixture catalogFixture) error {
	if fixture.Fixture == "" || fixture.ProvenanceSHA256 == "" || fixture.Signature == "" || fixture.AppliedVersion == 0 {
		return fmt.Errorf("release schema catalog has incomplete fixture mapping: %+v", fixture)
	}
	if !strings.HasPrefix(fixture.Signature, "sha256:") {
		return fmt.Errorf("release schema fixture %q signature = %q, want sha256", fixture.Fixture, fixture.Signature)
	}
	fixturePath := "testdata/release-schemas/" + fixture.Fixture
	fixtureBytes, err := fs.ReadFile(fsys, fixturePath)
	if err != nil {
		return fmt.Errorf("release schema fixture %q: %w", fixture.Fixture, err)
	}
	actualSHA256 := fmt.Sprintf("%x", sha256.Sum256(fixtureBytes))
	if actualSHA256 != fixture.ProvenanceSHA256 {
		return fmt.Errorf("release schema fixture %q SHA-256 = %s, want %s", fixture.Fixture, actualSHA256, fixture.ProvenanceSHA256)
	}
	return nil
}

func (c Catalog) schemaFixtures() []catalogFixture {
	seen := map[string]bool{}
	fixtures := make([]catalogFixture, 0, len(c.Releases)+len(c.UnmanifestedFixtures))
	for _, release := range c.Releases {
		fixture := fixtureFromRelease(release)
		if seen[fixture.Fixture] {
			continue
		}
		seen[fixture.Fixture] = true
		fixtures = append(fixtures, fixture)
	}
	for _, fixture := range c.UnmanifestedFixtures {
		if seen[fixture.Fixture] {
			continue
		}
		seen[fixture.Fixture] = true
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func (c *Catalog) attachLineages() error {
	lineages := map[lineageKey]Lineage{}
	for _, fixture := range c.schemaFixtures() {
		shape := SchemaShape{AppliedVersion: fixture.AppliedVersion, Signature: fixture.Signature}
		lineage := Lineage{
			shape:          shape,
			remainingSteps: remainingStepsFor(fixture.AppliedVersion, fixture.HasSourceMetadata),
		}
		key := keyForShape(shape)
		if existing, ok := lineages[key]; ok {
			if !sameMigrationSteps(existing.remainingSteps, lineage.remainingSteps) {
				return fmt.Errorf("ambiguous semantic collision version=%d signature=%s: fixtures disagree on remaining migration plan", shape.AppliedVersion, shape.Signature)
			}
			continue
		}
		lineages[key] = lineage
	}
	maxVersion := 0
	for _, lineage := range lineages {
		if lineage.shape.AppliedVersion > maxVersion {
			maxVersion = lineage.shape.AppliedVersion
		}
	}
	if maxVersion == 0 {
		return fmt.Errorf("release schema catalog has no fixtures")
	}

	var maxShape SchemaShape
	for _, lineage := range lineages {
		shape := lineage.shape
		if shape.AppliedVersion != maxVersion {
			continue
		}
		if maxShape.Signature == "" {
			maxShape = shape
			continue
		}
		if shape.Signature != maxShape.Signature {
			return fmt.Errorf("ambiguous current head version=%d: signatures %s and %s", maxVersion, maxShape.Signature, shape.Signature)
		}
	}
	c.currentShape = maxShape
	c.lineages = lineages
	return nil
}

func sameMigrationSteps(left, right []MigrationStep) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
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
