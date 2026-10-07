package services

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
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

func TestValidateSearchRequestAndExplicitScope(t *testing.T) {
	for _, contentType := range []string{"", "text", "code", "tool", "reasoning"} {
		request := SearchRequest{Query: "needle", Options: models.SearchOptions{ContentType: contentType}}
		if err := ValidateSearchRequest(request); err != nil {
			t.Errorf("content type %q: %v", contentType, err)
		}
	}

	tests := []struct {
		name    string
		request SearchRequest
		want    string
	}{
		{
			name: "missing query",
			want: "search query required (use --text <query> or positional argument)",
		},
		{
			name:    "invalid content type",
			request: SearchRequest{Query: "needle", Options: models.SearchOptions{ContentType: "audio"}},
			want:    "invalid --content-type \"audio\"; must be one of: text, code, tool, reasoning",
		},
		{
			name:    "invalid relaxation query",
			request: SearchRequest{Query: `"unclosed`, Relax: true},
			want:    "invalid --relax query",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateSearchRequest(test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}

	for _, request := range []SearchRequest{
		{Options: models.SearchOptions{Project: "alpha"}},
		{Options: models.SearchOptions{AllProjects: true}},
	} {
		if err := ValidateExplicitSearchScope(request); err != nil {
			t.Errorf("explicit scope %+v: %v", request.Options, err)
		}
	}
	for _, request := range []SearchRequest{
		{},
		{Options: models.SearchOptions{Project: "alpha", AllProjects: true}},
	} {
		if err := ValidateExplicitSearchScope(request); err == nil {
			t.Errorf("ambiguous or missing scope %+v was accepted", request.Options)
		}
	}
}

func TestValidateContextRequestSelectorsAndLimits(t *testing.T) {
	uuid := "opaque"
	path := "/session"
	ordinal := int64(-9)
	for _, request := range []ContextRequest{
		{UUID: &uuid, Before: 0, After: 50},
		{SourcePath: &path, Ordinal: &ordinal, Before: 50, After: 0},
	} {
		if err := ValidateContextRequest(request); err != nil {
			t.Errorf("valid request %+v: %v", request, err)
		}
	}

	empty := ""
	tests := []struct {
		name    string
		request ContextRequest
		want    string
	}{
		{name: "no selector", want: "provide exactly one anchor selector"},
		{name: "both selectors", request: ContextRequest{UUID: &uuid, SourcePath: &path, Ordinal: &ordinal}, want: "provide exactly one anchor selector"},
		{name: "path without ordinal", request: ContextRequest{SourcePath: &path}, want: "provide exactly one anchor selector"},
		{name: "ordinal without path", request: ContextRequest{Ordinal: &ordinal}, want: "provide exactly one anchor selector"},
		{name: "empty UUID", request: ContextRequest{UUID: &empty}, want: "--uuid must not be empty"},
		{name: "empty path", request: ContextRequest{SourcePath: &empty, Ordinal: &ordinal}, want: "--source-path must not be empty"},
		{name: "before low", request: ContextRequest{UUID: &uuid, Before: -1}, want: "--before must be between 0 and 50"},
		{name: "before high", request: ContextRequest{UUID: &uuid, Before: 51}, want: "--before must be between 0 and 50"},
		{name: "after low", request: ContextRequest{UUID: &uuid, After: -1}, want: "--after must be between 0 and 50"},
		{name: "after high", request: ContextRequest{UUID: &uuid, After: 51}, want: "--after must be between 0 and 50"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateContextRequest(test.request)
			if err == nil || err.Error() != test.want && !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestQueryServiceRejectsInvalidRequestsWithoutDatabaseAccess(t *testing.T) {
	service := QueryService{}
	if _, err := service.Search(context.Background(), SearchRequest{}); err == nil || !strings.Contains(err.Error(), "search query required") {
		t.Fatalf("search error = %v", err)
	}
	if _, err := service.Context(context.Background(), ContextRequest{}); err == nil || !strings.Contains(err.Error(), "exactly one anchor selector") {
		t.Fatalf("context error = %v", err)
	}
}

func TestQueryServiceChecksCancellationWithoutDatabaseAccess(t *testing.T) {
	service := QueryService{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.Search(ctx, SearchRequest{Query: "needle"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("search error = %v, want context.Canceled", err)
	}
	uuid := "opaque"
	if _, err := service.Context(ctx, ContextRequest{UUID: &uuid}); !errors.Is(err, context.Canceled) {
		t.Fatalf("context error = %v, want context.Canceled", err)
	}
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
