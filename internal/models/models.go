package models

import (
	"context"
	"time"
)

// SearchResult represents a single search result.
type SearchResult struct {
	Source      string // "session", "plan", "ke", "decision", etc.
	Role        string // "user", "assistant"
	Content     string // text content (possibly snippet)
	FilePath    string // path to original file
	Timestamp   time.Time
	SessionID   string
	ProjectPath string
	Score       float64
	Tags        []string
	ContentType string // "text", "code", "tool"
	Rank        int    // 1-based rank in results
}

// ParsedFile represents a parsed session or plan file.
type ParsedFile struct {
	Path    string
	Hash    string // SHA-256 hex
	Records []Message
}

// Message represents a message in a session.
type Message struct {
	Role        string
	Content     string
	ContentType string
	Timestamp   time.Time
}

// Stats represents indexing statistics.
type Stats struct {
	TotalFiles    int
	TotalMessages int
	IndexedAt     time.Time
}

// SearchOptions contains options for searching.
type SearchOptions struct {
	Project             string
	AllProjects         bool
	Source              string
	SourcePath          string // exact path, SQL LIKE pattern, or * glob
	After               *time.Time
	Before              *time.Time
	Role                string
	Limit               int
	Offset              int
	ContentType         string
	Tag                 string
	LexicalOnly         bool    // skip vector search, BM25 only
	SimilarityThreshold float64 // minimum cosine similarity (0 = no threshold)
}

// SyncOptions contains options for syncing.
type SyncOptions struct {
	IncludeAgents bool
	NoPlans       bool
}

// SearchEngine defines the interface for searching and syncing.
type SearchEngine interface {
	Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
	Sync(ctx context.Context, paths []string, opts SyncOptions) (Stats, error)
	Close() error
}
