package storage

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestAggregateCorrectionsFiltersOriginBeforePagination(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	rows := []struct {
		uuid       string
		origin     models.MessageOrigin
		confidence float64
	}{
		{uuid: "automation", origin: models.OriginAutomation, confidence: 0.99},
		{uuid: "human-one", origin: models.OriginHuman, confidence: 0.90},
		{uuid: "human-two", origin: models.OriginHuman, confidence: 0.80},
		{uuid: "unknown", origin: models.OriginUnknown, confidence: 0.70},
	}
	for ordinal, row := range rows {
		if _, err := db.db.Exec(`
			INSERT INTO search_items
				(source, source_path, ordinal, role, text, uuid, project, content_type, origin, origin_version)
			VALUES ('session', '/p/corrections.jsonl', ?, 'user', ?, ?, 'project', 'text', ?, 1)
		`, ordinal, "text-"+row.uuid, row.uuid, row.origin); err != nil {
			t.Fatalf("insert search item %q: %v", row.uuid, err)
		}
		if _, err := db.db.Exec(`
			INSERT INTO correction_signals
				(source_path, ordinal, detector, confidence, extraction_version)
			VALUES ('/p/corrections.jsonl', ?, 'test', ?, 1)
		`, ordinal, row.confidence); err != nil {
			t.Fatalf("insert correction signal %q: %v", row.uuid, err)
		}
	}

	got, err := db.AggregateCorrections(CorrectionAggOpts{
		Origin: models.OriginHuman,
		Limit:  1,
		Offset: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].UUID != "human-two" {
		t.Fatalf("filtered second page = %+v, want human-two", got)
	}
	if got[0].Origin != models.OriginHuman {
		t.Fatalf("filtered origin = %q, want human", got[0].Origin)
	}
}

func TestAggregateCorrectionsWithoutOriginPreservesPopulationAndShape(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	for ordinal, origin := range []models.MessageOrigin{models.OriginHuman, models.OriginAssistant} {
		uuid := string(origin)
		if _, err := db.db.Exec(`
			INSERT INTO search_items
				(source, source_path, ordinal, role, text, uuid, project, content_type, origin, origin_version)
			VALUES ('session', '/p/unfiltered.jsonl', ?, 'user', ?, ?, 'project', 'text', ?, 1)
		`, ordinal, uuid+" text", uuid, origin); err != nil {
			t.Fatal(err)
		}
		if _, err := db.db.Exec(`
			INSERT INTO correction_signals
				(source_path, ordinal, detector, confidence, extraction_version)
			VALUES ('/p/unfiltered.jsonl', ?, 'test', ?, 1)
		`, ordinal, 0.9-float64(ordinal)/10); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.AggregateCorrections(CorrectionAggOpts{})
	if err != nil {
		t.Fatal(err)
	}
	want := []CorrectionCandidate{
		{UUID: "human", SourcePath: "/p/unfiltered.jsonl", Ordinal: 0, Detectors: []string{"test"}, MaxConfidence: 0.9, TextSnippet: "human text"},
		{UUID: "assistant", SourcePath: "/p/unfiltered.jsonl", Ordinal: 1, Detectors: []string{"test"}, MaxConfidence: 0.8, TextSnippet: "assistant text"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unfiltered corrections = %+v, want %+v", got, want)
	}
}

func TestAggregateCorrectionsRejectsInvalidOrigin(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	_, err := db.AggregateCorrections(CorrectionAggOpts{Origin: models.MessageOrigin("user")})
	if err == nil || !strings.Contains(err.Error(), "invalid correction origin") {
		t.Fatalf("invalid origin error = %v", err)
	}
}
