package compat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RegenerateManifestJSON reads all fixtures and regenerates the manifest.json with
// new signatures computed using the current normalizeSQL implementation.
// This is called after changing normalizeSQL to ensure all signatures remain valid.
func RegenerateManifestJSON(manifestPath string) error {
	// Load the old manifest to preserve the release mappings and unmapped fixtures
	oldCatalog, err := loadCatalogFromPath(manifestPath)
	if err != nil {
		return fmt.Errorf("load old manifest: %w", err)
	}

	// Scan the testdata/release-schemas directory for all .sql files
	fixturesDir := filepath.Dir(manifestPath)

	// Build a map of fixture -> new signature
	fixtureSignatures := make(map[string]string)
	fixtureProvenances := make(map[string]string)

	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		return fmt.Errorf("read fixtures directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		fixturePath := filepath.Join(fixturesDir, entry.Name())
		fixtureData, err := os.ReadFile(fixturePath)
		if err != nil {
			return fmt.Errorf("read fixture %s: %w", entry.Name(), err)
		}

		// Compute provenance SHA256
		provenance := fmt.Sprintf("%x", sha256.Sum256(fixtureData))

		// Open and inspect the fixture
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			return fmt.Errorf("open sqlite for fixture %s: %w", entry.Name(), err)
		}
		if _, err := db.Exec(string(fixtureData)); err != nil {
			db.Close()
			return fmt.Errorf("execute fixture %s: %w", entry.Name(), err)
		}

		shape, err := inspectShape(context.Background(), db)
		db.Close()
		if err != nil {
			return fmt.Errorf("inspect fixture %s: %w", entry.Name(), err)
		}

		fixtureSignatures[entry.Name()] = shape.Signature
		fixtureProvenances[entry.Name()] = provenance
	}

	// Update the catalog with new signatures
	newCatalog := Catalog{
		FirstGoRelease:       oldCatalog.FirstGoRelease,
		LatestGoRelease:      oldCatalog.LatestGoRelease,
		Releases:             []catalogRelease{},
		UnmanifestedFixtures: []catalogFixture{},
	}

	// Update manifested releases with new signatures
	for _, oldRelease := range oldCatalog.Releases {
		newSig, ok := fixtureSignatures[oldRelease.Fixture]
		if !ok {
			return fmt.Errorf("fixture not found for release %s: %s", oldRelease.Tag, oldRelease.Fixture)
		}
		newProvenance, ok := fixtureProvenances[oldRelease.Fixture]
		if !ok {
			return fmt.Errorf("provenance not computed for fixture %s", oldRelease.Fixture)
		}

		// Check if provenance matches - if not, warn that fixture may have changed
		if oldRelease.ProvenanceSHA256 != newProvenance {
			fmt.Fprintf(os.Stderr, "WARNING: fixture %s for release %s changed (provenance mismatch)\n",
				oldRelease.Fixture, oldRelease.Tag)
		}

		newCatalog.Releases = append(newCatalog.Releases, catalogRelease{
			Tag:               oldRelease.Tag,
			Fixture:           oldRelease.Fixture,
			ProvenanceSHA256:  newProvenance, // Use new provenance in case fixture was regenerated
			Signature:         newSig,
			AppliedVersion:    oldRelease.AppliedVersion,
			HasSourceMetadata: oldRelease.HasSourceMetadata,
		})
	}

	// Update unmanifested fixtures with new signatures
	for _, oldUnmanifested := range oldCatalog.UnmanifestedFixtures {
		newSig, ok := fixtureSignatures[oldUnmanifested.Fixture]
		if !ok {
			return fmt.Errorf("fixture not found for unmanifested: %s", oldUnmanifested.Fixture)
		}
		newProvenance, ok := fixtureProvenances[oldUnmanifested.Fixture]
		if !ok {
			return fmt.Errorf("provenance not computed for fixture %s", oldUnmanifested.Fixture)
		}

		newCatalog.UnmanifestedFixtures = append(newCatalog.UnmanifestedFixtures, catalogFixture{
			Fixture:           oldUnmanifested.Fixture,
			ProvenanceSHA256:  newProvenance,
			Signature:         newSig,
			AppliedVersion:    oldUnmanifested.AppliedVersion,
			HasSourceMetadata: oldUnmanifested.HasSourceMetadata,
			Provenance:        oldUnmanifested.Provenance,
		})
	}

	// Marshal to JSON with nice formatting
	data, err := json.MarshalIndent(newCatalog, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}

	// Write to file
	if err := os.WriteFile(manifestPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	fmt.Printf("Regenerated manifest with %d releases and %d unmanifested fixtures\n",
		len(newCatalog.Releases), len(newCatalog.UnmanifestedFixtures))
	return nil
}

func loadCatalogFromPath(path string) (Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, err
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("unmarshal catalog: %w", err)
	}
	return catalog, nil
}
