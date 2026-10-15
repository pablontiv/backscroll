package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
