// Package services coordinates typed, read-only application queries.
package services

import (
	"context"
	"fmt"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

const (
	maxSearchLimit = 300

	maxContextWindow = 50

	// DefaultContextMaxTokens is the current context payload default.
	DefaultContextMaxTokens = 2000
	// MinContextMaxTokens is the smallest accepted context payload budget.
	MinContextMaxTokens = 64
	// MaxContextMaxTokens is the largest accepted context payload budget.
	MaxContextMaxTokens = 16384
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

// ValidateSearchRequest validates retrieval semantics without accessing storage.
func ValidateSearchRequest(request SearchRequest) error {
	if request.Query == "" {
		return fmt.Errorf("search query required (use --text <query> or positional argument)")
	}

	switch request.Options.ContentType {
	case "", "text", "code", "tool", "reasoning":
	default:
		return fmt.Errorf("invalid --content-type %q; must be one of: text, code, tool, reasoning", request.Options.ContentType)
	}

	// Limit 0 keeps the storage layer's historical internal default of 100.
	// The CLI supplies its separate historical default of 20.
	if request.Options.Limit < 0 || request.Options.Limit > maxSearchLimit {
		return fmt.Errorf("--limit must be between 0 and %d", maxSearchLimit)
	}
	if request.Options.Offset < 0 {
		return fmt.Errorf("--offset must be >= 0")
	}

	if request.Relax {
		return storage.ValidateRelaxationQuery(request.Query)
	}
	return nil
}

// ValidateExplicitSearchScope validates the base request and then requires one project scope.
// Remote consumers use this composed validator. The CLI keeps project inference before service use.
func ValidateExplicitSearchScope(request SearchRequest) error {
	if err := ValidateSearchRequest(request); err != nil {
		return err
	}
	hasProject := request.Options.Project != ""
	if hasProject == request.Options.AllProjects {
		return fmt.Errorf("provide exactly one project scope: project or all_projects=true")
	}
	return nil
}

// Search runs either ordinary hybrid retrieval or opt-in lexical relaxation.
func (s QueryService) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	if err := ctx.Err(); err != nil {
		return SearchResponse{}, err
	}
	if err := ValidateSearchRequest(request); err != nil {
		return SearchResponse{}, err
	}

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
	MaxTokens  int
}

// ContextResponse contains the anchor-relative indexed record window.
type ContextResponse struct {
	Records []storage.ContextRecord
}

// ValidateContextRequest validates selector and window semantics without accessing storage.
func ValidateContextRequest(request ContextRequest) error {
	uuidSelector := request.UUID != nil && request.SourcePath == nil && request.Ordinal == nil
	pathOrdinalSelector := request.UUID == nil && request.SourcePath != nil && request.Ordinal != nil
	if !uuidSelector && !pathOrdinalSelector {
		return fmt.Errorf("provide exactly one anchor selector: --uuid or --source-path with --ordinal")
	}
	if uuidSelector && *request.UUID == "" {
		return fmt.Errorf("--uuid must not be empty")
	}
	if pathOrdinalSelector && *request.SourcePath == "" {
		return fmt.Errorf("--source-path must not be empty")
	}
	if request.Before < 0 || request.Before > maxContextWindow {
		return fmt.Errorf("--before must be between 0 and %d", maxContextWindow)
	}
	if request.After < 0 || request.After > maxContextWindow {
		return fmt.Errorf("--after must be between 0 and %d", maxContextWindow)
	}
	if request.MaxTokens < MinContextMaxTokens || request.MaxTokens > MaxContextMaxTokens {
		return fmt.Errorf("--max-tokens must be between %d and %d", MinContextMaxTokens, MaxContextMaxTokens)
	}
	return nil
}

// Context retrieves an exact indexed neighborhood without consulting source files.
func (s QueryService) Context(ctx context.Context, request ContextRequest) (ContextResponse, error) {
	if err := ctx.Err(); err != nil {
		return ContextResponse{}, err
	}
	if err := ValidateContextRequest(request); err != nil {
		return ContextResponse{}, err
	}

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
