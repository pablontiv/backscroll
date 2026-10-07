// Package services coordinates typed, read-only application queries.
package services

import (
	"context"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

// QueryService coordinates read-only queries against the perennial index.
type QueryService struct {
	DB *storage.Database
}

// SearchRequest contains the query and retrieval policy for a search.
type SearchRequest struct {
	Query   string
	Options models.SearchOptions
	Relax   bool
}

// SearchProvenance describes the retrieval stages attempted by a search.
type SearchProvenance struct {
	RelaxationStages []string
}

// SearchResponse contains ranked search data and retrieval provenance.
type SearchResponse struct {
	Results    []storage.SearchResult
	Provenance SearchProvenance
}

// Search runs either ordinary hybrid retrieval or opt-in lexical relaxation.
func (s QueryService) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	if request.Relax {
		results, stages, err := s.DB.SearchRelaxedContext(ctx, request.Query, request.Options)
		return SearchResponse{
			Results: results,
			Provenance: SearchProvenance{
				RelaxationStages: stages,
			},
		}, err
	}

	results, err := s.DB.HybridSearchContext(ctx, request.Query, request.Options)
	return SearchResponse{Results: results}, err
}

// ContextRequest selects an exact anchor and its surrounding indexed records.
type ContextRequest struct {
	UUID       *string
	SourcePath *string
	Ordinal    *int64
	Before     int
	After      int
}

// ContextResponse contains the anchor-relative indexed record window.
type ContextResponse struct {
	Records []storage.ContextRecord
}

// Context retrieves an exact indexed neighborhood without consulting source files.
func (s QueryService) Context(ctx context.Context, request ContextRequest) (ContextResponse, error) {
	records, err := s.DB.QueryContextRecords(ctx, storage.ContextRecordQuery{
		UUID:       request.UUID,
		SourcePath: request.SourcePath,
		Ordinal:    request.Ordinal,
		Before:     request.Before,
		After:      request.After,
	})
	return ContextResponse{Records: records}, err
}

// StatusRequest identifies a typed status query.
type StatusRequest struct{}

// StatusResponse contains current index statistics.
type StatusResponse struct {
	Stats storage.Stats
}

// Status retrieves current index statistics.
func (s QueryService) Status(ctx context.Context, _ StatusRequest) (StatusResponse, error) {
	stats, err := s.DB.GetStatsContext(ctx)
	return StatusResponse{Stats: stats}, err
}
