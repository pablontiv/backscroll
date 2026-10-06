package storage

import (
	"encoding/json"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestIndexedContextReadsPersistedOrigin(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	const sourcePath = "/sessions/origin-context.jsonl"
	origins := []models.MessageOrigin{
		models.OriginHuman,
		models.OriginAssistant,
		models.OriginSystem,
		models.OriginAutomation,
	}
	for ordinal, origin := range origins {
		if _, err := db.db.Exec(`
			INSERT INTO search_items
				(source, source_path, ordinal, role, text, uuid, content_type, origin, origin_version)
			VALUES ('session', ?, ?, 'user', 'indexed origin context sentinel', ?, 'text', ?, 1)
		`, sourcePath, ordinal, "origin-context-current-"+string(origin), origin); err != nil {
			t.Fatalf("insert current origin %q: %v", origin, err)
		}
	}

	// Rows predating origin provenance acquire the schema default and must be
	// surfaced as unknown rather than inferred from their stored role.
	if _, err := db.db.Exec(`
		INSERT INTO search_items
			(source, source_path, ordinal, role, text, uuid, content_type)
		VALUES ('session', ?, 4, 'user', 'indexed origin context sentinel', 'origin-context-historical', 'text')
	`, sourcePath); err != nil {
		t.Fatalf("insert historical row: %v", err)
	}

	want := append(append([]models.MessageOrigin(nil), origins...), models.OriginUnknown)

	records, err := db.QueryIndexedRecords(IndexedRecordQuery{SourcePath: stringPointer(sourcePath)})
	if err != nil {
		t.Fatalf("QueryIndexedRecords: %v", err)
	}
	assertOriginsByOrdinal(t, "indexed records", recordOrigins(records), want)

	results, err := db.Search("sentinel", models.SearchOptions{
		ContentType: "text",
		SourcePath:  sourcePath,
		Limit:       len(want),
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	gotSearchOrigins := make(map[int]models.MessageOrigin, len(results))
	for _, result := range results {
		gotSearchOrigins[result.Ordinal] = result.Origin

		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("marshal search result: %v", err)
		}
		var public map[string]any
		if err := json.Unmarshal(encoded, &public); err != nil {
			t.Fatalf("decode search result JSON: %v", err)
		}
		if _, exposed := public["Origin"]; exposed {
			t.Fatalf("search JSON exposed Origin: %s", encoded)
		}
		if _, exposed := public["origin"]; exposed {
			t.Fatalf("search JSON exposed origin: %s", encoded)
		}
	}
	assertOriginsByOrdinal(t, "search results", gotSearchOrigins, want)
}

func stringPointer(value string) *string {
	return &value
}

func recordOrigins(records []models.IndexedRecord) map[int]models.MessageOrigin {
	origins := make(map[int]models.MessageOrigin, len(records))
	for _, record := range records {
		origins[int(record.Ordinal)] = record.Origin
	}
	return origins
}

func assertOriginsByOrdinal(t *testing.T, context string, got map[int]models.MessageOrigin, want []models.MessageOrigin) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s count = %d, want %d: %v", context, len(got), len(want), got)
	}
	for ordinal, origin := range want {
		if got[ordinal] != origin {
			t.Errorf("%s origin at ordinal %d = %q, want %q", context, ordinal, got[ordinal], origin)
		}
	}
}
