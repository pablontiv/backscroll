package services

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestQueryServiceSearch(t *testing.T) {
	service := newQueryServiceTestFixture(t)

	t.Run("normal", func(t *testing.T) {
		response, err := service.Search(context.Background(), SearchRequest{
			Query: "violet handshake",
			Options: models.SearchOptions{
				SourcePath:  "/target.jsonl",
				ContentType: "text",
				LexicalOnly: true,
				Limit:       10,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(response.Results) != 1 || response.Results[0].SourcePath != "/target.jsonl" {
			t.Fatalf("normal search results = %+v", response.Results)
		}
		if response.Provenance.RelaxationStages != nil {
			t.Fatalf("normal search provenance = %+v", response.Provenance)
		}
	})

	t.Run("relaxed", func(t *testing.T) {
		response, err := service.Search(context.Background(), SearchRequest{
			Query: "violet handshake adaptation",
			Options: models.SearchOptions{
				SourcePath:  "/target.jsonl",
				ContentType: "text",
				Limit:       10,
			},
			Relax: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(response.Results) != 1 {
			t.Fatalf("relaxed search results = %+v", response.Results)
		}
		result := response.Results[0]
		if result.MatchStage != "drop-terms" || !reflect.DeepEqual(result.DroppedTerms, []string{"adaptation"}) {
			t.Fatalf("relaxed result provenance = %+v", result)
		}
		if want := []string{"strict", "drop-terms/1"}; !reflect.DeepEqual(response.Provenance.RelaxationStages, want) {
			t.Fatalf("relaxation stages = %v, want %v", response.Provenance.RelaxationStages, want)
		}
	})
}

func TestQueryServiceContext(t *testing.T) {
	service := newQueryServiceTestFixture(t)

	t.Run("UUID selector", func(t *testing.T) {
		uuid := "anchor"
		response, err := service.Context(context.Background(), ContextRequest{
			UUID:   &uuid,
			Before: 1,
			After:  1,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertContextWindow(t, response.Records)
	})

	t.Run("path and ordinal selector", func(t *testing.T) {
		path := "/context.jsonl"
		ordinal := int64(1)
		response, err := service.Context(context.Background(), ContextRequest{
			SourcePath: &path,
			Ordinal:    &ordinal,
			Before:     1,
			After:      1,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertContextWindow(t, response.Records)
	})
}

func TestQueryServiceStatus(t *testing.T) {
	service := newQueryServiceTestFixture(t)
	response, err := service.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if response.Stats.TotalFiles != 6 || response.Stats.TotalMessages != 8 {
		t.Fatalf("status stats = %+v", response.Stats)
	}
	if response.Stats.IndexedAt.IsZero() {
		t.Fatalf("status omitted indexed timestamp: %+v", response.Stats)
	}
}

func TestQueryServiceCancellation(t *testing.T) {
	service := newQueryServiceTestFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "normal search",
			run: func() error {
				_, err := service.Search(ctx, SearchRequest{Query: "violet", Options: models.SearchOptions{ContentType: "text"}})
				return err
			},
		},
		{
			name: "relaxed search",
			run: func() error {
				_, err := service.Search(ctx, SearchRequest{Query: "violet handshake adaptation", Relax: true})
				return err
			},
		},
		{
			name: "context",
			run: func() error {
				uuid := "anchor"
				_, err := service.Context(ctx, ContextRequest{UUID: &uuid})
				return err
			},
		},
		{
			name: "status",
			run: func() error {
				_, err := service.Status(ctx, StatusRequest{})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func assertContextWindow(t *testing.T, records []storage.ContextRecord) {
	t.Helper()
	if len(records) != 3 {
		t.Fatalf("context records = %+v", records)
	}
	for i, record := range records {
		if record.Ordinal != int64(i) {
			t.Fatalf("record %d ordinal = %d", i, record.Ordinal)
		}
		if record.Anchor != (i == 1) {
			t.Fatalf("record %d anchor = %t", i, record.Anchor)
		}
	}
}

func newQueryServiceTestFixture(t *testing.T) QueryService {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "queries.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	files := []storage.IndexedFile{
		{
			SourcePath: "/target.jsonl",
			Source:     "session",
			Project:    "alpha",
			Hash:       "target",
			Messages: []storage.IndexedMessage{{
				Ordinal: 0, UUID: "target", Role: "assistant", ContentType: "text",
				Text: "violet handshake quartz marker", Timestamp: "2026-01-01T00:00:00Z",
			}},
		},
		{
			SourcePath: "/context.jsonl",
			Source:     "session",
			Project:    "alpha",
			Hash:       "context",
			Messages: []storage.IndexedMessage{
				{Ordinal: 0, UUID: "before", Role: "user", ContentType: "text", Text: "before context"},
				{Ordinal: 1, UUID: "anchor", Role: "assistant", ContentType: "text", Text: "anchor context"},
				{Ordinal: 2, UUID: "after", Role: "user", ContentType: "text", Text: "after context"},
			},
		},
	}
	for i := 0; i < 4; i++ {
		files = append(files, storage.IndexedFile{
			SourcePath: "/noise-" + string(rune('0'+i)) + ".jsonl",
			Source:     "session",
			Project:    "alpha",
			Hash:       "noise-" + string(rune('0'+i)),
			Messages: []storage.IndexedMessage{{
				Ordinal: 0, UUID: "noise-" + string(rune('0'+i)), Role: "assistant", ContentType: "text",
				Text: "adaptation rollout distractor",
			}},
		})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	return QueryService{DB: db}
}
