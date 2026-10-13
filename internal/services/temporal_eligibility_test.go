package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestSearchServiceHalfOpenTemporalEligibility(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BACKSCROLL_CONFIG_DIR", t.TempDir())
	db, err := storage.Open(filepath.Join(t.TempDir(), "dates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stamp := range []string{"2026-01-04T23:59:59Z", "2026-01-05T00:00:00Z", "2026-01-05T12:00:00Z", "2026-01-06T00:00:00Z", ""} {
		if err := db.SyncFiles([]storage.IndexedFile{{Source: "session", SourcePath: stamp, Hash: "fixture", Project: "p", Messages: []storage.IndexedMessage{{UUID: stamp + "uuid", ContentType: "text", Text: "temporal eligibility marker", Timestamp: stamp}}}}); err != nil {
			t.Fatal(err)
		}
	}
	lower, _ := time.Parse(time.RFC3339, "2026-01-05T00:00:00Z")
	upper, _ := time.Parse(time.RFC3339, "2026-01-06T00:00:00Z")
	// Sync accepts historical empty strings; this fixture specifically tests
	// an unavailable (SQL NULL) timestamp, not lexical empty-string ordering.
	if _, err := db.DB().Exec(`UPDATE search_items SET timestamp = NULL WHERE timestamp = ''`); err != nil {
		t.Fatal(err)
	}
	service := QueryService{DB: db}
	for _, tc := range []struct {
		name          string
		after, before *time.Time
		want          int
	}{
		{"inclusive lower", &lower, nil, 3}, {"exclusive upper", nil, &upper, 3}, {"half open", &lower, &upper, 2}, {"empty interval", &lower, &lower, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := service.Search(context.Background(), SearchRequest{Query: "temporal", Options: models.SearchOptions{AllProjects: true, After: tc.after, Before: tc.before}})
			if err != nil {
				t.Fatal(err)
			}
			if len(response.Results) != tc.want {
				t.Errorf("results=%d want %d", len(response.Results), tc.want)
			}
		})
	}
}
