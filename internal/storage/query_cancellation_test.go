package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestCanceledQueryContextsReturnNoPartialResults(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("search", func(t *testing.T) {
		results, err := db.SearchContext(ctx, "cancellation", models.SearchOptions{AllProjects: true})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SearchContext error = %v, want context.Canceled", err)
		}
		if results != nil {
			t.Fatalf("SearchContext returned partial results: %+v", results)
		}
	})

	t.Run("relaxed", func(t *testing.T) {
		results, _, err := db.SearchRelaxedContext(ctx, "cancellation query terms", models.SearchOptions{AllProjects: true})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("SearchRelaxedContext error = %v, want context.Canceled", err)
		}
		if results != nil {
			t.Fatalf("SearchRelaxedContext returned partial results: %+v", results)
		}
	})

	t.Run("status", func(t *testing.T) {
		stats, err := db.GetStatsContext(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("GetStatsContext error = %v, want context.Canceled", err)
		}
		if stats != (Stats{}) {
			t.Fatalf("GetStatsContext returned partial stats: %+v", stats)
		}
	})
}

type blockingEmbeddingProvider struct {
	entered chan struct{}
}

func (p *blockingEmbeddingProvider) Embed(ctx context.Context, _ string) ([]float32, error) {
	close(p.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (*blockingEmbeddingProvider) Dimensions() int { return 1 }
func (*blockingEmbeddingProvider) Close() error    { return nil }

type failingEmbeddingProvider struct {
	err error
}

func (p failingEmbeddingProvider) Embed(context.Context, string) ([]float32, error) {
	return nil, p.err
}

func (failingEmbeddingProvider) Dimensions() int { return 1 }
func (failingEmbeddingProvider) Close() error    { return nil }

func TestHybridSearchContextCancellationDoesNotFallBackToLexicalResults(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedCancelableHybridSearch(t, db)

	provider := &blockingEmbeddingProvider{entered: make(chan struct{})}
	db.SetEmbeddingProvider(provider)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type outcome struct {
		results []SearchResult
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		results, err := db.HybridSearchContext(ctx, "cancellation", models.SearchOptions{AllProjects: true})
		done <- outcome{results: results, err: err}
	}()

	select {
	case <-provider.entered:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("embedding provider was not called")
	}

	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("HybridSearchContext error = %v, want context.Canceled", got.err)
		}
		if got.results != nil {
			t.Fatalf("HybridSearchContext fell back to partial lexical results: %+v", got.results)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HybridSearchContext did not return after cancellation")
	}
}

func TestHybridSearchContextProviderFailureKeepsLexicalFallback(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedCancelableHybridSearch(t, db)

	providerErr := errors.New("provider unavailable")
	db.SetEmbeddingProvider(failingEmbeddingProvider{err: providerErr})
	results, err := db.HybridSearchContext(context.Background(), "cancellation", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearchContext provider fallback error = %v", err)
	}
	if len(results) != 1 || results[0].Text != "cancellation lexical fallback" {
		t.Fatalf("HybridSearchContext provider fallback results = %+v", results)
	}
}

func TestHybridSearchContextProviderDeadlineDoesNotFallBack(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	seedCancelableHybridSearch(t, db)

	db.SetEmbeddingProvider(failingEmbeddingProvider{err: context.DeadlineExceeded})
	results, err := db.HybridSearchContext(context.Background(), "cancellation", models.SearchOptions{AllProjects: true})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("HybridSearchContext error = %v, want context.DeadlineExceeded", err)
	}
	if results != nil {
		t.Fatalf("HybridSearchContext fell back to partial lexical results: %+v", results)
	}
}

type cancellationAfterChecksContext struct {
	context.Context
	cancelAt int
	checks   int
	err      error
}

func (c *cancellationAfterChecksContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return c.err
	}
	return nil
}

func TestFilterVectorResultsContextCancellationReturnsNoPartialResults(t *testing.T) {
	for _, cancellation := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cancellation.Error(), func(t *testing.T) {
			ctx := &cancellationAfterChecksContext{
				Context:  context.Background(),
				cancelAt: 3,
				err:      cancellation,
			}
			results, err := filterVectorResultsContext(ctx, []VectorResult{
				{ItemID: 1, Similarity: 0.9},
				{ItemID: 2, Similarity: 0.8},
				{ItemID: 3, Similarity: 0.7},
			}, 0.5)
			if !errors.Is(err, cancellation) {
				t.Fatalf("filterVectorResultsContext error = %v, want %v", err, cancellation)
			}
			if results != nil {
				t.Fatalf("filterVectorResultsContext returned partial results: %+v", results)
			}
		})
	}
}

func TestOptionalStatsQueryCancellationClassifiesDriverErrors(t *testing.T) {
	for _, cancellation := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cancellation.Error(), func(t *testing.T) {
			driverErr := fmt.Errorf("optional count: %w", cancellation)
			got := optionalStatsQueryCancellation(context.Background(), driverErr)
			if !errors.Is(got, cancellation) {
				t.Fatalf("optionalStatsQueryCancellation error = %v, want %v", got, cancellation)
			}
		})
	}
	if got := optionalStatsQueryCancellation(context.Background(), errors.New("no such table")); got != nil {
		t.Fatalf("genuine optional-schema error was not tolerated: %v", got)
	}
}

func seedCancelableHybridSearch(t *testing.T, db *Database) {
	t.Helper()
	const sourcePath = "/test/query-cancellation.jsonl"
	if err := db.SyncFiles([]IndexedFile{{
		SourcePath: sourcePath,
		Source:     "session",
		Hash:       "query-cancellation",
		Project:    "test",
		Messages: []IndexedMessage{{
			Ordinal:     0,
			Role:        "user",
			Text:        "cancellation lexical fallback",
			ContentType: "text",
		}},
	}}); err != nil {
		t.Fatalf("seed search item: %v", err)
	}
	ids, err := db.InsertChunks(sourcePath, []ChunkRecord{{
		ChunkIdx:   0,
		Content:    "cancellation lexical fallback",
		TokenCount: 3,
	}}, time.Now().Unix())
	if err != nil {
		t.Fatalf("seed chunk: %v", err)
	}
	if err := db.InsertChunkEmbedding(ids[0], []float32{1}); err != nil {
		t.Fatalf("seed embedding: %v", err)
	}
}
