package models

// IndexedRecord represents a single canonical record from search_items.
type IndexedRecord struct {
	Source          string
	SourcePath      string
	Ordinal         int64
	Role            string
	Origin          MessageOrigin
	Text            string
	Project         *string
	ProjectEvidence string `json:",omitempty"`
	UUID            *string
	Timestamp       *string
	ContentType     string
	SearchEcho      bool `json:",omitempty"` // positive reader provenance, retained by recovery
}

// ValidProjectEvidence accepts the legacy empty representation of unknown.
func ValidProjectEvidence(value string) bool {
	switch value {
	case "", "unknown", "path_fallback", "registry", "native_cwd":
		return true
	}
	return false
}
