// Package readers provides the SessionReader interface and registry for
// parsing sessions from different sources (JSONL, SQLite, etc.).
package readers

import (
	"fmt"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
)

// SessionReader abstracts reading sessions from a source (file, database, etc.).
type SessionReader interface {
	// Name returns the format identifier (e.g., "jsonl", "opencode").
	Name() string
	// Discover returns the list of session references (paths or IDs) matching the input definition.
	Discover(def input_config.InputDefinition) ([]string, error)
	// Hash returns a stable identifier for the current state of a session reference.
	// Used for incremental sync deduplication.
	Hash(sessionRef string) (string, error)
	// Parse parses a session reference into a ParsedFile.
	Parse(sessionRef string, def input_config.InputDefinition) (models.ParsedFile, error)
}

// Registry maps format names to their SessionReader implementations.
type Registry struct {
	readers map[string]SessionReader
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{readers: make(map[string]SessionReader)}
}

// Register adds a reader to the registry. Panics on duplicate name.
func (r *Registry) Register(reader SessionReader) {
	if _, exists := r.readers[reader.Name()]; exists {
		panic("readers: duplicate registration for " + reader.Name())
	}
	r.readers[reader.Name()] = reader
}

// Get returns the reader for the given format name.
func (r *Registry) Get(name string) (SessionReader, bool) {
	sr, ok := r.readers[name]
	return sr, ok
}

// Default returns the "claude" reader, or the first registered reader if claude is absent.
func (r *Registry) Default() SessionReader {
	if sr, ok := r.readers["claude"]; ok {
		return sr
	}
	for _, sr := range r.readers {
		return sr
	}
	return nil
}

// ForDef returns the reader appropriate for the given InputDefinition's decode format.
// Falls back to "claude" if the format is empty.
func (r *Registry) ForDef(def input_config.InputDefinition) (SessionReader, error) {
	format := def.Decode.Format
	if format == "" {
		format = "claude"
	}
	sr, ok := r.readers[format]
	if !ok {
		return nil, fmt.Errorf("no reader registered for format %q", format)
	}
	return sr, nil
}
