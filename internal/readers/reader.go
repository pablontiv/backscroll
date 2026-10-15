// Package readers provides the SessionReader interface and registry for
// parsing sessions from different sources (JSONL, SQLite, etc.).
package readers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
)

// SessionReader abstracts reading sessions from a source (file, database, etc.).
type SessionReader interface {
	// Name returns the format identifier (e.g., "jsonl", "opencode").
	Name() string
	// IngestionEpoch identifies this decoder's persisted parsing semantics.
	// Increment only the decoder whose interpretation of stable input changed.
	IngestionEpoch() uint64
	// Discover returns the list of session references (paths or IDs) matching the input definition.
	Discover(ctx context.Context, def input_config.InputDefinition) ([]string, error)
	// Hash returns a stable identifier for the current state of a session reference.
	// Used for incremental sync deduplication.
	Hash(ctx context.Context, sessionRef string) (string, error)
	// Parse parses a session reference into a ParsedFile.
	Parse(ctx context.Context, sessionRef string, def input_config.InputDefinition) (models.ParsedFile, error)
}

// IngestionFingerprint returns the versioned parsing contract persisted for a
// source. Source and reasoning are included alongside the decoder's own epoch,
// so stable bytes replay when any supported ingestion semantic changes.
func IngestionFingerprint(reader SessionReader, def input_config.InputDefinition) string {
	source := def.Source
	if source == "" {
		source = "session"
	}
	payload := make([]byte, 0, len(reader.Name())+len(source)+32)
	payload = appendFingerprintPart(payload, reader.Name())
	payload = strconv.AppendUint(payload, reader.IngestionEpoch(), 10)
	payload = append(payload, 0)
	payload = appendFingerprintPart(payload, source)
	if def.Decode.IndexReasoning {
		payload = append(payload, '1')
	} else {
		payload = append(payload, '0')
	}
	sum := sha256.Sum256(payload)
	var encoded [3 + sha256.Size*2]byte
	copy(encoded[:], "v1:")
	hex.Encode(encoded[3:], sum[:])
	return string(encoded[:])
}

func appendFingerprintPart(dst []byte, value string) []byte {
	dst = strconv.AppendInt(dst, int64(len(value)), 10)
	dst = append(dst, ':')
	dst = append(dst, value...)
	dst = append(dst, 0)
	return dst
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

// messageOriginForRole is used only after a reader has established that its
// native structured role field is an actor contract. Stored historical roles
// never pass through this function.
func messageOriginForRole(role string) models.MessageOrigin {
	switch role {
	case "user":
		return models.OriginHuman
	case "assistant", "reasoning":
		return models.OriginAssistant
	case "system":
		return models.OriginSystem
	case "tool":
		return models.OriginAutomation
	default:
		return models.OriginUnknown
	}
}
