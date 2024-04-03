package services

import (
	"context"
	"database/sql"
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
			UUID:      &uuid,
			Before:    1,
			After:     1,
			MaxTokens: DefaultContextMaxTokens,
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
			MaxTokens:  DefaultContextMaxTokens,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertContextWindow(t, response.Records)
	})
}

func TestValidateSearchRequestExactErrorsAndOrder(t *testing.T) {
	tests := []struct {
		name    string
		request SearchRequest
		want    string
	}{
		{
			name: "zero limit keeps internal default",
			request: SearchRequest{
				Query: "needle",
			},
		},
		{
			name: "maximum limit",
			request: SearchRequest{
				Query:   "needle",
				Options: models.SearchOptions{Limit: 300},
			},
		},
		{
			name: "missing query wins",
			request: SearchRequest{
				Options: models.SearchOptions{ContentType: "audio", Limit: -1, Offset: -1},
				Relax:   true,
			},
			want: "search query required (use --text <query> or positional argument)",
		},
		{
			name: "content type wins",
			request: SearchRequest{
				Query:   "needle",
				Options: models.SearchOptions{ContentType: "audio", Limit: -1, Offset: -1},
			},
			want: "invalid --content-type \"audio\"; must be one of: text, code, tool, reasoning",
		},
		{
			name:    "negative limit wins",
			request: SearchRequest{Query: `"unclosed`, Options: models.SearchOptions{Limit: -1, Offset: -1}, Relax: true},
			want:    "--limit must be between 0 and 300",
		},
		{
			name:    "limit over maximum wins",
			request: SearchRequest{Query: "needle", Options: models.SearchOptions{Limit: 301, Offset: -1}},
			want:    "--limit must be between 0 and 300",
		},
		{
			name:    "negative offset wins",
			request: SearchRequest{Query: `"unclosed`, Options: models.SearchOptions{Limit: 20, Offset: -1}, Relax: true},
			want:    "--offset must be >= 0",
		},
		{
			name:    "relaxation syntax",
			request: SearchRequest{Query: `"unclosed`, Options: models.SearchOptions{Limit: 20}, Relax: true},
			want:    "invalid --relax query: quoted phrases must be closed and whitespace-delimited",
		},
		{
			name:    "relaxation syntax stays opt-in",
			request: SearchRequest{Query: `"unclosed`, Options: models.SearchOptions{Limit: 20}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertExactValidationError(t, ValidateSearchRequest(test.request), test.want)
		})
	}
}

func TestQueryServiceRejectsInvalidRelaxationBeforeDatabaseAccess(t *testing.T) {
	service := QueryService{}
	response, err := service.Search(context.Background(), SearchRequest{
		Query:   `"unclosed`,
		Options: models.SearchOptions{Limit: 20},
		Relax:   true,
	})
	assertExactValidationError(t, err, "invalid --relax query: quoted phrases must be closed and whitespace-delimited")
	if len(response.Results) != 0 || response.Provenance.RelaxationStages != nil {
		t.Fatalf("response = %+v, want empty response", response)
	}
}

func TestValidateExplicitSearchScopeComposesBaseValidation(t *testing.T) {
	tests := []struct {
		name    string
		request SearchRequest
		want    string
	}{
		{
			name: "project",
			request: SearchRequest{
				Query:   "needle",
				Options: models.SearchOptions{Project: "alpha", Limit: 20},
			},
		},
		{
			name: "all projects",
			request: SearchRequest{
				Query:   "needle",
				Options: models.SearchOptions{AllProjects: true, Limit: 20},
			},
		},
		{
			name: "base validation first",
			request: SearchRequest{
				Options: models.SearchOptions{Limit: -1},
			},
			want: "search query required (use --text <query> or positional argument)",
		},
		{
			name: "base bounds before scope",
			request: SearchRequest{
				Query:   "needle",
				Options: models.SearchOptions{Limit: 301},
			},
			want: "--limit must be between 0 and 300",
		},
		{
			name:    "missing scope",
			request: SearchRequest{Query: "needle", Options: models.SearchOptions{Limit: 20}},
			want:    "provide exactly one project scope: project or all_projects=true",
		},
		{
			name: "ambiguous scope",
			request: SearchRequest{
				Query:   "needle",
				Options: models.SearchOptions{Project: "alpha", AllProjects: true, Limit: 20},
			},
			want: "provide exactly one project scope: project or all_projects=true",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertExactValidationError(t, ValidateExplicitSearchScope(test.request), test.want)
		})
	}
}

func TestValidateContextRequestExactErrorsAndOrder(t *testing.T) {
	if DefaultContextMaxTokens != 2000 {
		t.Fatalf("DefaultContextMaxTokens = %d, want 2000", DefaultContextMaxTokens)
	}

	uuid := "opaque"
	path := "/session"
	ordinal := int64(-9)
	empty := ""
	tests := []struct {
		name    string
		request ContextRequest
		want    string
	}{
		{
			name:    "current default",
			request: ContextRequest{UUID: &uuid, MaxTokens: DefaultContextMaxTokens},
		},
		{
			name:    "minimum bounds",
			request: ContextRequest{UUID: &uuid, Before: 0, After: 0, MaxTokens: 64},
		},
		{
			name:    "maximum bounds",
			request: ContextRequest{SourcePath: &path, Ordinal: &ordinal, Before: 50, After: 50, MaxTokens: 16384},
		},
		{
			name:    "selector wins",
			request: ContextRequest{Before: -1, After: -1, MaxTokens: 63},
			want:    "provide exactly one anchor selector: --uuid or --source-path with --ordinal",
		},
		{
			name:    "ambiguous selector",
			request: ContextRequest{UUID: &uuid, SourcePath: &path, Ordinal: &ordinal, MaxTokens: DefaultContextMaxTokens},
			want:    "provide exactly one anchor selector: --uuid or --source-path with --ordinal",
		},
		{
			name:    "empty UUID wins",
			request: ContextRequest{UUID: &empty, Before: -1, After: -1, MaxTokens: 63},
			want:    "--uuid must not be empty",
		},
		{
			name:    "empty path wins",
			request: ContextRequest{SourcePath: &empty, Ordinal: &ordinal, Before: -1, After: -1, MaxTokens: 63},
			want:    "--source-path must not be empty",
		},
		{
			name:    "before wins",
			request: ContextRequest{UUID: &uuid, Before: 51, After: -1, MaxTokens: 63},
			want:    "--before must be between 0 and 50",
		},
		{
			name:    "after wins",
			request: ContextRequest{UUID: &uuid, After: 51, MaxTokens: 63},
			want:    "--after must be between 0 and 50",
		},
		{
			name:    "max tokens low",
			request: ContextRequest{UUID: &uuid, MaxTokens: 63},
			want:    "--max-tokens must be between 64 and 16384",
		},
		{
			name:    "max tokens high",
			request: ContextRequest{UUID: &uuid, MaxTokens: 16385},
			want:    "--max-tokens must be between 64 and 16384",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertExactValidationError(t, ValidateContextRequest(test.request), test.want)
		})
	}
}

func TestQueryServiceRejectsInvalidRequestsWithoutDatabaseAccess(t *testing.T) {
	service := QueryService{}
	uuid := "opaque"
	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "search limit",
			run: func() error {
				_, err := service.Search(context.Background(), SearchRequest{Query: "needle", Options: models.SearchOptions{Limit: -1}})
				return err
			},
			want: "--limit must be between 0 and 300",
		},
		{
			name: "search limit over maximum",
			run: func() error {
				_, err := service.Search(context.Background(), SearchRequest{Query: "needle", Options: models.SearchOptions{Limit: 301}})
				return err
			},
			want: "--limit must be between 0 and 300",
		},
		{
			name: "search offset",
			run: func() error {
				_, err := service.Search(context.Background(), SearchRequest{Query: "needle", Options: models.SearchOptions{Offset: -1}})
				return err
			},
			want: "--offset must be >= 0",
		},
		{
			name: "context max tokens",
			run: func() error {
				_, err := service.Context(context.Background(), ContextRequest{UUID: &uuid, MaxTokens: 63})
				return err
			},
			want: "--max-tokens must be between 64 and 16384",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertExactValidationError(t, test.run(), test.want)
		})
	}
}

func assertExactValidationError(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("error = %q, want nil", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("error = nil, want %q", want)
	}
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
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
	if _, err := service.Context(ctx, ContextRequest{UUID: &uuid, MaxTokens: DefaultContextMaxTokens}); !errors.Is(err, context.Canceled) {
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
				_, err := service.Context(ctx, ContextRequest{UUID: &uuid, MaxTokens: DefaultContextMaxTokens})
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

func TestQueryServiceAnnotationIdentityParity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, u := range []string{"opaque one", "opaque two"} {
		if _, err := raw.Exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,content_type,uuid) VALUES ('session','/duplicate',7,'user','hello','text',?)`, u); err != nil {
			t.Fatal(err)
		}
	}
	service := QueryService{DB: db}
	p := "/duplicate"
	n := int64(7)
	absent := "absent"
	u := "opaque two"
	for _, c := range []struct {
		r    ContextRequest
		want error
	}{
		{ContextRequest{SourcePath: &p, Ordinal: &n, MaxTokens: DefaultContextMaxTokens}, storage.ErrContextRecordAmbiguous},
		{ContextRequest{UUID: &absent, MaxTokens: DefaultContextMaxTokens}, storage.ErrContextRecordNotFound},
	} {
		_, err := service.Context(context.Background(), c.r)
		if !errors.Is(err, c.want) {
			t.Fatalf("context: %v", err)
		}
		uuid, path, ordinal := "", "", -1
		if c.r.UUID != nil {
			uuid = *c.r.UUID
		} else {
			path, ordinal = *c.r.SourcePath, int(*c.r.Ordinal)
		}
		if err := db.UpsertAnnotation(uuid, path, ordinal, "correction", "rejected"); !errors.Is(err, c.want) {
			t.Fatalf("annotation: %v", err)
		}
	}
	response, err := service.Context(context.Background(), ContextRequest{UUID: &u, MaxTokens: DefaultContextMaxTokens})
	if err != nil || len(response.Records) != 1 || response.Records[0].SourcePath != p || response.Records[0].Ordinal != n {
		t.Fatalf("UUID context: %+v %v", response, err)
	}
	if err := db.UpsertAnnotation(u, "", -1, "correction", "reviewed"); err != nil {
		t.Fatal(err)
	}
}

func TestQueryServiceExplicitNegativeAnnotationSelector(t *testing.T) {
	path := filepath.Join(t.TempDir(), "negative.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SyncFiles([]storage.IndexedFile{{SourcePath: "/canonical", Source: "session", Hash: "fixture", Messages: []storage.IndexedMessage{{UUID: "opaque", Ordinal: 7, Role: "user", Text: "fixture", ContentType: "text"}}}}); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if err := db.UpsertAnnotationWithOrdinal("opaque", "", nil, "correction", "original"); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var state string
		if err := raw.QueryRow(`SELECT json_group_array(json_array(id,item_uuid,source_path,ordinal,kind,label,source,created_at)) FROM annotations`).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	service := QueryService{DB: db}
	uuid := "opaque"
	before := snapshot()
	for _, n := range []int{-1, -2} {
		ordinal := int64(n)
		if _, err := service.Context(context.Background(), ContextRequest{UUID: &uuid, Ordinal: &ordinal, MaxTokens: DefaultContextMaxTokens}); err == nil {
			t.Errorf("context accepted explicit extra %d", n)
		}
		if err := db.UpsertAnnotationWithOrdinal(uuid, "", &n, "correction", "rejected"); err == nil {
			t.Errorf("annotation accepted explicit %d", n)
		}
		if snapshot() != before {
			t.Errorf("explicit %d changed annotation state", n)
		}
	}
	if _, err := service.Context(context.Background(), ContextRequest{UUID: &uuid, MaxTokens: DefaultContextMaxTokens}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAnnotationWithOrdinal(uuid, "", nil, "correction", "accepted"); err != nil {
		t.Fatal(err)
	}
}

func TestSearchCandidatePaginationProvenance(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "candidate.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	file := storage.IndexedFile{SourcePath: "/pool/core.jsonl", Source: "session", Project: "alpha", Hash: "core", Tags: []string{"testing"}}
	for _, kind := range []string{"text", "tool"} {
		for range 200 {
			file.Messages = append(file.Messages, storage.IndexedMessage{
				Ordinal: len(file.Messages), Role: "assistant", ContentType: kind,
				Text: "orchard handshake exactboundary", Timestamp: "2026-01-01T00:00:00Z",
			})
		}
	}
	for i := range 205 {
		file.Messages = append(file.Messages, storage.IndexedMessage{
			Ordinal: len(file.Messages), Role: "assistant", ContentType: "tool",
			Text:       "Bash command=backscroll search orchard handshake exactboundary",
			SearchEcho: i%2 == 0, Timestamp: "2026-01-01T00:00:00Z",
		})
	}
	if err := db.SyncFiles([]storage.IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	service := QueryService{DB: db}
	options := models.SearchOptions{Project: "alpha", Source: "session", SourcePath: "/pool/*", Role: "assistant", Tag: "testing", Limit: 300}
	search := func(query string, opts models.SearchOptions, relax bool) SearchResponse {
		t.Helper()
		response, err := service.Search(context.Background(), SearchRequest{Query: query, Options: opts, Relax: relax})
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	baseline := search("orchard handshake", options, false)
	if len(baseline.Results) != 300 || baseline.Provenance.CandidateLimit != 200 || baseline.Provenance.CandidateLimitReached {
		t.Fatalf("exact boundary with hidden echoes = %d, %+v", len(baseline.Results), baseline.Provenance)
	}
	extra := storage.IndexedFile{SourcePath: "/pool/extra.jsonl", Source: "session", Project: "alpha", Hash: "extra", Tags: []string{"testing"}}
	for _, kind := range []string{"text", "tool"} {
		extra.Messages = append(extra.Messages, storage.IndexedMessage{Ordinal: len(extra.Messages), Role: "assistant", ContentType: kind, Text: "orchard handshake overflowboundary", Timestamp: "2026-01-01T00:00:00Z"})
	}
	for range 450 {
		extra.Messages = append(extra.Messages, storage.IndexedMessage{Ordinal: len(extra.Messages), Role: "assistant", ContentType: "text", Text: "adaptation unrelated distractor", Timestamp: "2026-01-01T00:00:00Z"})
	}
	if err := db.SyncFiles([]storage.IndexedFile{extra}); err != nil {
		t.Fatal(err)
	}
	capped := search("orchard handshake", options, false)
	if !capped.Provenance.CandidateLimitReached || !reflect.DeepEqual(capped.Results, baseline.Results) {
		t.Fatalf("overflow changed first 200 per stream or lost provenance: %+v", capped.Provenance)
	}
	late := options
	late.Offset = 300
	page := search("orchard handshake", late, false)
	if len(page.Results) != 100 || !page.Provenance.CandidateLimitReached || page.Results[0].ID == capped.Results[0].ID {
		t.Fatalf("late merged page = %d, %+v", len(page.Results), page.Provenance)
	}
	late.Offset = 400
	for _, relax := range []bool{false, true} {
		response := search("orchard handshake", late, relax)
		if len(response.Results) != 0 || !response.Provenance.CandidateLimitReached {
			t.Fatalf("empty capped page relax=%t: %+v", relax, response)
		}
	}
	for _, kind := range []string{"text", "tool"} {
		typed := options
		typed.ContentType, typed.Offset = kind, 200
		response := search("orchard handshake", typed, false)
		if len(response.Results) == 0 || response.Provenance.CandidateLimit != 0 || response.Provenance.CandidateLimitReached {
			t.Fatalf("typed late %s page = %+v", kind, response)
		}
	}
	exact := search("exactboundary", options, false)
	if exact.Provenance.CandidateLimitReached {
		t.Fatalf("exactly 200 plus echo-only overflow marked capped: %+v", exact.Provenance)
	}
	scoped := options
	scoped.SourcePath = "/pool/core.jsonl"
	if response := search("orchard handshake", scoped, false); response.Provenance.CandidateLimitReached {
		t.Fatalf("out-of-scope 201st row marked capped: %+v", response.Provenance)
	}
	for _, offset := range []int{0, 400} {
		relaxed := options
		relaxed.Offset = offset
		response := search("orchard handshake adaptation", relaxed, true)
		if !response.Provenance.CandidateLimitReached || !reflect.DeepEqual(response.Provenance.RelaxationStages, []string{"strict", "drop-terms/1"}) {
			t.Fatalf("relaxed producing-stage provenance offset=%d: %+v", offset, response.Provenance)
		}
	}
	direct, provenance, err := db.Search("orchard handshake", late)
	if err != nil || len(direct) != 0 || !provenance.CandidateLimitReached || provenance.CandidateLimit != 200 {
		t.Fatalf("direct consumer late-page provenance: %d %+v %v", len(direct), provenance, err)
	}
}
