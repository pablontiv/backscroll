package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/pablontiv/backscroll/internal/models"
)

// Format represents the output format type.
type Format string

const (
	FormatText  Format = "text"
	FormatJSON  Format = "json"
	FormatRobot Format = "robot"
)

// Formatter handles output formatting with optional token limiting.
type Formatter struct {
	Format    Format
	MaxTokens int // 0 = no limit
}

// NewFormatter creates a new Formatter with the given format and token limit.
func NewFormatter(format Format, maxTokens int) *Formatter {
	return &Formatter{
		Format:    format,
		MaxTokens: maxTokens,
	}
}

// WriteResults writes search results to the writer in the configured format.
func (f *Formatter) WriteResults(w io.Writer, results []models.SearchResult) error {
	// Apply token limiting if specified
	results = f.limitResults(results)

	switch f.Format {
	case FormatJSON:
		return f.writeJSON(w, results)
	case FormatRobot:
		return f.writeRobot(w, results)
	case FormatText:
		fallthrough
	default:
		return f.writeText(w, results)
	}
}

// WriteJSON writes arbitrary data as JSON to the writer.
func (f *Formatter) WriteJSON(w io.Writer, v any) error {
	return f.writeJSON(w, v)
}

// Private helper methods

func (f *Formatter) writeText(w io.Writer, results []models.SearchResult) error {
	for _, result := range results {
		// Write result header
		_, _ = fmt.Fprintf(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		_, _ = fmt.Fprintf(w, "Rank: %d | Source: %s | Role: %s | Score: %.2f\n", result.Rank, result.Source, result.Role, result.Score)
		_, _ = fmt.Fprintf(w, "Path: %s\n", result.FilePath)
		if !result.Timestamp.IsZero() {
			_, _ = fmt.Fprintf(w, "Time: %s\n", result.Timestamp.Format("2006-01-02 15:04:05"))
		}
		if result.SessionID != "" {
			_, _ = fmt.Fprintf(w, "Session: %s\n", result.SessionID)
		}
		if result.ProjectPath != "" {
			_, _ = fmt.Fprintf(w, "Project: %s\n", result.ProjectPath)
		}
		if len(result.Tags) > 0 {
			_, _ = fmt.Fprintf(w, "Tags: %s\n", strings.Join(result.Tags, ", "))
		}
		_, _ = fmt.Fprintf(w, "\n%s\n", result.Content)
	}
	return nil
}

func (f *Formatter) writeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func (f *Formatter) writeRobot(w io.Writer, results []models.SearchResult) error {
	for i, result := range results {
		_, _ = fmt.Fprintf(w, "result_%d_source=%s\n", i, result.Source)
		_, _ = fmt.Fprintf(w, "result_%d_role=%s\n", i, result.Role)
		_, _ = fmt.Fprintf(w, "result_%d_filepath=%s\n", i, result.FilePath)
		_, _ = fmt.Fprintf(w, "result_%d_content=%s\n", i, result.Content)
		if result.SessionID != "" {
			_, _ = fmt.Fprintf(w, "result_%d_session_id=%s\n", i, result.SessionID)
		}
		if result.ProjectPath != "" {
			_, _ = fmt.Fprintf(w, "result_%d_project=%s\n", i, result.ProjectPath)
		}
		_, _ = fmt.Fprintf(w, "result_%d_score=%.2f\n", i, result.Score)
		if len(result.Tags) > 0 {
			_, _ = fmt.Fprintf(w, "result_%d_tags=%s\n", i, strings.Join(result.Tags, ","))
		}
		_, _ = fmt.Fprintf(w, "result_%d_rank=%d\n", i, result.Rank)
	}
	return nil
}

// limitResults limits the results based on the MaxTokens setting.
// Uses approximate token counting: 1 word ≈ 1.3 tokens
func (f *Formatter) limitResults(results []models.SearchResult) []models.SearchResult {
	if f.MaxTokens <= 0 {
		return results
	}

	var limited []models.SearchResult
	tokenCount := 0

	for _, result := range results {
		// Estimate tokens in this result
		resultTokens := f.estimateTokens(result)
		if tokenCount+resultTokens > f.MaxTokens && len(limited) > 0 {
			// Adding this result would exceed limit, and we already have at least one
			break
		}
		limited = append(limited, result)
		tokenCount += resultTokens
	}

	return limited
}

// estimateTokens estimates the token count for a SearchResult.
// Uses simple heuristic: words * 1.3
func (f *Formatter) estimateTokens(result models.SearchResult) int {
	words := 0
	words += len(strings.Fields(result.Content))
	words += len(strings.Fields(result.FilePath))
	words += len(strings.Fields(result.SessionID))
	words += len(result.Tags)

	// 1 word ≈ 1.3 tokens, rounded up
	tokens := (words * 13) / 10
	if tokens < 10 {
		tokens = 10 // Minimum estimate per result
	}

	return tokens
}
